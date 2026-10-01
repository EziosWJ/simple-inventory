//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/purchasereturn"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
	"github.com/EziosWJ/simple-inventory/server/internal/salereturn"
)

func TestSQLiteDirectDeliveryPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "direct.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertDirectDeliveryContract(t, router)
}
func TestPostgresDirectDeliveryPostgresSQLiteHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertDirectDeliveryContract(t, router)
}
func assertDirectDeliveryContract(t *testing.T, r http.Handler) {
	token := loginAdmin(t, r)
	assertUnauthenticated(t, serveJSON(r, "POST", "/api/v1/sales", `{}`, ""))
	p := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"DIRECT-P","name":"直送往来","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	g := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"DIRECT-G","name":"直送商品","type":"GOODS","unit":"台"}`, token), 200).ID
	svc := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"DIRECT-S","name":"安装","type":"SERVICE","unit":"次"}`, token), 200).ID
	pi := inventoryData[purchase.Draft](t, serveJSON(r, "POST", "/api/v1/purchases", fmt.Sprintf(`{"directDelivery":true,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"10.00"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"20.00"}]}`, p, g, g), token), 200)
	saleBody := fmt.Sprintf(`{"directDelivery":true,"directPurchaseId":%d,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"3","unitPrice":"30.00"},{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","unitPrice":"5.00"}]}`, pi.ID, p, g, svc)
	so := inventoryData[sale.Draft](t, serveJSON(r, "POST", "/api/v1/sales", saleBody, token), 200)
	expect := func(method, url, body string, code int) {
		t.Helper()
		x := serveJSON(r, method, url, body, token)
		if x.Code != code {
			t.Fatalf("%s %s=%d expected %d: %s", method, url, x.Code, code, x.Body.String())
		}
	}
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":1}`, 409)
	expect("POST", "/api/v1/sales", saleBody, 409)
	expect("POST", fmt.Sprintf("/api/v1/purchases/%d/cancel", pi.ID), `{"version":1,"reason":"不能先取消"}`, 409)
	expect("POST", fmt.Sprintf("/api/v1/purchases/%d/post", pi.ID), `{"version":1}`, 200)
	// A changed linked draft cannot bypass per-product quantity validation.
	mismatch := strings.Replace(saleBody, `"quantity":"3"`, `"quantity":"2"`, 1)
	mismatch = `{"version":1,` + strings.TrimPrefix(mismatch, "{")
	expect("PUT", fmt.Sprintf("/api/v1/sales/%d", so.ID), mismatch, 200)
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":2}`, 409)
	fixed := `{"version":2,` + strings.TrimPrefix(saleBody, "{")
	expect("PUT", fmt.Sprintf("/api/v1/sales/%d", so.ID), fixed, 200)
	so = inventoryData[sale.Draft](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":3}`, token), 200)
	expect("PUT", fmt.Sprintf("/api/v1/partners/%d", p), `{"code":"DIRECT-P","name":"改名后的往来","type":"COMPANY","isCustomer":true,"isSupplier":true}`, 200)
	snapshot := inventoryData[purchase.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/purchases/%d", pi.ID), "", token), 200)
	if len(snapshot.DirectDocuments) != 1 || snapshot.DirectDocuments[0].PartnerName != "直送往来" {
		t.Fatalf("posted association lost partner snapshot: %+v", snapshot.DirectDocuments)
	}
	assertDirectReturnPath(t, r, token, p, g, pi, so)
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":1}`, 409)
	expect("POST", fmt.Sprintf("/api/v1/purchases/%d/cancel", pi.ID), `{"version":2,"reason":"不能先取消"}`, 409)
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/cancel", so.ID), `{"version":4,"reason":"录错"}`, 200)
	replacement := inventoryData[sale.Draft](t, serveJSON(r, "POST", "/api/v1/sales", saleBody, token), 200)
	got := inventoryData[purchase.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/purchases/%d", pi.ID), "", token), 200)
	if len(got.DirectDocuments) != 2 || got.DirectDocuments[0].Status != "CANCELLED" || got.DirectDocuments[0].PartnerName != "直送往来" || got.DirectDocuments[1].PartnerName != "改名后的往来" {
		t.Fatalf("history lost: %+v", got.DirectDocuments)
	}
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/cancel", replacement.ID), `{"version":1,"reason":"取消草稿"}`, 200)
	expect("POST", fmt.Sprintf("/api/v1/purchases/%d/cancel", pi.ID), `{"version":2,"reason":"录错"}`, 200)
	assertDirectConcurrentAssociations(t, r, token, p, g)
}

func assertDirectReturnPath(t *testing.T, r http.Handler, token string, partner, goods int64, pi purchase.Draft, so sale.Draft) {
	t.Helper()
	expect := func(url, body string, code int) {
		t.Helper()
		x := serveJSON(r, "POST", url, body, token)
		if x.Code != code {
			t.Fatalf("%s=%d expected %d: %s", url, x.Code, code, x.Body.String())
		}
	}
	// The supplier side initially fails without changing the already posted originals.
	pr := inventoryData[purchasereturn.Document](t, serveJSON(r, "POST", "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"0.5"}]}`, pi.ID, pi.Items[1].ID), token), 200)
	expect(fmt.Sprintf("/api/v1/purchase-returns/%d/post", pr.ID), `{"version":1}`, 409)
	sr := inventoryData[salereturn.Document](t, serveJSON(r, "POST", "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"0.5"}]}`, so.ID, so.Items[0].ID), token), 200)
	sr = inventoryData[salereturn.Document](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/sale-returns/%d/post", sr.ID), `{"version":1}`, token), 200)
	if sr.BusinessDate != "2026-10-01" || sr.TotalAmount != "15.00" {
		t.Fatalf("sale return own price lost: %+v", sr)
	}
	stock := inventoryData[struct {
		Records []struct {
			ProductID int64
			Quantity  string
		}
	}](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/inventory/balances?productId=%d&quantityMode=ALL", goods), "", token), 200)
	found := false
	for _, x := range stock.Records {
		if x.ProductID == goods && x.Quantity == "0.5" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sale return stock interim=%+v", stock)
	}
	pr = inventoryData[purchasereturn.Document](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchase-returns/%d/post", pr.ID), `{"version":1}`, token), 200)
	if pr.BusinessDate != "2026-10-01" || pr.TotalAmount != "10.00" || pr.DirectTrace == nil || len(pr.DirectTrace.SaleReturns) != 1 || pr.DirectTrace.SaleReturns[0].TotalAmount != "15.00" || pr.DirectTrace.PurchaseReturns[0].TotalAmount != "10.00" || pr.DirectTrace.SaleReturns[0].Items[0].Quantity != "0.5" || pr.DirectTrace.SaleReturns[0].PostedByName == "" {
		t.Fatalf("two sides trace=%+v", pr.DirectTrace)
	}
	for _, doc := range append(append(pr.DirectTrace.Sales, pr.DirectTrace.SaleReturns...), pr.DirectTrace.PurchaseReturns...) {
		if doc.BusinessDate != "2026-10-01" {
			t.Fatalf("trace business date=%q", doc.BusinessDate)
		}
	}
	for _, url := range []string{fmt.Sprintf("/api/v1/purchases/%d", pi.ID), fmt.Sprintf("/api/v1/sales/%d", so.ID), fmt.Sprintf("/api/v1/sale-returns/%d", sr.ID), fmt.Sprintf("/api/v1/purchase-returns/%d", pr.ID), fmt.Sprintf("/api/v1/sale-returns/source/%d", so.ID), fmt.Sprintf("/api/v1/purchase-returns/source/%d", pi.ID)} {
		x := inventoryData[struct{ DirectTrace json.RawMessage }](t, serveJSON(r, "GET", url, "", token), 200)
		if len(x.DirectTrace) < 10 {
			t.Fatalf("missing trace: %s %s", url, x.DirectTrace)
		}
	}
	for direction, amount := range map[string]string{"CUSTOMER": "80.00", "SUPPLIER": "40.00"} {
		x := inventoryData[struct{ Records []struct{ Amount string } }](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances?partnerId=%d&direction=%s", partner, direction), "", token), 200)
		if len(x.Records) != 1 || x.Records[0].Amount != amount {
			t.Fatalf("%s independent balance=%+v", direction, x)
		}
	}
	// A second partial return uses each side's own price and original line quota.
	sr2 := inventoryData[salereturn.Document](t, serveJSON(r, "POST", "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"0.25"}]}`, so.ID, so.Items[0].ID), token), 200)
	sr2 = inventoryData[salereturn.Document](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/sale-returns/%d/post", sr2.ID), `{"version":1}`, token), 200)
	pr2 := inventoryData[purchasereturn.Document](t, serveJSON(r, "POST", "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"0.25"}]}`, pi.ID, pi.Items[1].ID), token), 200)
	pr2 = inventoryData[purchasereturn.Document](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchase-returns/%d/post", pr2.ID), `{"version":1}`, token), 200)
	if sr2.TotalAmount != "7.50" || pr2.TotalAmount != "5.00" {
		t.Fatalf("second partial return price: sale=%s purchase=%s", sr2.TotalAmount, pr2.TotalAmount)
	}
	// The earlier return cannot be reversed while a later return consumes the same line.
	expect(fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", pr.ID), `{"version":2,"reason":"应先取消后续退货"}`, 409)
	expect(fmt.Sprintf("/api/v1/sale-returns/%d/cancel", sr.ID), `{"version":2,"reason":"应先取消后续退货"}`, 409)
	quota := inventoryData[purchasereturn.Document](t, serveJSON(r, "POST", "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"2"}]}`, pi.ID, pi.Items[1].ID), token), 200)
	expect(fmt.Sprintf("/api/v1/purchase-returns/%d/post", quota.ID), `{"version":1}`, 409)
	expect(fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", quota.ID), `{"version":1,"reason":"超额草稿"}`, 200)
	expect(fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", pr2.ID), `{"version":2,"reason":"纠错第二次"}`, 200)
	expect(fmt.Sprintf("/api/v1/sale-returns/%d/cancel", sr2.ID), `{"version":2,"reason":"纠错第二次"}`, 200)
	expect(fmt.Sprintf("/api/v1/sales/%d/cancel", so.ID), `{"version":4,"reason":"有生效退货"}`, 409)
	expect(fmt.Sprintf("/api/v1/sale-returns/%d/cancel", sr.ID), `{"version":2,"reason":"已退给供应商，库存不足"}`, 409)
	// Reverse supplier return first; two concurrent reversal submissions append once.
	start := make(chan struct{})
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			codes <- serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", pr.ID), `{"version":2,"reason":"纠错"}`, token).Code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 200 {
			success++
		} else if code != 409 {
			t.Fatalf("return cancellation race=%d", code)
		}
	}
	if success != 1 {
		t.Fatalf("return cancellation success=%d", success)
	}
	expect(fmt.Sprintf("/api/v1/sale-returns/%d/cancel", sr.ID), `{"version":2,"reason":"纠错"}`, 200)
	history := inventoryData[salereturn.Document](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sale-returns/%d", sr.ID), "", token), 200)
	if len(history.DirectTrace.SaleReturns) != 2 || len(history.DirectTrace.PurchaseReturns) != 3 || history.DirectTrace.SaleReturns[0].Status != "CANCELLED" || history.DirectTrace.PurchaseReturns[0].Status != "CANCELLED" || history.DirectTrace.SaleReturns[0].TotalAmount != "15.00" {
		t.Fatalf("return history erased=%+v", history.DirectTrace)
	}
	ledger := inventoryData[struct {
		Records []struct {
			PurchaseReturnID, SaleReturnID  int64
			EntryType, Quantity, DocumentNo string
		}
	}](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/inventory/entries?productId=%d&pageSize=500", goods), "", token), 200)
	signs := map[string]bool{}
	for _, row := range ledger.Records {
		if row.SaleReturnID == sr.ID {
			signs["SALE_"+row.EntryType+"_"+row.Quantity] = true
			if row.DocumentNo != sr.DocumentNo {
				t.Fatal("sales return inventory origin lost")
			}
		}
		if row.PurchaseReturnID == pr.ID {
			signs["PURCHASE_"+row.EntryType+"_"+row.Quantity] = true
			if row.DocumentNo != pr.DocumentNo {
				t.Fatal("purchase return inventory origin lost")
			}
		}
	}
	for _, key := range []string{"SALE_ORIGINAL_0.5", "SALE_REVERSAL_-0.5", "PURCHASE_ORIGINAL_-0.5", "PURCHASE_REVERSAL_0.5"} {
		if !signs[key] {
			t.Fatalf("missing immutable return inventory movement %s: %+v", key, ledger)
		}
	}
	for direction, documentNo := range map[string]string{"CUSTOMER": sr.DocumentNo, "SUPPLIER": pr.DocumentNo} {
		entries := inventoryData[struct {
			Records []struct {
				ID                 int64
				DocumentNo, Amount string
				ReversesID         *int64
			}
		}](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries?partnerId=%d&direction=%s&pageSize=500", partner, direction), "", token), 200)
		var original int64
		for _, row := range entries.Records {
			if row.DocumentNo == documentNo {
				original = row.ID
				if row.Amount != map[string]string{"CUSTOMER": "-15.00", "SUPPLIER": "-10.00"}[direction] {
					t.Fatalf("return money source=%+v", row)
				}
			}
		}
		reversal := false
		for _, row := range entries.Records {
			if row.ReversesID != nil && *row.ReversesID == original {
				reversal = true
				if row.Amount != map[string]string{"CUSTOMER": "15.00", "SUPPLIER": "10.00"}[direction] {
					t.Fatalf("return reversal amount=%+v", row)
				}
			}
		}
		if original == 0 || !reversal {
			t.Fatalf("return financial trace lost for %s: %+v", direction, entries)
		}
	}

}
func assertDirectConcurrentAssociations(t *testing.T, r http.Handler, token string, p, g int64) {
	t.Helper()
	pi := inventoryData[purchase.Draft](t, serveJSON(r, "POST", "/api/v1/purchases", fmt.Sprintf(`{"directDelivery":true,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"1.00"}]}`, p, g), token), 200)
	body := fmt.Sprintf(`{"directDelivery":true,"directPurchaseId":%d,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"2.00"}]}`, pi.ID, p, g)
	start := make(chan struct{})
	answers := make(chan struct {
		Code int
		ID   int64
	}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			x := serveJSON(r, "POST", "/api/v1/sales", body, token)
			var v struct{ Data struct{ ID int64 } }
			_ = json.Unmarshal(x.Body.Bytes(), &v)
			answers <- struct {
				Code int
				ID   int64
			}{x.Code, v.Data.ID}
		}()
	}
	close(start)
	success := 0
	var id int64
	for i := 0; i < 2; i++ {
		x := <-answers
		if x.Code == 200 {
			success++
			id = x.ID
		} else if x.Code != 409 {
			t.Fatalf("association race=%d", x.Code)
		}
	}
	if success != 1 {
		t.Fatalf("association success=%d", success)
	}
	inventoryData[sale.Draft](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/sales/%d/cancel", id), `{"version":1,"reason":"释放关联"}`, token), 200)
	// New association races purchase cancellation: both cannot succeed.
	start = make(chan struct{})
	codes := make(chan int, 2)
	go func() { <-start; codes <- serveJSON(r, "POST", "/api/v1/sales", body, token).Code }()
	go func() {
		<-start
		codes <- serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchases/%d/cancel", pi.ID), `{"version":1,"reason":"同时取消"}`, token).Code
	}()
	close(start)
	success = 0
	for i := 0; i < 2; i++ {
		code := <-codes
		if code == 200 {
			success++
		} else if code != 409 {
			t.Fatalf("association/cancel race=%d", code)
		}
	}
	if success != 1 {
		t.Fatalf("association/cancel successes=%d", success)
	}
	got := inventoryData[purchase.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/purchases/%d", pi.ID), "", token), 200)
	if got.Status == "CANCELLED" {
		for _, d := range got.DirectDocuments {
			if d.Status != "CANCELLED" {
				t.Fatal("cancelled purchase has active sale")
			}
		}
	}
}
