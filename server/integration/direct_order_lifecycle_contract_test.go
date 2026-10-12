//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/directorder"
	"github.com/EziosWJ/simple-inventory/server/internal/inventory"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
	"github.com/EziosWJ/simple-inventory/server/internal/salereturn"
)

func TestSQLiteDirectOrderLifecycleHTTPContract(t *testing.T) {
	assertDirectOrderLifecycle(t, "sqlite")
}
func TestPostgresDirectOrderLifecycleHTTPContract(t *testing.T) {
	assertDirectOrderLifecycle(t, "postgres")
}
func assertDirectOrderLifecycle(t *testing.T, dialect string) {
	db, _ := directDraftDatabase(t, dialect)
	defer db.Close()
	r := directDraftRouter(t, db, dialect)
	token := loginAdmin(t, r)
	partner := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"LIFE-P","name":"直送交易方","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	other := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"LIFE-P2","name":"第二交易方","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	goods := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"LIFE-G","name":"历史直送商品","type":"GOODS","unit":"台"}`, token), 200).ID
	service := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"LIFE-S","name":"安装服务","type":"SERVICE","unit":"次"}`, token), 200).ID
	create := func(key string, supplier, customer int64, price string) directorder.Draft {
		t.Helper()
		return inventoryData[directorder.Draft](t, serveJSON(r, "POST", "/api/v1/direct-orders", fmt.Sprintf(`{"requestKey":%q,"supplierId":%d,"customerId":%d,"businessDate":"2026-10-01","deliveryContact":"","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","purchasePrice":%q,"salePrice":%q},{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","salePrice":%q}]}`, key, supplier, customer, goods, price, price, service, price), token), 200)
	}
	post := func(d directorder.Draft, code int) directorder.Draft {
		t.Helper()
		return inventoryData[directorder.Draft](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/direct-orders/%d/post", d.ID), fmt.Sprintf(`{"version":%d}`, d.Version), token), code)
	}
	cancel := func(d directorder.Draft, code int) directorder.Draft {
		t.Helper()
		return inventoryData[directorder.Draft](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/direct-orders/%d/cancel", d.ID), fmt.Sprintf(`{"version":%d,"reason":"实际情况变化"}`, d.Version), token), code)
	}
	stock := func() int64 {
		t.Helper()
		var qty int64
		if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", goods).Scan(&qty).Error; e != nil {
			t.Fatal(e)
		}
		return qty
	}
	countChildren := func(id int64) int64 {
		t.Helper()
		var p, s int64
		db.GORM.Table("purchase_document").Where("source_order_id=?", id).Count(&p)
		db.GORM.Table("sale_document").Where("source_order_id=?", id).Count(&s)
		return p + s
	}
	// Fail after the purchase side and at the final coordinator audit: no child or ledger commits.
	for i, action := range []string{"sale.post", "directorder.post"} {
		d := create(fmt.Sprintf("rollback-%d", i), partner, partner, "1.00")
		installRedesignAuditFailure(t, db, dialect, action)
		post(d, 500)
		dropRedesignAuditFailure(t, db, dialect)
		after := inventoryData[directorder.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/direct-orders/%d", d.ID), "", token), 200)
		if after.Status != "DRAFT" || after.Version != 1 || countChildren(d.ID) != 0 || stock() != 0 {
			t.Fatal("failed confirmation partially committed")
		}
	}
	d := create("life-post", partner, partner, "10.00")
	posted := post(d, 200)
	if posted.Status != "POSTED" || posted.Version != 2 || posted.PurchaseID == nil || posted.SaleID == nil || posted.PurchaseTotal != "20.00" || posted.SaleTotal != "30.00" || stock() != 0 {
		t.Fatalf("bad coordinated posting: %+v", posted)
	}
	p := inventoryData[purchase.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/purchases/%d", *posted.PurchaseID), "", token), 200)
	s := inventoryData[sale.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d", *posted.SaleID), "", token), 200)
	if p.Status != "POSTED" || s.Status != "POSTED" || p.SourceOrderID == nil || s.SourceOrderID == nil || *p.SourceOrderID != d.ID || *s.SourceOrderID != d.ID || p.PostedAt == nil || s.PostedAt == nil || !p.PostedAt.Equal(*s.PostedAt) || !p.PostedAt.Equal(*posted.PostedAt) || s.DeliveryContact == nil || *s.DeliveryContact != "" {
		t.Fatal("generated state/time/snapshot differs")
	}
	entries := inventoryData[inventory.EntryPage](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/inventory/entries?productId=%d", goods), "", token), 200)
	if entries.Total != 2 {
		t.Fatalf("ledger entries=%d", entries.Total)
	}
	for _, entry := range entries.Records {
		if !entry.DirectDelivery || !entry.OccurredAt.Equal(*posted.PostedAt) {
			t.Fatal("direct ledger time/marker missing")
		}
	}
	for _, url := range []string{fmt.Sprintf("/api/v1/purchases/%d/cancel", p.ID), fmt.Sprintf("/api/v1/sales/%d/cancel", s.ID)} {
		inventoryData[any](t, serveJSON(r, "POST", url, `{"version":2,"reason":"独立取消"}`, token), 409)
	}
	replay := post(d, 200)
	if replay.PurchaseID == nil || *replay.PurchaseID != p.ID || countChildren(d.ID) != 2 {
		t.Fatal("retry created duplicate children")
	}
	// Customer print projection excludes purchasing values and retains explicit empty delivery fields.
	note := inventoryData[sale.DeliveryNote](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d/delivery-note", s.ID), "", token), 200)
	if note.TotalAmount != "30.00" || len(note.Items) != 2 || note.DeliveryContact == nil || *note.DeliveryContact != "" {
		t.Fatal("delivery note incorrect")
	}
	inventoryData[any](t, serveJSON(r, "PUT", fmt.Sprintf("/api/v1/products/%d", goods), `{"code":"LIFE-G","name":"当前改名商品","type":"GOODS","unit":"台"}`, token), 200)
	old := inventoryData[directorder.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/direct-orders/%d", posted.ID), "", token), 200)
	if old.Items[0].ProductName != "历史直送商品" {
		t.Fatal("direct document lost snapshot")
	}
	// Cancel after actual collection/payment preserves money and produces refundable balances.
	inventoryData[any](t, serveJSON(r, "POST", "/api/v1/partner-balances/settlements", fmt.Sprintf(`{"requestKey":"life-collect","partnerId":%d,"direction":"CUSTOMER","amount":"30.00","businessDate":"2026-10-01","paymentMethod":"CASH"}`, partner), token), 200)
	inventoryData[any](t, serveJSON(r, "POST", "/api/v1/partner-balances/settlements", fmt.Sprintf(`{"requestKey":"life-pay","partnerId":%d,"direction":"SUPPLIER","amount":"20.00","businessDate":"2026-10-01","paymentMethod":"CASH"}`, partner), token), 200)
	installRedesignAuditFailure(t, db, dialect, "purchase.draft.cancel")
	cancel(posted, 500)
	dropRedesignAuditFailure(t, db, dialect)
	kept := inventoryData[sale.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d", s.ID), "", token), 200)
	if kept.Status != "POSTED" || stock() != 0 {
		t.Fatal("failed cancel partially reversed sale")
	}
	cancelled := cancel(posted, 200)
	cancel(posted, 200)
	if cancelled.Status != "CANCELLED" || stock() != 0 {
		t.Fatal("cancel not net zero")
	}
	for _, want := range []struct {
		Direction string
		Amount    int64
	}{{"CUSTOMER", -3000}, {"SUPPLIER", -2000}} {
		var amount int64
		if e := db.GORM.Table("partner_balance").Where("partner_id=? AND direction=?", partner, want.Direction).Pluck("amount_cents", &amount).Error; e != nil || amount != want.Amount {
			t.Fatalf("cancel balance %s=%d err=%v", want.Direction, amount, e)
		}
	}
	// Zero amount, concurrent original-version confirmation/cancellation, and independent new document.
	zero := create("zero-direct", other, other, "0.00")
	z := post(zero, 200)
	var financial int64
	db.GORM.Table("partner_balance_entry").Where("purchase_id=? OR sale_id=?", *z.PurchaseID, *z.SaleID).Count(&financial)
	if financial != 0 {
		t.Fatal("zero direct wrote financial ledger")
	}
	cancel(z, 200)
	concurrent := create("concurrent-direct", other, other, "1.00")
	assertConcurrentDirectAction(t, r, token, concurrent, "post", 200)
	current := inventoryData[directorder.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/direct-orders/%d", concurrent.ID), "", token), 200)
	if countChildren(concurrent.ID) != 2 {
		t.Fatal("concurrent duplicate children")
	}
	assertConcurrentDirectAction(t, r, token, current, "cancel", 200)
	a := create("cross-a", partner, other, "1.00")
	b := create("cross-b", other, partner, "1.00")
	var wg sync.WaitGroup
	responses := make(chan int, 2)
	for _, x := range []directorder.Draft{a, b} {
		wg.Add(1)
		go func(d directorder.Draft) {
			defer wg.Done()
			responses <- serveJSON(r, "POST", fmt.Sprintf("/api/v1/direct-orders/%d/post", d.ID), `{"version":1}`, token).Code
		}(x)
	}
	wg.Wait()
	close(responses)
	for code := range responses {
		if code != 200 {
			t.Fatalf("cross-partner posting failed: %d", code)
		}
	}
	// Returning into the shop and then to the supplier uses the existing return paths.
	returned := post(create("returned-direct", other, other, "2.00"), 200)
	rs := inventoryData[sale.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d", *returned.SaleID), "", token), 200)
	sr := inventoryData[salereturn.Document](t, serveJSON(r, "POST", "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-02","items":[{"saleItemId":%d,"quantity":"1"}]}`, rs.ID, rs.Items[0].ID), token), 200)
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/sale-returns/%d/post", sr.ID), `{"version":1}`, token), 200)
	cancel(returned, 409)
	rp := inventoryData[purchase.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/purchases/%d", *returned.PurchaseID), "", token), 200)
	pr := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-02","items":[{"purchaseItemId":%d,"quantity":"1"}]}`, rp.ID, rp.Items[0].ID), token), 200)
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchase-returns/%d/post", pr.ID), `{"version":1}`, token), 200)
	cancel(returned, 409)
	detail := inventoryData[directorder.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/direct-orders/%d", returned.ID), "", token), 200)
	if len(detail.ActiveSaleReturns) != 1 || len(detail.ActivePurchaseReturns) != 1 {
		t.Fatal("return blockers not exposed")
	}
	draft := create("cancel-draft", partner, partner, "1.00")
	cancel(draft, 200)
	if countChildren(draft.ID) != 0 {
		t.Fatal("draft cancel generated children")
	}
	// No independent managed legacy link can be created, even with the known purchase ID.
	inventoryData[any](t, serveJSON(r, "POST", "/api/v1/sales", fmt.Sprintf(`{"directDelivery":true,"directPurchaseId":%d,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"1"}]}`, rp.ID, partner, goods), token), 409)
}
func assertConcurrentDirectAction(t *testing.T, r http.Handler, token string, d directorder.Draft, action string, want int) {
	t.Helper()
	var wg sync.WaitGroup
	responses := make(chan struct {
		Code int
		Body string
	}, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := fmt.Sprintf(`{"version":%d,"reason":"并发取消"}`, d.Version)
			res := serveJSON(r, "POST", fmt.Sprintf("/api/v1/direct-orders/%d/%s", d.ID, action), body, token)
			responses <- struct {
				Code int
				Body string
			}{res.Code, res.Body.String()}
		}()
	}
	wg.Wait()
	close(responses)
	for res := range responses {
		if res.Code != want {
			t.Fatalf("concurrent %s=%d: %s", action, res.Code, res.Body)
		}
	}
}
func installRedesignAuditFailure(t *testing.T, db *platformdatabase.Database, dialect, action string) {
	t.Helper()
	statement := fmt.Sprintf(`CREATE TRIGGER redesign_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='%s' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`, action)
	if dialect == "postgres" {
		statement = fmt.Sprintf(`CREATE FUNCTION reject_redesign_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='%s' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`, action)
	}
	if e := db.GORM.Exec(statement).Error; e != nil {
		t.Fatal(e)
	}
	if dialect == "postgres" {
		if e := db.GORM.Exec(`CREATE TRIGGER redesign_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_redesign_audit()`).Error; e != nil {
			t.Fatal(e)
		}
	}
}
func dropRedesignAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	statement := "DROP TRIGGER redesign_audit_failure"
	if dialect == "postgres" {
		statement += " ON sys_oper_log"
	}
	if e := db.GORM.Exec(statement).Error; e != nil {
		t.Fatal(e)
	}
	if dialect == "postgres" {
		if e := db.GORM.Exec("DROP FUNCTION reject_redesign_audit()").Error; e != nil {
			t.Fatal(e)
		}
	}
}
