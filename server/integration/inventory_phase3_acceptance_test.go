//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

// decimalMilli parses the API's decimal quantity text without ever using a
// float, so the verification matches how the server stores the value.
func decimalMilli(t *testing.T, value string) int64 {
	t.Helper()
	text := value
	sign := int64(1)
	if strings.HasPrefix(text, "-") {
		sign, text = -1, text[1:]
	}
	whole, fraction, _ := strings.Cut(text, ".")
	if whole == "" || len(fraction) > 3 {
		t.Fatalf("quantity %q is not a valid decimal string", value)
	}
	milli := int64(0)
	for _, digit := range whole + fraction + strings.Repeat("0", 3-len(fraction)) {
		if digit < '0' || digit > '9' {
			t.Fatalf("quantity %q is not a valid decimal string", value)
		}
		milli = milli*10 + int64(digit-'0')
	}
	return sign * milli
}

// TestSQLiteInventoryPhase3Acceptance walks the whole Phase 3 operator journey on
// one database: create a draft, edit it, post mixed increases and decreases,
// read current stock and the ledger, cancel the posting by reversal and read the
// reversal. Every step is asserted against the stored state, and the closing
// check compares each stored balance with the accumulated ledger lines.
func TestSQLiteInventoryPhase3Acceptance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory-phase3.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryPhase3Journey(t, router, db)
}
func TestPostgresInventoryPhase3Acceptance(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryPhase3Journey(t, router, db)
}

