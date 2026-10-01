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

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

func TestSQLiteInventoryPostedCancelContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory-posted-cancel.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryPostedCancelContract(t, router, db, "sqlite")
}
func TestPostgresInventoryPostedCancelContract(t *testing.T) {
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
	assertInventoryPostedCancelContract(t, router, db, "postgres")
}

// reversalRows lists the ledger lines of one type for a product, which is how a
// cancellation reversal is told apart from the original posting.
func reversalRows(t *testing.T, db *platformdatabase.Database, productID int64, entryType string) []inventoryLedgerRow {
	t.Helper()
	rows := []inventoryLedgerRow{}
	if e := db.GORM.Table("inventory_entry").Where("product_id=? AND entry_type=?", productID, entryType).Order("id ASC").Scan(&rows).Error; e != nil {
		t.Fatal(e)
	}
	return rows
}

func assertInventoryPostedCancelContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	specs := []struct{ code string }{{"CAN-A"}, {"CAN-B"}, {"CAN-C"}, {"CAN-D"}, {"CAN-E"}, {"CAN-F"}}
	products := map[string]int64{}
	for _, spec := range specs {
		products[spec.code] = inventoryData[struct {
			ID int64 `json:"id"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":%q,"name":%q,"type":"GOODS","unit":"台","model":"型号乙","specification":"规格乙"}`, spec.code, spec.code+"名称"), token), 200).ID
	}
	item := func(code, quantity, reason string) adjustmentItemInput {
		return adjustmentItemInput{ProductID: products[code], ProductType: "GOODS", Unit: "台", Quantity: quantity, Reason: reason}
	}
	createDraft := func(items ...adjustmentItemInput) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(items...), token), 200)
	}
	detail := func(id int64) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(id), "", token), 200)
	}
	post := func(id, version int64) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(id)+"/post", fmt.Sprintf(`{"version":%d}`, version), token), 200)
	}
	cancel := func(id, version int64, reason string) *httptest.ResponseRecorder {
		return serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(id)+"/cancel", cancelBody(version, reason), token)
	}
	balanceOf := func(code string) int64 {
		rows := balanceRows(t, db, products[code])
		if len(rows) != 1 {
			t.Fatalf("balance rows of %s=%v", code, rows)
		}
		return rows[0]
	}

	// A posted document is cancelled by reversing it, never by reloading the
	// catalog: the reversal repeats the posting snapshot and the opposite sign.
	intake := post(createDraft(item("CAN-A", "10", "OPENING"), item("CAN-B", "4", "SURPLUS")).ID, 1)
	intakeCancelPath := adjustmentPath + "/" + itoa(intake.ID) + "/cancel"
	assertUnauthenticated(t, serveJSON(router, http.MethodPost, intakeCancelPath, cancelBody(2, "误录"), ""))
	for _, body := range []string{`{}`, cancelBody(0, "误录"), cancelBody(-1, "误录"), cancelBody(9223372036854775807, "误录"), cancelBody(2, ""), cancelBody(2, " \n\t "), cancelBody(2, strings.Repeat("长", 501)), `{"version":2,"reason":1}`, `{"version":"2","reason":"误录"}`} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, intakeCancelPath, body, token), 400)
	}
	for _, id := range []string{"0", "-1", "bad", "9223372036854775808"} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+id+"/cancel", cancelBody(1, "误录"), token), 400)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPost, adjustmentPath+"/999999/cancel", cancelBody(1, "误录"), token), 404)
	sameDraft(t, intake, detail(intake.ID))

	// A later decrease leaves stock 3 for a line that originally added 10, so its
	// cancellation must be refused as a whole rather than producing -7.
	reduction := post(createDraft(item("CAN-A", "-7", "SHORTAGE")).ID, 1)
	if balanceOf("CAN-A") != 3000 {
		t.Fatalf("balance after reduction=%d", balanceOf("CAN-A"))
	}
	refused := cancel(intake.ID, 2, "误录，需要撤销")
	inventoryData[any](t, refused, 409)
	if !strings.Contains(refused.Body.String(), "整单未取消") {
		t.Fatalf("insufficient reversal response=%s", refused.Body.String())
	}
	kept := detail(intake.ID)
	if kept.Status != "POSTED" || kept.Version != 2 || kept.PostedBy == nil || kept.PostedAt == nil || kept.CancelledBy != nil || kept.CancelReason != nil {
		t.Fatalf("refused cancellation changed the document=%+v", kept)
	}
	sameDraft(t, intake, kept)
	if balanceOf("CAN-A") != 3000 || len(reversalRows(t, db, products["CAN-A"], "REVERSAL")) != 0 || len(ledgerRows(t, db, products["CAN-A"])) != 2 {
		t.Fatal("refused cancellation moved stock or appended a reversal")
	}

	// Reversing the decrease first is allowed and restores 10, and only then can
	// the original increase be reversed, leaving exactly zero.
	reductionCancelled := inventoryData[adjustmentDTO](t, cancel(reduction.ID, 2, "数量录错"), 200)
	if reductionCancelled.Status != "CANCELLED" || reductionCancelled.Version != 3 || reductionCancelled.CancelledBy == nil || *reductionCancelled.CancelledBy != 1 || reductionCancelled.CancelledByName != "管理员" || reductionCancelled.CancelledAt == nil || reductionCancelled.CancelReason == nil || *reductionCancelled.CancelReason != "数量录错" || reductionCancelled.PostedBy == nil || reductionCancelled.PostedAt == nil {
		t.Fatalf("cancelled posted document=%+v", reductionCancelled)
	}
	if balanceOf("CAN-A") != 10000 {
		t.Fatalf("balance after reversing the decrease=%d", balanceOf("CAN-A"))
	}
	// The detail keeps the impact the document actually had when it was posted;
	// the reversal lives in the ledger and never rewrites the original line.
	if *reductionCancelled.Items[0].BalanceBefore != "10" || *reductionCancelled.Items[0].BalanceAfter != "3" {
		t.Fatalf("posted impact must stay the original one: %+v", reductionCancelled.Items[0])
	}
	reversals := reversalRows(t, db, products["CAN-A"], "REVERSAL")
	if len(reversals) != 1 {
		t.Fatalf("reversal rows=%+v", reversals)
	}
	if row := reversals[0]; row.QuantityMilli != 7000 || row.BalanceBeforeMilli != 3000 || row.BalanceAfterMilli != 10000 || row.Reason != "SHORTAGE" || row.ProductCode != "CAN-A" || row.ProductName != "CAN-A名称" || row.ProductModel == nil || *row.ProductModel != "型号乙" || row.Unit != "台" || row.OperatorID != 1 || row.AdjustmentID != reduction.ID || row.OccurredAt.IsZero() {
		t.Fatalf("reversal row=%+v", row)
	}
	// The reversal points back at its own original line and no original record
	// was rewritten.
	originals := reversalRows(t, db, products["CAN-A"], "ORIGINAL")
	if len(originals) != 2 || originals[1].AdjustmentItemID != reduction.Items[0].ID || originals[1].QuantityMilli != -7000 || reversals[0].AdjustmentItemID != originals[1].AdjustmentItemID || reversals[0].AdjustmentID != reduction.ID {
		t.Fatalf("original rows=%+v reversal=%+v", originals, reversals[0])
	}

	// Cancelling the increase now succeeds, and the document keeps both the
	// posting and the cancellation records.
	intakeCancelled := inventoryData[adjustmentDTO](t, cancel(intake.ID, 2, "重复录入"), 200)
	if intakeCancelled.Status != "CANCELLED" || intakeCancelled.Version != 3 || intakeCancelled.PostedBy == nil || *intakeCancelled.PostedBy != 1 || intakeCancelled.PostedAt == nil || intakeCancelled.CancelReason == nil || *intakeCancelled.CancelReason != "重复录入" || len(intakeCancelled.Items) != 2 {
		t.Fatalf("cancelled intake=%+v", intakeCancelled)
	}
	if balanceOf("CAN-A") != 0 || balanceOf("CAN-B") != 0 {
		t.Fatalf("balances A=%d B=%d", balanceOf("CAN-A"), balanceOf("CAN-B"))
	}
	if rows := ledgerRows(t, db, products["CAN-B"]); len(rows) != 2 || rows[1].EntryType != "REVERSAL" || rows[1].QuantityMilli != -4000 || rows[1].BalanceBeforeMilli != 4000 || rows[1].BalanceAfterMilli != 0 {
		t.Fatalf("CAN-B ledger=%+v", rows)
	}
	// Zero stock must not unlock the product identity.
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["CAN-A"]), `{"name":"改名","type":"GOODS","unit":"件"}`, token), 409)
	renamed := inventoryData[struct {
		Name            string `json:"name"`
		InventoryLocked bool   `json:"inventoryLocked"`
	}](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["CAN-A"]), `{"name":"归零后改名","type":"GOODS","unit":"台"}`, token), 200)
	if renamed.Name != "归零后改名" || !renamed.InventoryLocked {
		t.Fatalf("zeroed product edit=%+v", renamed)
	}

	// A cancelled document is final: no edit, no re-post, no second cancel.
	intakePath := adjustmentPath + "/" + itoa(intake.ID)
	inventoryData[any](t, serveJSON(router, http.MethodPut, intakePath, editBody(3, item("CAN-A", "1", "SURPLUS")), token), 409)
	inventoryData[any](t, serveJSON(router, http.MethodPost, intakePath+"/post", `{"version":3}`, token), 409)
	for _, version := range []int64{1, 2, 3} {
		inventoryData[any](t, cancel(intake.ID, version, "重复取消"), 409)
	}
	sameDraft(t, intakeCancelled, detail(intake.ID))
	if len(reversalRows(t, db, products["CAN-A"], "REVERSAL")) != 2 || inventoryActionCount(t, db, "inventory.adjustment.cancel") != 2 {
		t.Fatal("repeated cancellation appended reversal rows or audit records")
	}

	// A draft cancellation is unchanged: no reversal, no stock movement, and the
	// cancellation records stay empty of posting metadata.
	draft := createDraft(item("CAN-C", "5", "OPENING"))
	draftCancelled := inventoryData[adjustmentDTO](t, cancel(draft.ID, 1, "不再需要"), 200)
	if draftCancelled.Status != "CANCELLED" || draftCancelled.PostedBy != nil || draftCancelled.PostedAt != nil || len(balanceRows(t, db, products["CAN-C"])) != 0 || len(ledgerRows(t, db, products["CAN-C"])) != 0 {
		t.Fatalf("draft cancellation=%+v", draftCancelled)
	}

	// A product disabled after posting can still have its history cancelled.
	disabledDoc := post(createDraft(item("CAN-D", "2", "OPENING")).ID, 1)
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["CAN-D"])+"/status", `{"status":0}`, token), 200)
	disabledCancelled := inventoryData[adjustmentDTO](t, cancel(disabledDoc.ID, 2, "停用后仍可取消"), 200)
	if disabledCancelled.Status != "CANCELLED" || balanceOf("CAN-D") != 0 || len(ledgerRows(t, db, products["CAN-D"])) != 2 {
		t.Fatalf("disabled product cancellation=%+v", disabledCancelled)
	}
	// The product stays disabled; cancelling history never re-enables it.
	var status int
	if e := db.GORM.Table("product").Where("id=?", products["CAN-D"]).Pluck("status", &status).Error; e != nil || status != 0 {
		t.Fatalf("disabled product status=%d err=%v", status, e)
	}

	// Concurrent cancellations of the same posted document reverse it once.
	raceDoc := post(createDraft(item("CAN-E", "3", "OPENING")).ID, 1)
	racePath := adjustmentPath + "/" + itoa(raceDoc.ID) + "/cancel"
	cancelAudits := inventoryActionCount(t, db, "inventory.adjustment.cancel")
	raceStarted := make(chan struct{})
	raceResponses := make(chan *httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-raceStarted
			raceResponses <- serveJSON(router, http.MethodPost, racePath, cancelBody(2, "并发取消"), token)
		}()
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
	if success != 1 || conflicts != 7 || balanceOf("CAN-E") != 0 || len(reversalRows(t, db, products["CAN-E"], "REVERSAL")) != 1 || inventoryActionCount(t, db, "inventory.adjustment.cancel") != cancelAudits+1 {
		t.Fatalf("concurrent cancellation success=%d conflicts=%d rows=%+v", success, conflicts, reversalRows(t, db, products["CAN-E"], "REVERSAL"))
	}

	// Two cancellations that would each take 2 out of stock 2 cannot both
	// succeed, and the refused one keeps its document posted.
	firstIn := post(createDraft(item("CAN-C", "2", "SURPLUS")).ID, 1)
	secondIn := post(createDraft(item("CAN-C", "2", "SURPLUS")).ID, 1)
	post(createDraft(item("CAN-C", "-2", "SHORTAGE")).ID, 1)
	if balanceOf("CAN-C") != 2000 {
		t.Fatalf("balance before the reversal race=%d", balanceOf("CAN-C"))
	}
	firstOut, secondOut := firstIn, secondIn
	outStarted := make(chan struct{})
	outResponses := make(chan *httptest.ResponseRecorder, 2)
	wg = sync.WaitGroup{}
	for _, id := range []int64{firstOut.ID, secondOut.ID} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-outStarted
			outResponses <- serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(id)+"/cancel", cancelBody(2, "并发冲销"), token)
		}(id)
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
	if success != 1 || conflicts != 1 || balanceOf("CAN-C") != 0 {
		t.Fatalf("concurrent insufficient reversal success=%d conflicts=%d balance=%d", success, conflicts, balanceOf("CAN-C"))
	}
	postedState := 0
	for _, id := range []int64{firstOut.ID, secondOut.ID} {
		if detail(id).Status == "POSTED" {
			postedState++
		}
	}
	if postedState != 1 {
		t.Fatal("a refused reversal did not leave its document posted")
	}
	// Only the winning cancellation reversed anything.
	if rows := reversalRows(t, db, products["CAN-C"], "REVERSAL"); len(rows) != 1 || rows[0].QuantityMilli != -2000 {
		t.Fatalf("reversal race rows=%+v", rows)
	}

	// A failing audit rolls back the reversal, the balance and the status.
	audited := post(createDraft(item("CAN-F", "6", "OPENING")).ID, 1)
	balanceBefore := balanceOf("CAN-F")
	entriesBefore := len(ledgerRows(t, db, products["CAN-F"]))
	cancelAudits = inventoryActionCount(t, db, "inventory.adjustment.cancel")
	installInventoryAuditFailure(t, db, dialect)
	inventoryData[any](t, cancel(audited.ID, 2, "失败冲销"), 500)
	removeInventoryAuditFailure(t, db, dialect)
	sameDraft(t, audited, detail(audited.ID))
	if balanceOf("CAN-F") != balanceBefore || len(ledgerRows(t, db, products["CAN-F"])) != entriesBefore || len(reversalRows(t, db, products["CAN-F"], "REVERSAL")) != 0 || inventoryActionCount(t, db, "inventory.adjustment.cancel") != cancelAudits {
		t.Fatal("failed audit left a partial reversal behind")
	}
	inventoryData[adjustmentDTO](t, cancel(audited.ID, 2, "重试冲销"), 200)
	if balanceOf("CAN-F") != 0 {
		t.Fatalf("retried cancellation balance=%d", balanceOf("CAN-F"))
	}

	// Every stored balance equals the sum of its original and reversal lines.
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
}
