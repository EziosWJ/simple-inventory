//go:build integration

package integration

import (
	"context"
	"encoding/json"
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
	"github.com/pressly/goose/v3"
)

func TestSQLiteInventoryDraftMaintenanceContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory-maintenance.db")
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	prepareInventorySchema9Upgrade(t, db, "sqlite3", filepath.Join(projectRoot(t), "migrations", "sqlite"))
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryMaintenanceContract(t, router, db, "sqlite")
}
func TestPostgresInventoryDraftMaintenanceContract(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	prepareInventorySchema9Upgrade(t, db, "postgres", filepath.Join(projectRoot(t), "migrations"))
	router, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryMaintenanceContract(t, router, db, "postgres")
}
func prepareInventorySchema9Upgrade(t *testing.T, db *platformdatabase.Database, dialect, root string) {
	t.Helper()
	if e := goose.SetDialect(dialect); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "schema"), 9); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_seed_db_version")
	if e := goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "seed"), 7); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_schema_db_version")
	// Persist an authentic #8 draft before #9 columns exist; migration must keep
	// its original metadata, quantities and references unchanged in both directions.
	for _, sql := range []string{
		"INSERT INTO product(code,name,type,unit) VALUES ('SCHEMA9-GOODS','旧版草稿商品','GOODS','台')",
		"INSERT INTO inventory_adjustment(document_no,status,version,created_by) VALUES ('SCHEMA9-DRAFT','DRAFT',1,1)",
		"INSERT INTO inventory_adjustment_item(adjustment_id,product_id,product_type,unit,quantity_milli,reason) SELECT a.id,p.id,'GOODS','台',1234,'OPENING' FROM inventory_adjustment a CROSS JOIN product p WHERE a.document_no='SCHEMA9-DRAFT' AND p.code='SCHEMA9-GOODS'",
	} {
		if e := db.GORM.Exec(sql).Error; e != nil {
			t.Fatal(e)
		}
	}
	schema := filepath.Join(root, "schema")
	for _, up := range []bool{true, false, true} {
		var e error
		if up {
			e = goose.UpToContext(context.Background(), db.SQL, schema, 11)
		} else {
			e = goose.DownToContext(context.Background(), db.SQL, schema, 10)
		}
		if e != nil {
			t.Fatalf("schema11 up=%v: %v", up, e)
		}
		for _, column := range []string{"cancelled_by", "cancelled_at", "cancel_reason"} {
			// Added by schema 10, so this column set survives the 11 → 10 rollback.
			if !db.GORM.Migrator().HasColumn("inventory_adjustment", column) {
				t.Fatalf("schema11 up=%v column %s is missing", up, column)
			}
		}
		for _, column := range []string{"posted_by", "posted_at"} {
			if got := db.GORM.Migrator().HasColumn("inventory_adjustment", column); got != up {
				t.Fatalf("schema11 up=%v column %s exists=%v", up, column, got)
			}
		}
		for _, table := range []string{"inventory_balance", "inventory_entry"} {
			if got := db.GORM.Migrator().HasTable(table); got != up {
				t.Fatalf("schema11 up=%v table %s exists=%v", up, table, got)
			}
		}
		var preserved int64
		if e := db.GORM.Table("inventory_adjustment a").Joins("JOIN inventory_adjustment_item i ON i.adjustment_id=a.id").Where("a.document_no='SCHEMA9-DRAFT' AND a.status='DRAFT' AND a.version=1 AND a.created_by=1 AND i.quantity_milli=1234 AND i.unit='台'").Count(&preserved).Error; e != nil || preserved != 1 {
			t.Fatalf("schema11 up=%v lost existing draft count=%d err=%v", up, preserved, e)
		}
	}
}
func editBody(version int64, items ...adjustmentItemInput) string {
	b, e := json.Marshal(struct {
		Version int64                 `json:"version"`
		Items   []adjustmentItemInput `json:"items"`
	}{version, items})
	if e != nil {
		panic(e)
	}
	return string(b)
}
func cancelBody(version int64, reason string) string {
	b, e := json.Marshal(struct {
		Version int64  `json:"version"`
		Reason  string `json:"reason"`
	}{version, reason})
	if e != nil {
		panic(e)
	}
	return string(b)
}
func inventoryActionCount(t *testing.T, db *platformdatabase.Database, action string) int64 {
	t.Helper()
	var count int64
	if e := db.GORM.Table("sys_oper_log").Where("module_name='inventory' AND operation_type=?", action).Count(&count).Error; e != nil {
		t.Fatal(e)
	}
	return count
}
func sameDraft(t *testing.T, before, after adjustmentDTO) {
	t.Helper()
	x, _ := json.Marshal(before)
	y, _ := json.Marshal(after)
	if string(x) != string(y) {
		t.Fatalf("failed operation changed draft:\nbefore=%s\nafter=%s", x, y)
	}
}
func sameCreation(t *testing.T, before, after adjustmentDTO) {
	t.Helper()
	if before.ID != after.ID || before.DocumentNo != after.DocumentNo || before.CreatedBy != after.CreatedBy || before.CreatedByName != after.CreatedByName || !before.CreateTime.Equal(after.CreateTime) {
		t.Fatalf("creation metadata overwritten: %+v -> %+v", before, after)
	}
}
func assertInventoryMaintenanceContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	oldPage := inventoryData[adjustmentPageDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"?documentNo=SCHEMA9-DRAFT", "", token), 200)
	if oldPage.Total != 1 {
		t.Fatalf("upgraded draft missing: %+v", oldPage)
	}
	old := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(oldPage.Records[0].ID), "", token), 200)
	if old.Version != 1 || old.Status != "DRAFT" || old.CancelledBy != nil || old.CancelledAt != nil || old.CancelReason != nil || len(old.Items) != 1 || old.Items[0].Quantity != "1.234" {
		t.Fatalf("upgraded detail=%+v", old)
	}
	pids := map[string]int64{}
	for _, p := range []struct{ code, kind, unit string }{{"EDIT-A", "GOODS", "台"}, {"EDIT-B", "GOODS", "支"}, {"EDIT-SERVICE", "SERVICE", "次"}, {"EDIT-DISABLED", "GOODS", "个"}} {
		v := inventoryData[struct {
			ID int64 `json:"id"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":%q,"name":%q,"type":%q,"unit":%q}`, p.code, p.code, p.kind, p.unit), token), 200)
		pids[p.code] = v.ID
	}
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(pids["EDIT-DISABLED"])+"/status", `{"status":0}`, token), 200)
	a := adjustmentItemInput{ProductID: pids["EDIT-A"], ProductType: "GOODS", Unit: "台", Quantity: "1.234", Reason: "OPENING"}
	b := adjustmentItemInput{ProductID: pids["EDIT-B"], ProductType: "GOODS", Unit: "支", Quantity: "-0.001", Reason: "DAMAGE"}
	create := func(items ...adjustmentItemInput) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(items...), token), 200)
	}
	detail := func(id int64) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(id), "", token), 200)
	}
	draft := create(a, b)
	path := adjustmentPath + "/" + itoa(draft.ID)
	assertUnauthenticated(t, serveJSON(router, http.MethodPut, path, editBody(1, a), ""))
	assertUnauthenticated(t, serveJSON(router, http.MethodPost, path+"/cancel", cancelBody(1, "取消"), ""))
	editedItem := b
	editedItem.Quantity = "-2.500"
	editedItem.Reason = "OTHER"
	editedItem.Remark = "重新核对"
	beforeEdit := inventoryActionCount(t, db, "inventory.adjustment.edit")
	// Attempts to send creation metadata are ignored; the writable DTO contains
	// only a confirmed version and the complete replacement item collection.
	body := strings.TrimSuffix(editBody(1, editedItem), "}") + `,"documentNo":"FORGED","createdBy":99999,"createTime":"2000-01-01T00:00:00Z","status":"POSTED"}`
	edited := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPut, path, body, token), 200)
	sameCreation(t, draft, edited)
	if edited.Status != "DRAFT" || edited.Version != 2 || len(edited.Items) != 1 || edited.Items[0].ProductID != b.ProductID || edited.Items[0].Quantity != "-2.5" || edited.Items[0].Reason != "OTHER" || edited.Items[0].Remark != "重新核对" || edited.CancelledBy != nil || inventoryActionCount(t, db, "inventory.adjustment.edit") != beforeEdit+1 {
		t.Fatalf("edited draft=%+v", edited)
	}
	sameDraft(t, edited, detail(draft.ID))
	inventoryData[any](t, serveJSON(router, http.MethodPut, path, editBody(1, a), token), 409)
	inventoryData[any](t, serveJSON(router, http.MethodPost, path+"/cancel", cancelBody(1, "旧内容取消"), token), 409)
	sameDraft(t, edited, detail(draft.ID))
	for _, body := range []string{`{}`, `{"version":0,"items":[]}`, editBody(-1, a), editBody(9223372036854775807, a), editBody(2), editBody(2, a, a), `{"version":"2","items":[]}`, `{"version":2.5,"items":[]}`, `{"version":9223372036854775808,"items":[]}`} {
		inventoryData[any](t, serveJSON(router, http.MethodPut, path, body, token), 400)
	}
	for _, change := range []func(*adjustmentItemInput){func(x *adjustmentItemInput) { x.Quantity = "0" }, func(x *adjustmentItemInput) { x.Quantity = "1.0000" }, func(x *adjustmentItemInput) { x.Quantity = "9223372036854775.808" }, func(x *adjustmentItemInput) { x.Reason = "SHORTAGE" }, func(x *adjustmentItemInput) { x.Reason = "OTHER"; x.Remark = " " }, func(x *adjustmentItemInput) { x.Unit = "件" }, func(x *adjustmentItemInput) { x.ProductType = "SERVICE" }, func(x *adjustmentItemInput) { x.ProductID = 999999 }, func(x *adjustmentItemInput) { x.ProductID = pids["EDIT-SERVICE"]; x.Unit = "次" }, func(x *adjustmentItemInput) { x.ProductID = pids["EDIT-DISABLED"]; x.Unit = "个" }} {
		item := a
		change(&item)
		inventoryData[any](t, serveJSON(router, http.MethodPut, path, editBody(2, item), token), 400)
		sameDraft(t, edited, detail(draft.ID))
	}
	for _, body := range []string{`{}`, cancelBody(0, "取消"), cancelBody(-1, "取消"), cancelBody(9223372036854775807, "取消"), cancelBody(2, ""), cancelBody(2, " \n\t "), cancelBody(2, strings.Repeat("长", 501)), `{"version":2,"reason":1}`} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, path+"/cancel", body, token), 400)
	}
	for _, id := range []string{"0", "-1", "bad", "9223372036854775808"} {
		inventoryData[any](t, serveJSON(router, http.MethodPut, adjustmentPath+"/"+id, editBody(1, a), token), 400)
		inventoryData[any](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+id+"/cancel", cancelBody(1, "取消"), token), 400)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPut, adjustmentPath+"/999999", editBody(1, a), token), 404)
	inventoryData[any](t, serveJSON(router, http.MethodPost, adjustmentPath+"/999999/cancel", cancelBody(1, "取消"), token), 404)
	// Eight editors holding the same displayed version produce one committed edit.
	concurrent := create(a, b)
	concurrentPath := adjustmentPath + "/" + itoa(concurrent.ID)
	editsBefore := inventoryActionCount(t, db, "inventory.adjustment.edit")
	responses := make(chan *httptest.ResponseRecorder, 8)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			item := a
			item.Quantity = fmt.Sprintf("%d.001", i+10)
			responses <- serveJSON(router, http.MethodPut, concurrentPath, editBody(1, item, b), token)
		}(i)
	}
	close(start)
	wg.Wait()
	close(responses)
	success, conflicts := 0, 0
	var winner adjustmentDTO
	for response := range responses {
		if response.Code == 200 {
			winner = inventoryData[adjustmentDTO](t, response, 200)
			success++
		} else {
			inventoryData[any](t, response, 409)
			conflicts++
		}
	}
	if success != 1 || conflicts != 7 || winner.Version != 2 || inventoryActionCount(t, db, "inventory.adjustment.edit") != editsBefore+1 {
		t.Fatalf("concurrent edit success=%d conflicts=%d winner=%+v", success, conflicts, winner)
	}
	sameDraft(t, winner, detail(concurrent.ID))
	sameCreation(t, concurrent, winner)
	// Competing edit and cancel must also compare the version at the actual write.
	race := create(a, b)
	racePath := adjustmentPath + "/" + itoa(race.ID)
	editCount := inventoryActionCount(t, db, "inventory.adjustment.edit")
	cancelCount := inventoryActionCount(t, db, "inventory.adjustment.cancel")
	raceResponses := make(chan *httptest.ResponseRecorder, 2)
	raceStart := make(chan struct{})
	wg = sync.WaitGroup{}
	for _, cancel := range []bool{false, true} {
		wg.Add(1)
		go func(cancel bool) {
			defer wg.Done()
			<-raceStart
			if cancel {
				raceResponses <- serveJSON(router, http.MethodPost, racePath+"/cancel", cancelBody(1, "竞争取消"), token)
			} else {
				item := a
				item.Quantity = "10"
				raceResponses <- serveJSON(router, http.MethodPut, racePath, editBody(1, item), token)
			}
		}(cancel)
	}
	close(raceStart)
	wg.Wait()
	close(raceResponses)
	success, conflicts = 0, 0
	for response := range raceResponses {
		if response.Code == 200 {
			inventoryData[adjustmentDTO](t, response, 200)
			success++
		} else {
			inventoryData[any](t, response, 409)
			conflicts++
		}
	}
	finalRace := detail(race.ID)
	if success != 1 || conflicts != 1 || finalRace.Version != 2 || (inventoryActionCount(t, db, "inventory.adjustment.edit")-editCount)+(inventoryActionCount(t, db, "inventory.adjustment.cancel")-cancelCount) != 1 {
		t.Fatalf("edit-cancel race success=%d conflicts=%d final=%+v", success, conflicts, finalRace)
	}
	if finalRace.Status == "CANCELLED" {
		if len(finalRace.Items) != 2 || finalRace.Items[0].Quantity != a.Quantity {
			t.Fatalf("cancel saw unconfirmed edit=%+v", finalRace)
		}
	} else if finalRace.Status != "DRAFT" || len(finalRace.Items) != 1 || finalRace.Items[0].Quantity != "10" {
		t.Fatalf("race final=%+v", finalRace)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPost, racePath+"/cancel", cancelBody(1, "未确认的取消"), token), 409)
	// Failed audit rolls back version, item replacement and cancellation metadata.
	rollback := create(a, b)
	rollbackPath := adjustmentPath + "/" + itoa(rollback.ID)
	editCount = inventoryActionCount(t, db, "inventory.adjustment.edit")
	cancelCount = inventoryActionCount(t, db, "inventory.adjustment.cancel")
	installInventoryAuditFailure(t, db, dialect)
	inventoryData[any](t, serveJSON(router, http.MethodPut, rollbackPath, editBody(1, editedItem), token), 500)
	sameDraft(t, rollback, detail(rollback.ID))
	inventoryData[any](t, serveJSON(router, http.MethodPost, rollbackPath+"/cancel", cancelBody(1, "失败取消"), token), 500)
	sameDraft(t, rollback, detail(rollback.ID))
	if inventoryActionCount(t, db, "inventory.adjustment.edit") != editCount || inventoryActionCount(t, db, "inventory.adjustment.cancel") != cancelCount {
		t.Fatal("failed audit left edit/cancel logs")
	}
	removeInventoryAuditFailure(t, db, dialect)
	recovered := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPut, rollbackPath, editBody(1, editedItem), token), 200)
	if recovered.Version != 2 {
		t.Fatalf("audit recovery=%+v", recovered)
	}
	inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, rollbackPath+"/cancel", cancelBody(2, "恢复后取消"), token), 200)
	// Type/unit edits are still allowed before inventory postings. Reconfirming
	// the new unit permits draft edits, but an old confirmed unit is rejected.
	changeDraft := create(a)
	productPath := "/api/v1/products/" + itoa(a.ProductID)
	inventoryData[any](t, serveJSON(router, http.MethodPut, productPath, `{"name":"更改单位","type":"GOODS","unit":"件"}`, token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPut, adjustmentPath+"/"+itoa(changeDraft.ID), editBody(1, a), token), 400)
	unchanged := detail(changeDraft.ID)
	if unchanged.Version != 1 || unchanged.Items[0].Unit != "台" {
		t.Fatalf("old unit validation changed draft=%+v", unchanged)
	}
	confirmed := a
	confirmed.Unit = "件"
	confirmed.Quantity = "3.001"
	updated := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPut, adjustmentPath+"/"+itoa(changeDraft.ID), editBody(1, confirmed), token), 200)
	if updated.Version != 2 || updated.Items[0].Unit != "件" || updated.Items[0].Quantity != "3.001" {
		t.Fatalf("reconfirmed unit=%+v", updated)
	}
	// Disabled/changed products never prevent cancelling an obsolete draft.
	inventoryData[any](t, serveJSON(router, http.MethodPut, productPath, `{"name":"改为服务","type":"SERVICE","unit":"次"}`, token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPut, productPath+"/status", `{"status":0}`, token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPut, adjustmentPath+"/"+itoa(changeDraft.ID), editBody(2, confirmed), token), 400)
	// A different logged-in user cancels the original creator's draft.
	inventoryData[any](t, serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"draft-canceller","nickname":"取消操作员","deptId":1,"status":1}`, token), 200)
	userPage := inventoryData[struct {
		Records []struct {
			ID int64 `json:"id"`
		} `json:"records"`
	}](t, serveJSON(router, http.MethodGet, "/api/system/user/page?username=draft-canceller", "", token), 200)
	if len(userPage.Records) != 1 {
		t.Fatal("cancel actor user missing")
	}
	actor := userPage.Records[0].ID
	otherToken := loginUser(t, router, "draft-canceller", "admin123")
	cancelCount = inventoryActionCount(t, db, "inventory.adjustment.cancel")
	cancelled := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, path+"/cancel", cancelBody(2, "  放弃这张草稿  "), otherToken), 200)
	sameCreation(t, draft, cancelled)
	if cancelled.Status != "CANCELLED" || cancelled.Version != 3 || cancelled.CancelledBy == nil || *cancelled.CancelledBy != actor || cancelled.CancelledByName != "取消操作员" || cancelled.CancelledAt == nil || cancelled.CancelledAt.IsZero() || cancelled.CancelReason == nil || *cancelled.CancelReason != "放弃这张草稿" || len(cancelled.Items) != 1 || cancelled.Items[0].Quantity != "-2.5" {
		t.Fatalf("cancellation record=%+v", cancelled)
	}
	sameDraft(t, cancelled, detail(draft.ID))
	for _, version := range []int64{2, 3} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, path+"/cancel", cancelBody(version, "重复取消"), otherToken), 409)
		inventoryData[any](t, serveJSON(router, http.MethodPut, path, editBody(version, b), otherToken), 409)
	}
	if inventoryActionCount(t, db, "inventory.adjustment.cancel") != cancelCount+1 {
		t.Fatal("duplicate cancellation wrote extra audit")
	}
	var actorLogs int64
	if e := db.GORM.Table("sys_oper_log").Where("module_name='inventory' AND operation_type='inventory.adjustment.cancel' AND operator_id=?", actor).Count(&actorLogs).Error; e != nil || actorLogs != 1 {
		t.Fatalf("actual cancellation actor audit=%d err=%v", actorLogs, e)
	}
	cancelledChange := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(changeDraft.ID)+"/cancel", cancelBody(2, "商品已改变，不再使用"), otherToken), 200)
	if cancelledChange.Version != 3 || cancelledChange.Status != "CANCELLED" || cancelledChange.Items[0].Unit != "件" {
		t.Fatalf("obsolete-product draft cancellation=%+v", cancelledChange)
	}
	cancelledPage := inventoryData[adjustmentPageDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"?status=CANCELLED&documentNo="+cancelled.DocumentNo, "", token), 200)
	if cancelledPage.Total != 1 || cancelledPage.Records[0].CancelledBy == nil || *cancelledPage.Records[0].CancelledBy != actor || cancelledPage.Records[0].CancelReason == nil {
		t.Fatalf("cancelled state query=%+v", cancelledPage)
	}
	// A posted document is locked against edits and version-stale operations.
	// Posting itself is covered by the posting contract test; here the real
	// endpoint is used because the schema rejects a status written alone.
	postProduct := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"EDIT-POSTED","name":"已过账商品","type":"GOODS","unit":"台"}`, token), 200)
	posted := create(adjustmentItemInput{ProductID: postProduct.ID, ProductType: "GOODS", Unit: "台", Quantity: "2", Reason: "OPENING"})
	postedPath := adjustmentPath + "/" + itoa(posted.ID)
	postedResult := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, postedPath+"/post", `{"version":1}`, token), 200)
	if postedResult.Status != "POSTED" || postedResult.Version != 2 {
		t.Fatalf("posted document=%+v", postedResult)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPut, postedPath, editBody(2, b), token), 409)
	inventoryData[any](t, serveJSON(router, http.MethodPost, postedPath+"/post", `{"version":2}`, token), 409)
	// A posted document cannot be cancelled through the draft-cancel path at any
	// version: only the confirmed version matches a state, and it is POSTED.
	// Reversing it is the posted-cancel task's concern.
	for _, version := range []int64{1, 2, 3} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, postedPath+"/cancel", cancelBody(version, "已过账不可仅取消"), token), 409)
	}
	sameDraft(t, postedResult, detail(posted.ID))
	var entries int64
	if e := db.GORM.Table("inventory_entry").Where("adjustment_id=?", posted.ID).Count(&entries).Error; e != nil || entries != 1 {
		t.Fatalf("posted document ledger entries=%d err=%v", entries, e)
	}
	inventoryData[any](t, serveJSON(router, http.MethodDelete, path, "", token), 404)
	// A cancelled document cannot be posted either, whatever version is claimed.
	for _, version := range []int64{3, 4} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, path+"/post", fmt.Sprintf(`{"version":%d}`, version), token), 409)
	}
}