func assertInventoryPhase3Journey(t *testing.T, router http.Handler, db *platformdatabase.Database) {
	t.Helper()
	token := loginAdmin(t, router)
	products, units := map[string]int64{}, map[string]string{}
	for _, spec := range []struct{ code, unit string }{{"PH-A", "台"}, {"PH-B", "支"}, {"PH-C", "个"}} {
		units[spec.code] = spec.unit
		products[spec.code] = inventoryData[struct {
			ID int64 `json:"id"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":%q,"name":%q,"type":"GOODS","unit":%q,"category":"阶段验收"}`, spec.code, spec.code+"名称", spec.unit), token), 200).ID
	}
	item := func(code, quantity, reason string) adjustmentItemInput {
		return adjustmentItemInput{ProductID: products[code], ProductType: "GOODS", Unit: units[code], Quantity: quantity, Reason: reason}
	}
	detail := func(id int64) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(id), "", token), 200)
	}

	// 1. A draft is created and then edited before anything reaches the stock.
	draft := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(item("PH-A", "10", "OPENING")), token), 200)
	if draft.Status != "DRAFT" || draft.Version != 1 {
		t.Fatalf("created draft=%+v", draft)
	}
	edited := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPut, adjustmentPath+"/"+itoa(draft.ID), editBody(1, item("PH-A", "8", "OPENING"), item("PH-B", "5", "SURPLUS")), token), 200)
	if edited.Status != "DRAFT" || edited.Version != 2 || len(edited.Items) != 2 {
		t.Fatalf("edited draft=%+v", edited)
	}
	if stale := serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(draft.ID)+"/post", `{"version":1}`, token); stale.Code != http.StatusConflict {
		t.Fatalf("stale version posting=%d body=%s", stale.Code, stale.Body.String())
	}
	// A draft is visible as a product but contributes no stock: every physical
	// product still reads zero and no balance row exists yet.
	switch stock := inventoryData[balancePageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/balances?stock=all&keyword=PH-&pageSize=50", "", token), 200); {
	case stock.Total != 3 || len(stock.Records) != 3:
		t.Fatalf("draft-only stock view=%+v", stock)
	default:
		for _, record := range stock.Records {
			if record.Quantity != "0" {
				t.Fatalf("a draft changed stock: %+v", record)
			}
		}
	}
	if stored := balanceRows(t, db, products["PH-A"]); len(stored) != 0 {
		t.Fatalf("a draft created a balance row=%v", stored)
	}

	// 2. Mixed increases post as one document.
	posted := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(draft.ID)+"/post", `{"version":2}`, token), 200)
	if posted.Status != "POSTED" || posted.Version != 3 || posted.PostedBy == nil || posted.PostedAt == nil {
		t.Fatalf("posted draft=%+v", posted)
	}
	if *posted.Items[0].BalanceBefore != "0" || *posted.Items[0].BalanceAfter != "8" || *posted.Items[1].BalanceAfter != "5" {
		t.Fatalf("posting impact=%+v", posted.Items)
	}

	// 3. Current stock answers "how much", the ledger answers "why".
	stock := inventoryData[balancePageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/balances?keyword=PH-&pageSize=50", "", token), 200)
	if stock.Total != 2 {
		t.Fatalf("current stock=%+v", stock)
	}
	for _, record := range stock.Records {
		switch record.Code {
		case "PH-A":
			if record.Quantity != "8" {
				t.Fatalf("PH-A stock=%+v", record)
			}
		case "PH-B":
			if record.Quantity != "5" {
				t.Fatalf("PH-B stock=%+v", record)
			}
		default:
			t.Fatalf("unexpected product in stock=%+v", record)
		}
	}
	ledger := inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?productId="+itoa(products["PH-A"]), "", token), 200)
	if ledger.Total != 1 || ledger.Records[0].EntryType != "ORIGINAL" || ledger.Records[0].Quantity != "8" || ledger.Records[0].DocumentNo != posted.DocumentNo || ledger.Records[0].DocumentNo == "" || ledger.Records[0].OperatorName != "管理员" {
		t.Fatalf("PH-A ledger=%+v", ledger)
	}

	// 4. A later decrease, then a zeroing adjustment.
	post := func(items ...adjustmentItemInput) adjustmentDTO {
		document := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(items...), token), 200)
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(document.ID)+"/post", `{"version":1}`, token), 200)
	}
	decrease := post(item("PH-A", "-3", "SHORTAGE"))
	if decrease.Status != "POSTED" {
		t.Fatalf("decrease posting=%+v", decrease)
	}
	if stock := inventoryData[balancePageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/balances?keyword=PH-A", "", token), 200); stock.Records[0].Quantity != "5" {
		t.Fatalf("stock after decrease=%+v", stock)
	}
	post(item("PH-C", "1", "OPENING"))
	post(item("PH-C", "-1", "SHORTAGE"))
	if stock := inventoryData[balancePageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/balances?stock=zero&keyword=PH-C", "", token), 200); stock.Total != 1 || stock.Records[0].Quantity != "0" {
		t.Fatalf("zero view=%+v", stock)
	}

	// 5. Cancelling the original increase is refused while it would go negative,
	// and the same cancellation succeeds once the decrease is reversed.
	refused := serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(draft.ID)+"/cancel", cancelBody(3, "重复录入"), token)
	if refused.Code != http.StatusConflict {
		t.Fatalf("negative reversal must be refused: %d body=%s", refused.Code, refused.Body.String())
	}
	if kept := detail(draft.ID); kept.Status != "POSTED" || kept.Version != 3 {
		t.Fatalf("refused cancellation changed the document=%+v", kept)
	}
	reversedDecrease := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(decrease.ID)+"/cancel", cancelBody(2, "数量录错"), token), 200)
	if reversedDecrease.Status != "CANCELLED" || reversedDecrease.PostedBy == nil {
		t.Fatalf("reversed decrease=%+v", reversedDecrease)
	}
	cancelled := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(draft.ID)+"/cancel", cancelBody(3, "重复录入"), token), 200)
	if cancelled.Status != "CANCELLED" || cancelled.PostedBy == nil || cancelled.PostedAt == nil || cancelled.CancelledBy == nil || cancelled.CancelReason == nil {
		t.Fatalf("cancelled posting=%+v", cancelled)
	}

	// 6. The ledger keeps every line: two originals and two reversals for PH-A,
	// and the filter tells them apart.
	phA := inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?productId="+itoa(products["PH-A"])+"&pageSize=500", "", token), 200)
	originals, reversals := 0, 0
	for _, record := range phA.Records {
		switch record.EntryType {
		case "ORIGINAL":
			originals++
		case "REVERSAL":
			reversals++
		default:
			t.Fatalf("unexpected entry type=%+v", record)
		}
	}
	if phA.Total != 4 || originals != 2 || reversals != 2 {
		t.Fatalf("PH-A history=%+v", phA)
	}
	// The reversal of the decrease is visible as such, keeps the original
	// snapshot, and links back to the document whose cancellation caused it.
	reversalPage := inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?productId="+itoa(products["PH-A"])+"&entryType=REVERSAL", "", token), 200)
	if reversalPage.Total != 2 || len(reversalPage.Records) != 2 {
		t.Fatalf("reversal page=%+v", reversalPage)
	}
	bySource := map[string]entryDTO{}
	for _, record := range reversalPage.Records {
		bySource[record.DocumentNo] = record
	}
	decreaseReversal, ok := bySource[reversedDecrease.DocumentNo]
	if !ok {
		t.Fatalf("reversal lost its source document=%+v", reversalPage)
	}
	// The reversal mirrors the original line: same snapshot, same reason, and the
	// exact opposite sign, while the original row is still present untouched.
	if decreaseReversal.Quantity != "3" || decreaseReversal.Reason != "SHORTAGE" || decreaseReversal.ProductName != "PH-A名称" || decreaseReversal.ProductCode != "PH-A" || decreaseReversal.Unit != "台" || decreaseReversal.BalanceBefore != "5" || decreaseReversal.BalanceAfter != "8" {
		t.Fatalf("decrease reversal=%+v", decreaseReversal)
	}
	intakeReversal, ok := bySource[draft.DocumentNo]
	if !ok || intakeReversal.Quantity != "-8" || intakeReversal.BalanceBefore != "8" || intakeReversal.BalanceAfter != "0" {
		t.Fatalf("intake reversal=%+v", intakeReversal)
	}
	originalsPage := inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?productId="+itoa(products["PH-A"])+"&entryType=ORIGINAL", "", token), 200)
	if originalsPage.Total != 2 {
		t.Fatalf("originals after reversal=%+v", originalsPage)
	}
	for _, record := range originalsPage.Records {
		if record.Quantity != "8" && record.Quantity != "-3" {
			t.Fatalf("original row changed=%+v", record)
		}
	}

	// 7. Every stored balance equals the sum of all its committed ledger lines.
	type productLine struct {
		ProductID int64
		Total     int64
	}
	lines := []productLine{}
	if e := db.GORM.Table("inventory_entry").Select("product_id,SUM(quantity_milli) AS total").Group("product_id").Scan(&lines).Error; e != nil {
		t.Fatal(e)
	}
	if len(lines) != 3 {
		t.Fatalf("ledger product groups=%+v", lines)
	}
	for _, line := range lines {
		balance := balanceRows(t, db, line.ProductID)
		if len(balance) != 1 || balance[0] != line.Total || balance[0] < 0 {
			t.Fatalf("product %d balance=%v ledger total=%d", line.ProductID, balance, line.Total)
		}
		// The API's decimal text must sum to exactly the stored integer, which is
		// the check that no rounding happens between storage and display.
		page := inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?productId="+itoa(line.ProductID)+"&pageSize=500", "", token), 200)
		var sum int64
		for _, record := range page.Records {
			sum += decimalMilli(t, record.Quantity)
		}
		if sum != line.Total {
			t.Fatalf("product %d reported lines sum=%d stored total=%d", line.ProductID, sum, line.Total)
		}
		for _, record := range page.Records {
			if decimalMilli(t, record.BalanceAfter)-decimalMilli(t, record.BalanceBefore) != decimalMilli(t, record.Quantity) {
				t.Fatalf("line %d balance delta does not match its quantity: %+v", record.ID, record)
			}
		}
	}
	// The stock view reports the same quantity the ledger sums to.
	final := inventoryData[balancePageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/balances?stock=all&keyword=PH-&pageSize=50", "", token), 200)
	if final.Total != 3 {
		t.Fatalf("final stock view=%+v", final)
	}
	for _, record := range final.Records {
		var stored int64
		if e := db.GORM.Table("inventory_balance").Where("product_id=?", record.ProductID).Pluck("quantity_milli", &stored).Error; e != nil {
			t.Fatal(e)
		}
		if decimalMilli(t, record.Quantity) != stored {
			t.Fatalf("reported stock %q does not match stored %d for product %d", record.Quantity, stored, record.ProductID)
		}
	}
	// Reading the whole history twice yields the same rows in the same order.
	once, twice := inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?pageSize=500", "", token), 200), inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?pageSize=500", "", token), 200)
	if once.Total != twice.Total || len(once.Records) != len(twice.Records) {
		t.Fatalf("ledger totals differ between reads: %+v vs %+v", once, twice)
	}
	for i := range once.Records {
		if once.Records[i].ID != twice.Records[i].ID || !once.Records[i].OccurredAt.UTC().Truncate(time.Microsecond).Equal(twice.Records[i].OccurredAt.UTC().Truncate(time.Microsecond)) {
			t.Fatalf("unstable ledger order at %d", i)
		}
	}
	assertNullDescriptionSnapshot(t, router)
}

