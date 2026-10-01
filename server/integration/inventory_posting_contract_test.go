//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

func TestSQLiteInventoryPostingContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory-posting.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryPostingContract(t, router, db, "sqlite")
}
func TestPostgresInventoryPostingContract(t *testing.T) {
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
	assertInventoryPostingContract(t, router, db, "postgres")
}

type inventoryLedgerRow struct {
	ID                   int64
	ProductID            int64
	AdjustmentID         int64
	AdjustmentItemID     int64
	EntryType            string
	QuantityMilli        int64
	BalanceBeforeMilli   int64
	BalanceAfterMilli    int64
	Reason               string
	Remark               *string
	ProductCode          string
	ProductName          string
	ProductModel         *string
	ProductSpecification *string
	Unit                 string
	OperatorID           int64
	OccurredAt           time.Time
}

func ledgerRows(t *testing.T, db *platformdatabase.Database, productID int64) []inventoryLedgerRow {
	t.Helper()
	rows := []inventoryLedgerRow{}
	if e := db.GORM.Table("inventory_entry").Where("product_id=?", productID).Order("id ASC").Scan(&rows).Error; e != nil {
		t.Fatal(e)
	}
	return rows
}
func balanceRows(t *testing.T, db *platformdatabase.Database, productID int64) []int64 {
	t.Helper()
	rows := []int64{}
	if e := db.GORM.Table("inventory_balance").Where("product_id=?", productID).Order("id ASC").Pluck("quantity_milli", &rows).Error; e != nil {
		t.Fatal(e)
	}
	return rows
}
func productUnit(t *testing.T, db *platformdatabase.Database, productID int64) string {
	t.Helper()
	var unit string
	if e := db.GORM.Table("product").Where("id=?", productID).Pluck("unit", &unit).Error; e != nil {
		t.Fatal(e)
	}
	return unit
}

func assertInventoryPostingContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	assertUnauthenticated(t, serveJSON(router, http.MethodPost, adjustmentPath+"/1/post", `{"version":1}`, ""))
	specs := []struct{ code, kind, unit string }{
		{"INV-A", "GOODS", "台"}, {"INV-B", "GOODS", "支"}, {"INV-C", "GOODS", "个"}, {"INV-D", "GOODS", "件"},
		{"INV-E", "GOODS", "台"}, {"INV-F", "GOODS", "台"}, {"INV-R", "GOODS", "台"}, {"INV-H", "GOODS", "台"},
		{"INV-PLAIN", "GOODS", "台"}, {"INV-DISABLED", "GOODS", "个"},
	}
	products := map[string]int64{}
	units := map[string]string{}
	for _, spec := range specs {
		units[spec.code] = spec.unit
		response := serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":%q,"name":%q,"type":%q,"unit":%q,"model":"型号甲","specification":"规格甲"}`, spec.code, spec.code+"名称", spec.kind, spec.unit), token)
		products[spec.code] = inventoryData[struct {
			ID int64 `json:"id"`
		}](t, response, 200).ID
	}
	item := func(code, quantity, reason string, remark ...string) adjustmentItemInput {
		in := adjustmentItemInput{ProductID: products[code], ProductType: "GOODS", Unit: units[code], Quantity: quantity, Reason: reason}
		if len(remark) == 1 {
			in.Remark = remark[0]
		}
		return in
	}
	createDraft := func(items ...adjustmentItemInput) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(items...), token), 200)
	}
	detail := func(id int64) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(id), "", token), 200)
	}
	post := func(id, version int64) *httptest.ResponseRecorder {
		return serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(id)+"/post", fmt.Sprintf(`{"version":%d}`, version), token)
	}
	postAudits := func() int64 { return inventoryActionCount(t, db, "inventory.adjustment.post") }

	// Only the confirmed version identifies the reviewed document.
	draft := createDraft(item("INV-A", "10", "OPENING"))
	draftPath := adjustmentPath + "/" + itoa(draft.ID)
	for _, body := range []string{`{}`, `{"version":0}`, `{"version":-1}`, `{"version":9223372036854775807}`, `{"version":"1"}`, `{"version":1.5}`, `{"version":9223372036854775808}`, `{"version":null}`} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, draftPath+"/post", body, token), 400)
	}
	for _, id := range []string{"0", "-1", "bad", "9223372036854775808"} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+id+"/post", `{"version":1}`, token), 400)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPost, adjustmentPath+"/999999/post", `{"version":1}`, token), 404)
	inventoryData[any](t, serveJSON(router, http.MethodPost, draftPath+"/post", `{"version":2}`, token), 409)
	sameDraft(t, draft, detail(draft.ID))

	// A mixed document posts once, creating the first balance for each product
	// and recording the impact per line.
	mixed := createDraft(item("INV-A", "10", "OPENING"), item("INV-B", "5", "SURPLUS"))
	posted := inventoryData[adjustmentDTO](t, post(mixed.ID, 1), 200)
	if posted.Status != "POSTED" || posted.Version != 2 || posted.PostedBy == nil || *posted.PostedBy != 1 || posted.PostedByName != "管理员" || posted.PostedAt == nil || posted.PostedAt.IsZero() || len(posted.Items) != 2 {
		t.Fatalf("posted document=%+v", posted)
	}
	for i, want := range []struct{ before, after string }{{"0", "10"}, {"0", "5"}} {
		got, ok := "", posted.Items[i].BalanceBefore != nil && posted.Items[i].BalanceAfter != nil
		if ok {
			got = *posted.Items[i].BalanceBefore + "/" + *posted.Items[i].BalanceAfter
		}
		if !ok || got != want.before+"/"+want.after {
			t.Fatalf("posted item %d impact=%q want %q", i, got, want.before+"/"+want.after)
		}
	}
	if posted.Items[0].ProductCode != "INV-A" || posted.Items[0].ProductName != "INV-A名称" || posted.Items[0].ProductModel != "型号甲" || posted.Items[0].ProductSpecification != "规格甲" || posted.Items[1].Unit != "支" {
		t.Fatalf("posted snapshots=%+v", posted.Items)
	}
	if before, after := balanceRows(t, db, products["INV-A"]), balanceRows(t, db, products["INV-B"]); len(before) != 1 || before[0] != 10000 || len(after) != 1 || after[0] != 5000 {
		t.Fatalf("balances A=%v B=%v", before, after)
	}
	rowsA := ledgerRows(t, db, products["INV-A"])
	if len(rowsA) != 1 {
		t.Fatalf("ledger rows A=%+v", rowsA)
	}
	if rows := rowsA[0]; rows.EntryType != "ORIGINAL" || rows.QuantityMilli != 10000 || rows.BalanceBeforeMilli != 0 || rows.BalanceAfterMilli != 10000 || rows.Reason != "OPENING" || rows.Remark != nil || rows.ProductCode != "INV-A" || rows.ProductName != "INV-A名称" || rows.ProductModel == nil || *rows.ProductModel != "型号甲" || rows.ProductSpecification == nil || *rows.ProductSpecification != "规格甲" || rows.Unit != "台" || rows.OperatorID != 1 || rows.OccurredAt.IsZero() || rows.AdjustmentID != mixed.ID {
		t.Fatalf("ledger row=%+v", rows)
	}
	// A posted document is frozen: no edit, no second post, and no stale-version
	// cancellation. Reversing a posted document is a later task's concern; here
	// a stale cancellation must simply not touch the posted stock.
	inventoryData[any](t, serveJSON(router, http.MethodPut, adjustmentPath+"/"+itoa(mixed.ID), editBody(2, item("INV-A", "99", "OPENING")), token), 409)
	for _, version := range []int64{1, 2, 3} {
		inventoryData[any](t, post(mixed.ID, version), 409)
	}
	for _, stale := range []int64{1, 3} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(mixed.ID)+"/cancel", cancelBody(stale, "过期版本取消"), token), 409)
	}
	sameDraft(t, posted, detail(mixed.ID))
	if len(ledgerRows(t, db, products["INV-A"])) != 1 || postAudits() != 1 {
		t.Fatal("repeated posting created extra ledger rows or audit records")
	}

	// A second document moves the existing balances in both directions.
	second := createDraft(item("INV-A", "-4", "SHORTAGE"), item("INV-B", "-2", "DAMAGE"))
	postedSecond := inventoryData[adjustmentDTO](t, post(second.ID, 1), 200)
	if *postedSecond.Items[0].BalanceBefore != "10" || *postedSecond.Items[0].BalanceAfter != "6" || *postedSecond.Items[1].BalanceBefore != "5" || *postedSecond.Items[1].BalanceAfter != "3" {
		t.Fatalf("second impacts=%+v", postedSecond.Items)
	}
	if before := balanceRows(t, db, products["INV-A"]); len(before) != 1 || before[0] != 6000 {
		t.Fatalf("balance A=%v", before)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPost, draftPath+"/post", `{"version":1}`, token), 200)
	balanceA := balanceRows(t, db, products["INV-A"])[0]

	// Any line that would go negative rejects the whole document and leaves both
	// the draft and every product untouched.
	entriesBefore := len(ledgerRows(t, db, products["INV-A"]))
	shortfalls := []struct {
		name   string
		items  []adjustmentItemInput
		absent string
	}{
		{"single line", []adjustmentItemInput{item("INV-A", "-100", "SHORTAGE")}, ""},
		{"mixed lines", []adjustmentItemInput{item("INV-A", "1", "SURPLUS"), item("INV-B", "-999", "DAMAGE")}, "INV-B"},
		{"no balance row", []adjustmentItemInput{item("INV-D", "-1", "SHORTAGE")}, "INV-D"},
	}
	for _, test := range shortfalls {
		document := createDraft(test.items...)
		response := post(document.ID, 1)
		inventoryData[any](t, response, 409)
		if !strings.Contains(response.Body.String(), "库存不足") {
			t.Fatalf("%s shortage response=%s", test.name, response.Body.String())
		}
		sameDraft(t, document, detail(document.ID))
	}
	if len(ledgerRows(t, db, products["INV-A"])) != entriesBefore || balanceRows(t, db, products["INV-A"])[0] != balanceA {
		t.Fatal("rejected document changed an earlier product in the same transaction")
	}
	if rows := ledgerRows(t, db, products["INV-D"]); len(rows) != 0 {
		t.Fatalf("missing balance row produced ledger rows=%+v", rows)
	}

	// Disabled products and units that no longer match the confirmed line are
	// rejected before any balance moves; both become postable again afterwards.
	// The draft is created while the product is enabled, then the product is
	// disabled, so the rejection comes from confirmation and not from drafting.
	disabled := createDraft(item("INV-DISABLED", "1", "OPENING"))
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["INV-DISABLED"])+"/status", `{"status":0}`, token), 200)
	response := post(disabled.ID, 1)
	inventoryData[any](t, response, 400)
	if !strings.Contains(response.Body.String(), "启用实物商品") {
		t.Fatalf("disabled product response=%s", response.Body.String())
	}
	if rows := ledgerRows(t, db, products["INV-DISABLED"]); len(rows) != 0 {
		t.Fatalf("disabled product posted=%+v", rows)
	}
	sameDraft(t, disabled, detail(disabled.ID))
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["INV-DISABLED"])+"/status", `{"status":1}`, token), 200)
	inventoryData[adjustmentDTO](t, post(disabled.ID, 1), 200)

	changedUnit := createDraft(item("INV-H", "1", "OPENING"))
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["INV-H"]), `{"name":"单位已改变","type":"GOODS","unit":"件"}`, token), 200)
	inventoryData[any](t, post(changedUnit.ID, 1), 400)
	if rows := ledgerRows(t, db, products["INV-H"]); len(rows) != 0 || productUnit(t, db, products["INV-H"]) != "件" {
		t.Fatalf("stale unit posted=%+v", rows)
	}

	// Unrepresentable sums are refused instead of wrapping around.
	maxed := createDraft(item("INV-F", "9223372036854775.807", "SURPLUS"))
	inventoryData[adjustmentDTO](t, post(maxed.ID, 1), 200)
	if balance := balanceRows(t, db, products["INV-F"]); len(balance) != 1 || balance[0] != 9223372036854775807 {
		t.Fatalf("maximum balance=%v", balance)
	}
	overflow := createDraft(item("INV-F", "0.001", "SURPLUS"))
	response = post(overflow.ID, 1)
	inventoryData[any](t, response, 409)
	if !strings.Contains(response.Body.String(), "可表示范围") {
		t.Fatalf("overflow response=%s", response.Body.String())
	}
	sameDraft(t, overflow, detail(overflow.ID))
	if balance := balanceRows(t, db, products["INV-F"]); balance[0] != 9223372036854775807 || len(ledgerRows(t, db, products["INV-F"])) != 1 {
		t.Fatal("overflow changed the stored balance or the ledger")
	}

	// Thousandths survive posting unchanged, in both directions.
	thousandths := createDraft(item("INV-B", "0.001", "SURPLUS"))
	postedThousandths := inventoryData[adjustmentDTO](t, post(thousandths.ID, 1), 200)
	if *postedThousandths.Items[0].BalanceBefore != "3" || *postedThousandths.Items[0].BalanceAfter != "3.001" {
		t.Fatalf("thousandth increase impact=%v → %v", postedThousandths.Items[0].BalanceBefore, postedThousandths.Items[0].BalanceAfter)
	}
	back := createDraft(item("INV-B", "-0.001", "SHORTAGE"))
	postedBack := inventoryData[adjustmentDTO](t, post(back.ID, 1), 200)
	if *postedBack.Items[0].BalanceBefore != "3.001" || *postedBack.Items[0].BalanceAfter != "3" {
		t.Fatalf("thousandth decrease impact=%v → %v", postedBack.Items[0].BalanceBefore, postedBack.Items[0].BalanceAfter)
	}
	if balance := balanceRows(t, db, products["INV-B"]); len(balance) != 1 || balance[0] != 3000 {
		t.Fatalf("thousandth balance=%v", balance)
	}

	// Posting and cancelling the same draft race on one version: exactly one
	// transition is effective and the stored state matches the winner. A draft
	// cancellation never moves stock, so the losing posting must leave the
	// ledger and the balance untouched.
	raceCancel := createDraft(item("INV-E", "2", "OPENING"))
	raceCancelPath := adjustmentPath + "/" + itoa(raceCancel.ID)
	outcomeStarted := make(chan struct{})
	outcomeResponses := make(chan struct {
		post bool
		code int
	}, 2)
	var outcomeWG sync.WaitGroup
	outcomeWG.Add(2)
	go func() {
		defer outcomeWG.Done()
		<-outcomeStarted
		outcomeResponses <- struct {
			post bool
			code int
		}{true, post(raceCancel.ID, 1).Code}
	}()
	go func() {
		defer outcomeWG.Done()
		<-outcomeStarted
		response := serveJSON(router, http.MethodPost, raceCancelPath+"/cancel", cancelBody(1, "并发取消"), token)
		outcomeResponses <- struct {
			post bool
			code int
		}{false, response.Code}
	}()
	close(outcomeStarted)
	outcomeWG.Wait()
	close(outcomeResponses)
	postStatus, cancelStatus := 0, 0
	for response := range outcomeResponses {
		if response.post {
			postStatus = response.code
			continue
		}
		cancelStatus = response.code
	}
	settled := detail(raceCancel.ID)
	switch {
	case postStatus == 200 && cancelStatus == 409:
		if settled.Status != "POSTED" || len(ledgerRows(t, db, products["INV-E"])) != 1 {
			t.Fatalf("posting won but state=%s rows=%d", settled.Status, len(ledgerRows(t, db, products["INV-E"])))
		}
	case cancelStatus == 200 && postStatus == 409:
		if settled.Status != "CANCELLED" || len(ledgerRows(t, db, products["INV-E"])) != 0 || len(balanceRows(t, db, products["INV-E"])) != 0 {
			t.Fatalf("cancellation won but state=%s rows=%d balances=%v", settled.Status, len(ledgerRows(t, db, products["INV-E"])), balanceRows(t, db, products["INV-E"]))
		}
	default:
		t.Fatalf("post/cancel race post=%d cancel=%d", postStatus, cancelStatus)
	}

	// The same document posted concurrently is effective exactly once.
	race := createDraft(item("INV-C", "3", "OPENING"))
	raceStarted := make(chan struct{})
	raceResponses := make(chan *httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-raceStarted; raceResponses <- post(race.ID, 1) }()
	}
	close(raceStarted)
	wg.Wait()
	close(raceResponses)
	success, conflicts := 0, 0
	for response := range raceResponses {
		if response.Code == 200 {
			inventoryData[adjustmentDTO](t, response, 200)
			success++
		} else {
			inventoryData[any](t, response, 409)
			conflicts++
		}
	}
	if success != 1 || conflicts != 7 || balanceRows(t, db, products["INV-C"])[0] != 3000 || len(ledgerRows(t, db, products["INV-C"])) != 1 {
		t.Fatalf("concurrent same-document posting success=%d conflicts=%d", success, conflicts)
	}

	// Stock 3 with two documents of 2 each: at most one may post, so the balance
	// can never go negative.
	firstOut, secondOut := createDraft(item("INV-C", "-2", "SHORTAGE")), createDraft(item("INV-C", "-2", "SHORTAGE"))
	outStarted := make(chan struct{})
	outResponses := make(chan *httptest.ResponseRecorder, 2)
	wg = sync.WaitGroup{}
	for _, id := range []int64{firstOut.ID, secondOut.ID} {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); <-outStarted; outResponses <- post(id, 1) }(id)
	}
	close(outStarted)
	wg.Wait()
	close(outResponses)
	success, conflicts = 0, 0
	for response := range outResponses {
		if response.Code == 200 {
			inventoryData[adjustmentDTO](t, response, 200)
			success++
		} else {
			inventoryData[any](t, response, 409)
			conflicts++
		}
	}
	if success != 1 || conflicts != 1 || balanceRows(t, db, products["INV-C"])[0] != 1000 || len(ledgerRows(t, db, products["INV-C"])) != 2 {
		t.Fatalf("concurrent decrement success=%d conflicts=%d balance=%v", success, conflicts, balanceRows(t, db, products["INV-C"]))
	}

	// Concurrent first increases on one product create exactly one balance row
	// and lose no update, whatever the number of parallel documents.
	firstIncreases := make([]adjustmentDTO, 0, 4)
	for i := 0; i < 4; i++ {
		firstIncreases = append(firstIncreases, createDraft(item("INV-D", "1", "SURPLUS")))
	}
	inStarted := make(chan struct{})
	inResponses := make(chan *httptest.ResponseRecorder, len(firstIncreases))
	wg = sync.WaitGroup{}
	for _, document := range firstIncreases {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); <-inStarted; inResponses <- post(id, 1) }(document.ID)
	}
	close(inStarted)
	wg.Wait()
	close(inResponses)
	for response := range inResponses {
		inventoryData[adjustmentDTO](t, response, 200)
	}
	if balance := balanceRows(t, db, products["INV-D"]); len(balance) != 1 || balance[0] != 4000 {
		t.Fatalf("concurrent first increase balance=%v", balance)
	}
	if len(ledgerRows(t, db, products["INV-D"])) != 4 {
		t.Fatal("concurrent first increase lost or duplicated ledger rows")
	}

	// A product with inventory history keeps its identity: descriptions stay
	// editable, while type and base unit are refused by the API itself.
	productPath := "/api/v1/products/" + itoa(products["INV-A"])
	renamed := inventoryData[struct {
		Name            string `json:"name"`
		Unit            string `json:"unit"`
		InventoryLocked bool   `json:"inventoryLocked"`
	}](t, serveJSON(router, http.MethodPut, productPath, `{"name":"改名后的A","type":"GOODS","unit":"台"}`, token), 200)
	if renamed.Name != "改名后的A" || renamed.Unit != "台" || !renamed.InventoryLocked {
		t.Fatalf("description edit=%+v", renamed)
	}
	for _, body := range []string{`{"name":"改成服务","type":"SERVICE","unit":"台"}`, `{"name":"改单位","type":"GOODS","unit":"件"}`, `{"name":"改单位","type":"GOODS","unit":" 件 "}`} {
		inventoryData[any](t, serveJSON(router, http.MethodPut, productPath, body, token), 409)
	}
	if productUnit(t, db, products["INV-A"]) != "台" {
		t.Fatal("locked unit was changed")
	}
	// Posted history keeps the description it was posted with.
	if name := detail(mixed.ID).Items[0].ProductName; name != "INV-A名称" {
		t.Fatalf("posted snapshot was rewritten to %q", name)
	}
	plainPath := "/api/v1/products/" + itoa(products["INV-PLAIN"])
	plain := inventoryData[struct {
		Type            string `json:"type"`
		Unit            string `json:"unit"`
		InventoryLocked bool   `json:"inventoryLocked"`
	}](t, serveJSON(router, http.MethodPut, plainPath, `{"name":"无流水可改","type":"SERVICE","unit":"次"}`, token), 200)
	if plain.Type != "SERVICE" || plain.Unit != "次" || plain.InventoryLocked {
		t.Fatalf("product without history=%+v", plain)
	}

	// A product edit racing the first posting obeys the same lock instead of
	// deciding from a history read outside the writing transaction.
	identityRace := createDraft(item("INV-R", "1", "OPENING"))
	identityStarted := make(chan struct{})
	identityResponses := make(chan struct {
		post bool
		code int
	}, 2)
	wg = sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-identityStarted
		identityResponses <- struct {
			post bool
			code int
		}{true, post(identityRace.ID, 1).Code}
	}()
	go func() {
		defer wg.Done()
		<-identityStarted
		response := serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["INV-R"]), `{"name":"R改名","type":"GOODS","unit":"件"}`, token)
		identityResponses <- struct {
			post bool
			code int
		}{false, response.Code}
	}()
	close(identityStarted)
	wg.Wait()
	close(identityResponses)
	postStatus, editStatus := 0, 0
	for response := range identityResponses {
		if response.post {
			postStatus = response.code
			continue
		}
		editStatus = response.code
	}
	rowsR := ledgerRows(t, db, products["INV-R"])
	switch {
	case postStatus == 200 && editStatus == 409:
		if productUnit(t, db, products["INV-R"]) != "台" || len(rowsR) != 1 || rowsR[0].Unit != "台" {
			t.Fatalf("posting won but identity changed: unit=%s rows=%+v", productUnit(t, db, products["INV-R"]), rowsR)
		}
	case editStatus == 200 && (postStatus == 400 || postStatus == 409):
		if len(rowsR) != 0 || productUnit(t, db, products["INV-R"]) != "件" {
			t.Fatalf("edit won but stock moved: unit=%s rows=%+v", productUnit(t, db, products["INV-R"]), rowsR)
		}
	default:
		t.Fatalf("identity race post=%d edit=%d", postStatus, editStatus)
	}

	// A failing audit rolls back balance, ledger, status and version together.
	audited := createDraft(item("INV-A", "1", "SURPLUS"))
	balanceBefore := balanceRows(t, db, products["INV-A"])[0]
	entriesBefore = len(ledgerRows(t, db, products["INV-A"]))
	postAuditsBefore := postAudits()
	installInventoryAuditFailure(t, db, dialect)
	inventoryData[any](t, post(audited.ID, 1), 500)
	removeInventoryAuditFailure(t, db, dialect)
	sameDraft(t, audited, detail(audited.ID))
	if balanceRows(t, db, products["INV-A"])[0] != balanceBefore || len(ledgerRows(t, db, products["INV-A"])) != entriesBefore || postAudits() != postAuditsBefore {
		t.Fatal("failed audit left stock, ledger or audit changes behind")
	}
	inventoryData[adjustmentDTO](t, post(audited.ID, 1), 200)

	// Every stored balance equals the sum of its committed ledger lines and no
	// product is negative.
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
		if len(balance) != 1 || balance[0] != line.Total || balance[0] < 0 {
			t.Fatalf("product %d balance=%v ledger total=%d", line.ProductID, balance, line.Total)
		}
	}
	for _, spec := range specs {
		if len(ledgerRows(t, db, products[spec.code])) == 0 && len(balanceRows(t, db, products[spec.code])) != 0 {
			t.Fatalf("product %s has a balance without any ledger line", spec.code)
		}
	}
}
