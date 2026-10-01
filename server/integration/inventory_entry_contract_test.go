//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

func TestSQLiteInventoryEntryContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory-entries.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryEntryContract(t, router, db)
}
func TestPostgresInventoryEntryContract(t *testing.T) {
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
	assertInventoryEntryContract(t, router, db)
}

type entryDTO struct {
	ID                   int64     `json:"id"`
	ProductID            int64     `json:"productId"`
	AdjustmentID         int64     `json:"adjustmentId"`
	AdjustmentItemID     int64     `json:"adjustmentItemId"`
	EntryType            string    `json:"entryType"`
	Quantity             string    `json:"quantity"`
	BalanceBefore        string    `json:"balanceBefore"`
	BalanceAfter         string    `json:"balanceAfter"`
	Reason               string    `json:"reason"`
	Remark               *string   `json:"remark"`
	ProductCode          string    `json:"productCode"`
	ProductName          string    `json:"productName"`
	ProductModel         *string   `json:"productModel"`
	ProductSpecification *string   `json:"productSpecification"`
	Unit                 string    `json:"unit"`
	OperatorID           int64     `json:"operatorId"`
	OperatorName         string    `json:"operatorName"`
	DocumentNo           string    `json:"documentNo"`
	OccurredAt           time.Time `json:"occurredAt"`
	CreateTime           time.Time `json:"createTime"`
}
type entryPageDTO struct {
	Records  []entryDTO `json:"records"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
}

func assertInventoryEntryContract(t *testing.T, router http.Handler, db *platformdatabase.Database) {
	t.Helper()
	token := loginAdmin(t, router)
	const entryPath = "/api/v1/inventory/entries"
	page := func(query string) entryPageDTO {
		return inventoryData[entryPageDTO](t, serveJSON(router, http.MethodGet, entryPath+query, "", token), 200)
	}
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, entryPath, "", ""))

	products := map[string]int64{}
	units := map[string]string{}
	for _, spec := range []struct{ code, unit string }{{"ENT-A", "台"}, {"ENT-B", "支"}, {"ENT-C", "个"}} {
		units[spec.code] = spec.unit
		products[spec.code] = inventoryData[struct {
			ID int64 `json:"id"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":%q,"name":%q,"type":"GOODS","unit":%q,"model":"型号丁","specification":"规格丁","category":"追溯"}`, spec.code, spec.code+"名称", spec.unit), token), 200).ID
	}
	item := func(code, quantity, reason string, remark ...string) adjustmentItemInput {
		in := adjustmentItemInput{ProductID: products[code], ProductType: "GOODS", Unit: units[code], Quantity: quantity, Reason: reason}
		if len(remark) == 1 {
			in.Remark = remark[0]
		}
		return in
	}
	create := func(items ...adjustmentItemInput) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(items...), token), 200)
	}
	post := func(document adjustmentDTO) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(document.ID)+"/post", `{"version":1}`, token), 200)
	}
	cancel := func(document adjustmentDTO, version int64, reason string) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(document.ID)+"/cancel", cancelBody(version, reason), token), 200)
	}
	// A second operator proves that the ledger records the actual actor of each
	// posting, not the creator of the document.
	inventoryData[any](t, serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"ledger-operator","nickname":"流水操作员","deptId":1,"status":1}`, token), 200)
	operatorToken := loginUser(t, router, "ledger-operator", "admin123")

	if empty := page(""); empty.Total != 0 || len(empty.Records) != 0 {
		t.Fatalf("ledger before posting=%+v", empty)
	}
	for _, query := range []string{"?page=0", "?pageSize=0", "?page=abc", "?productId=0", "?productId=-1", "?productId=x", "?entryType=SOMETHING", "?occurredFrom=2026-01-01", "?occurredTo=bad", "?occurredFrom=2026-02-01T00:00:00Z&occurredTo=2026-01-01T00:00:00Z"} {
		inventoryData[any](t, serveJSON(router, http.MethodGet, entryPath+query, "", token), 400)
	}
	for _, id := range []string{"0", "-1", "bad", "999999"} {
		inventoryData[any](t, serveJSON(router, http.MethodGet, entryPath+"/"+id, "", token), 404)
	}

	// Posting a mixed document writes one ledger line per product, each with its
	// own before/after balance and the operator who actually posted it.
	intake := create(item("ENT-A", "10", "OPENING"), item("ENT-B", "4", "SURPLUS", "同行盘盈"))
	postedIntake := post(intake)
	afterIntake := page("?productId=" + itoa(products["ENT-A"]))
	if afterIntake.Total != 1 || len(afterIntake.Records) != 1 {
		t.Fatalf("ledger after posting=%+v", afterIntake)
	}
	first := afterIntake.Records[0]
	if first.EntryType != "ORIGINAL" || first.Quantity != "10" || first.BalanceBefore != "0" || first.BalanceAfter != "10" || first.Reason != "OPENING" || first.Remark != nil || first.ProductCode != "ENT-A" || first.ProductName != "ENT-A名称" || first.ProductModel == nil || *first.ProductModel != "型号丁" || first.ProductSpecification == nil || *first.ProductSpecification != "规格丁" || first.Unit != "台" || first.OperatorID != 1 || first.OperatorName != "管理员" || first.DocumentNo != postedIntake.DocumentNo || first.AdjustmentID != intake.ID || first.OccurredAt.IsZero() || first.CreateTime.IsZero() {
		t.Fatalf("original ledger row=%+v", first)
	}
	if remark := page("?productId=" + itoa(products["ENT-B"])).Records[0].Remark; remark == nil || *remark != "同行盘盈" {
		t.Fatalf("ledger remark=%v", remark)
	}

	// A later decrease and a cancellation reversal are both listed, in stable
	// order, and can be told apart by the entry type.
	reduction := post(create(item("ENT-A", "-7", "SHORTAGE")))
	cancelled := cancel(reduction, 2, "数量录错")
	ledger := page("?productId=" + itoa(products["ENT-A"]) + "&pageSize=500")
	if ledger.Total != 3 || len(ledger.Records) != 3 {
		t.Fatalf("ledger after cancellation=%+v", ledger)
	}
	if ledger.Records[0].EntryType != "REVERSAL" || ledger.Records[1].EntryType != "ORIGINAL" || ledger.Records[2].EntryType != "ORIGINAL" {
		t.Fatalf("ledger order=%+v", ledger.Records)
	}
	reversal := ledger.Records[0]
	original := ledger.Records[1]
	if reversal.Quantity != "7" || reversal.BalanceBefore != "3" || reversal.BalanceAfter != "10" || reversal.Reason != "SHORTAGE" || reversal.DocumentNo != cancelled.DocumentNo || reversal.OccurredAt.Before(original.OccurredAt) {
		t.Fatalf("reversal row=%+v", reversal)
	}
	// The reversal names the same source line as its original, and both the
	// original entry and the document stay untouched.
	if reversal.AdjustmentItemID != original.AdjustmentItemID || reversal.AdjustmentID != original.AdjustmentID || reversal.ProductCode != "ENT-A" || reversal.ProductName != "ENT-A名称" || reversal.Unit != "台" {
		t.Fatalf("reversal does not trace back to its original: %+v vs %+v", reversal, original)
	}
	afterCancel := page("?productId=" + itoa(products["ENT-A"]) + "&entryType=REVERSAL")
	if afterCancel.Total != 1 || afterCancel.Records[0].ID != reversal.ID {
		t.Fatalf("reversal filter=%+v", afterCancel)
	}
	if originals := page("?productId=" + itoa(products["ENT-A"]) + "&entryType=ORIGINAL"); originals.Total != 2 || originals.Records[0].ID != original.ID {
		t.Fatalf("original filter=%+v", originals)
	}
	// The document detail still shows the cancellation reason, so the ledger can
	// explain why the reversal exists.
	documentDetail := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(reduction.ID), "", token), 200)
	if documentDetail.Status != "CANCELLED" || documentDetail.CancelReason == nil || *documentDetail.CancelReason != "数量录错" || documentDetail.CancelledByName != "管理员" {
		t.Fatalf("cancellation record=%+v", documentDetail)
	}

	// The creator of a document is not automatically its operator: this draft is
	// created by the administrator and posted by the second account, and the
	// ledger names the account that actually posted it.
	otherIntake := create(item("ENT-C", "2.5", "OPENING"))
	postedOther := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(otherIntake.ID)+"/post", `{"version":1}`, operatorToken), 200)
	otherRow := page("?productId=" + itoa(products["ENT-C"])).Records[0]
	if otherRow.OperatorID != 2 || otherRow.OperatorName != "流水操作员" || otherRow.DocumentNo != postedOther.DocumentNo || otherRow.Quantity != "2.5" || otherRow.BalanceAfter != "2.5" {
		t.Fatalf("ledger operator=%+v", otherRow)
	}
	detailOther := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(otherIntake.ID), "", operatorToken), 200)
	if detailOther.CreatedBy != 1 || detailOther.CreatedByName != "管理员" || detailOther.PostedBy == nil || *detailOther.PostedBy != 2 || detailOther.PostedByName != "流水操作员" || !detailOther.PostedAt.Equal(otherRow.OccurredAt) {
		t.Fatalf("document posting record=%+v entry=%+v", detailOther, otherRow)
	}

	// History survives renaming, disabling and zeroing a product: the ledger
	// keeps the posting-time text while current stock shows the new one.
	productPath := "/api/v1/products/" + itoa(products["ENT-A"])
	inventoryData[any](t, serveJSON(router, http.MethodPut, productPath, `{"name":"改名后的ENT-A","type":"GOODS","unit":"台","model":"新型号","specification":"新规格"}`, token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["ENT-A"])+"/status", `{"status":0}`, token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPut, productPath, `{"name":"改名后的ENT-A","type":"GOODS","unit":"台","model":"新型号","specification":"新规格"}`, token), 200)
	renamed := page("?productId=" + itoa(products["ENT-A"]))
	if renamed.Total != 3 {
		t.Fatalf("history after disabling=%+v", renamed)
	}
	for _, record := range renamed.Records {
		if record.ProductName != "ENT-A名称" || record.ProductModel == nil || *record.ProductModel != "型号丁" || record.ProductSpecification == nil || *record.ProductSpecification != "规格丁" {
			t.Fatalf("ledger snapshot rewritten: %+v", record)
		}
	}
	// Current stock, by contrast, reports the live description.
	balances := inventoryData[balancePageDTO](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/balances?keyword=改名后的ENT-A", "", token), 200)
	if balances.Total != 1 || balances.Records[0].Name != "改名后的ENT-A" || balances.Records[0].Quantity != "10" || balances.Records[0].Status != 0 {
		t.Fatalf("current stock after renaming=%+v", balances)
	}
	// The product-linked entry query needs no catalog state: it keeps working
	// after the product is disabled or renamed.
	if linked := page(fmt.Sprintf("?productId=%d&entryType=ORIGINAL", products["ENT-A"])); linked.Total != 2 {
		t.Fatalf("linked history=%+v", linked)
	}

	// Occurrence-time bounds are inclusive at the lower edge and exclusive at the
	// upper edge, and the same ordering holds across the boundary.
	inside := page("?productId=" + itoa(products["ENT-A"]) + "&occurredFrom=" + first.OccurredAt.UTC().Format(time.RFC3339Nano))
	if inside.Total != 3 {
		t.Fatalf("lower bound should include rows=%+v", inside)
	}
	if before := page("?productId=" + itoa(products["ENT-A"]) + "&occurredFrom=" + reversal.OccurredAt.Add(time.Second).UTC().Format(time.RFC3339Nano)); before.Total != 0 {
		t.Fatalf("future lower bound=%+v", before)
	}
	if upper := page("?productId=" + itoa(products["ENT-A"]) + "&occurredTo=" + first.OccurredAt.UTC().Format(time.RFC3339Nano)); upper.Total != 0 {
		t.Fatalf("upper bound should exclude the same instant=%+v", upper)
	}

	// Unfiltered paging is stable and complete, and the total counts every line.
	all, seen := page("?pageSize=500"), map[int64]bool{}
	if all.Total != 5 || len(all.Records) != 5 {
		t.Fatalf("all entries=%+v", all)
	}
	for p := 1; p <= 3; p++ {
		result := page(fmt.Sprintf("?page=%d&pageSize=2", p))
		if result.Total != 5 || result.Page != p || result.PageSize != 2 || len(result.Records) > 2 {
			t.Fatalf("page %d=%+v", p, result)
		}
		for _, record := range result.Records {
			if seen[record.ID] {
				t.Fatalf("entry %d appeared on two pages", record.ID)
			}
			seen[record.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Fatalf("paging covered %d entries", len(seen))
	}
	// Repeated identical queries return the same order, so the ledger cannot
	// appear to change between page loads.
	for run := 0; run < 3; run++ {
		repeat := page("?pageSize=500")
		for i := range repeat.Records {
			if repeat.Records[i].ID != all.Records[i].ID {
				t.Fatalf("unstable ledger ordering: %v vs %v", repeat.Records, all.Records)
			}
		}
	}

	// A document-level query lists every line of one adjustment, which is what
	// the source link needs, and the adjustment page still finds cancelled
	// documents by product without duplicating them.
	documentLines := []entryDTO{}
	for _, record := range all.Records {
		if record.AdjustmentID == intake.ID {
			documentLines = append(documentLines, record)
		}
	}
	if len(documentLines) != 2 {
		t.Fatalf("source document lines=%+v", documentLines)
	}
	filtered := inventoryData[adjustmentPageDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"?productId="+itoa(products["ENT-A"])+"&pageSize=50", "", token), 200)
	if filtered.Total != 2 {
		t.Fatalf("adjustment page by product=%+v", filtered)
	}
	cancelledOnly := inventoryData[adjustmentPageDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"?status=CANCELLED&productId="+itoa(products["ENT-A"]), "", token), 200)
	if cancelledOnly.Total != 1 || cancelledOnly.Records[0].ID != reduction.ID {
		t.Fatalf("cancelled adjustment by product=%+v", cancelledOnly)
	}

	// The ledger is append-only: no write method is routed, and the stored rows
	// and balances agree with the lines the API reports.
	if value := balanceRows(t, db, products["ENT-A"]); len(value) != 1 || value[0] != 10000 {
		t.Fatalf("stored balance=%v", value)
	}
	type productLine struct {
		ProductID int64
		Total     int64
	}
	lines := []productLine{}
	if e := db.GORM.Table("inventory_entry").Select("product_id,SUM(quantity_milli) AS total").Group("product_id").Scan(&lines).Error; e != nil {
		t.Fatal(e)
	}
	for _, line := range lines {
		balance := balanceRows(t, db, line.ProductID)
		if len(balance) != 1 || balance[0] != line.Total {
			t.Fatalf("product %d balance=%v ledger total=%d", line.ProductID, balance, line.Total)
		}
	}
	var entriesBefore int64
	if e := db.GORM.Table("inventory_entry").Count(&entriesBefore).Error; e != nil {
		t.Fatal(e)
	}
	for _, target := range []struct{ method, path string }{
		{http.MethodPost, entryPath}, {http.MethodPut, entryPath + "/" + itoa(all.Records[0].ID)},
		{http.MethodPatch, entryPath + "/" + itoa(all.Records[0].ID)}, {http.MethodDelete, entryPath},
	} {
		response := serveJSON(router, target.method, target.path, `{"quantity":"999"}`, token)
		if response.Code != http.StatusNotFound && response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s = %d, body=%s", target.method, target.path, response.Code, response.Body.String())
		}
	}
	var entriesAfter int64
	if e := db.GORM.Table("inventory_entry").Count(&entriesAfter).Error; e != nil {
		t.Fatal(e)
	}
	if entriesAfter != entriesBefore {
		t.Fatalf("a write attempt changed the ledger: %d -> %d", entriesBefore, entriesAfter)
	}
}