// assertNullDescriptionSnapshot covers the distinction between an editable
// draft, whose description is live, and a posted document, whose NULL values
// are frozen. The detail request is also the source-document view opened from
// the ledger, so it verifies that source tracing preserves the same snapshot.
func assertNullDescriptionSnapshot(t *testing.T, router http.Handler) {
	t.Helper()
	token := loginAdmin(t, router)
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"PH-NULL-SNAPSHOT","name":"空描述快照","type":"GOODS","unit":"个"}`, token), 200)
	created := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(adjustmentItemInput{
		ProductID: product.ID, ProductType: "GOODS", Unit: "个", Quantity: "2", Reason: "OPENING",
	}), token), 200)
	if len(created.Items) != 1 || created.Items[0].ProductModel != "" || created.Items[0].ProductSpecification != "" {
		t.Fatalf("new draft should show empty live model/specification: %+v", created.Items)
	}
	posted := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(created.ID)+"/post", `{"version":1}`, token), 200)
	if len(posted.Items) != 1 || posted.Items[0].ProductModel != "" || posted.Items[0].ProductSpecification != "" {
		t.Fatalf("posted line did not preserve empty model/specification: %+v", posted.Items)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(product.ID), `{"name":"后来补全","type":"GOODS","unit":"个","model":"后来型号","specification":"后来规格"}`, token), 200)
	detail := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(created.ID), "", token), 200)
	if detail.Status != "POSTED" || len(detail.Items) != 1 || detail.Items[0].ProductModel != "" || detail.Items[0].ProductSpecification != "" || detail.PostedBy == nil || detail.PostedAt == nil {
		t.Fatalf("source document detail changed its empty posting snapshot: %+v", detail)
	}
	ledger := inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?productId="+itoa(product.ID), "", token), 200)
	if ledger.Total != 1 || len(ledger.Records) != 1 || ledger.Records[0].ProductModel != nil || ledger.Records[0].ProductSpecification != nil {
		t.Fatalf("ledger snapshot differs from posted source: %+v", ledger)
	}
	cancelled := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(created.ID)+"/cancel", cancelBody(2, "验收空快照取消"), token), 200)
	if cancelled.Status != "CANCELLED" || len(cancelled.Items) != 1 || cancelled.Items[0].ProductModel != "" || cancelled.Items[0].ProductSpecification != "" || cancelled.CancelledBy == nil || cancelled.CancelledAt == nil {
		t.Fatalf("cancelled source document changed its empty posting snapshot: %+v", cancelled)
	}
	ledger = inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/entries?productId="+itoa(product.ID)+"&pageSize=10", "", token), 200)
	if ledger.Total != 2 {
		t.Fatalf("expected original and reversal after cancellation, got %+v", ledger)
	}
	for _, row := range ledger.Records {
		if row.ProductModel != nil || row.ProductSpecification != nil || row.ProductName != "空描述快照" || row.ProductCode != "PH-NULL-SNAPSHOT" {
			t.Fatalf("ledger snapshot changed after catalog edit/cancellation: %+v", row)
		}
	}
}
