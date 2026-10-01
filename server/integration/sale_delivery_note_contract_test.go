//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

func TestSQLiteSaleDeliveryNotePostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-note.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertSaleDeliveryNoteContract(t, router, db, "sqlite")
}
func TestPostgresSaleDeliveryNotePostgresSQLiteHTTPContract(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertSaleDeliveryNoteContract(t, router, db, "postgres")
}

type deliveryNoteResponse struct {
	DocumentNo      string  `json:"documentNo"`
	Status          string  `json:"status"`
	Posted          bool    `json:"posted"`
	BusinessDate    string  `json:"businessDate"`
	PartnerID       int64   `json:"partnerId"`
	PartnerName     string  `json:"partnerName"`
	DeliveryContact *string `json:"deliveryContact"`
	DeliveryPhone   *string `json:"deliveryPhone"`
	DeliveryAddress *string `json:"deliveryAddress"`
	OwnerName       string  `json:"ownerName"`
	OwnerPhone      string  `json:"ownerPhone"`
	OwnerAddress    string  `json:"ownerAddress"`
	TotalQuantity   string  `json:"totalQuantity"`
	TotalAmount     string  `json:"totalAmount"`
	Items           []struct {
		ProductCode          string  `json:"productCode"`
		ProductName          string  `json:"productName"`
		ProductModel         *string `json:"productModel"`
		ProductSpecification *string `json:"productSpecification"`
		Unit                 string  `json:"unit"`
		Quantity             string  `json:"quantity"`
		UnitPrice            string  `json:"unitPrice"`
		Amount               string  `json:"amount"`
	} `json:"items"`
}

func assertSaleDeliveryNoteContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	if r := serveJSON(router, http.MethodGet, "/api/v1/sales/1/delivery-note", "", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delivery note status=%d body=%s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodGet, "/api/v1/sales/99999/delivery-note", "", token); r.Code != 404 {
		t.Fatalf("missing delivery note status=%d body=%s", r.Code, r.Body.String())
	}
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", fmt.Sprintf(`{"code":"DN-%s","name":"送货客户原名","type":"COMPANY","isCustomer":true,"contact":"档案联系人","phone":"10086","address":"档案地址"}`, dialect), token), 200).ID
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"DNG-%s","name":"送货商品原名","type":"GOODS","model":"M1","specification":"S1","unit":"台"}`, dialect), token), 200).ID
	if r := serveJSON(router, http.MethodPut, "/api/v1/print-profile", `{"name":"经营者原名","phone":"13900000000","address":"经营者地址"}`, token); r.Code != 200 {
		t.Fatalf("set print profile: %d %s", r.Code, r.Body.String())
	}
	draftID := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","deliveryContact":"本次联系人","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"1.50"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"3","unitPrice":"2.00"}]}`, partner, product, product), token), 200).ID
	// A draft prints with the current saved content and is flagged not posted.
	draft := inventoryData[deliveryNoteResponse](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sales/%d/delivery-note", draftID), "", token), 200)
	if draft.Status != "DRAFT" || draft.Posted || draft.PartnerName != "送货客户原名" || draft.OwnerName != "经营者原名" || draft.DeliveryContact == nil || *draft.DeliveryContact != "本次联系人" || len(draft.Items) != 2 || draft.TotalQuantity != "5" || draft.TotalAmount != "9.00" {
		t.Fatalf("draft delivery note = %+v", draft)
	}
	if draft.Items[1].ProductModel == nil || *draft.Items[1].ProductModel != "M1" || draft.Items[1].UnitPrice != "2.00" || draft.Items[1].Amount != "6.00" {
		t.Fatalf("draft line snapshot = %+v", draft.Items[1])
	}
	// A delivery note never changes stock or receivables.
	opening := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"10","reason":"OPENING"}]}`, product), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", opening.ID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post opening: %d %s", r.Code, r.Body.String())
	}
	var stockBefore, receivableBefore int64
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stockBefore).Error; e != nil {
		t.Fatal(e)
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", draftID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post sale: %d %s", r.Code, r.Body.String())
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&receivableBefore).Error; e != nil {
		t.Fatal(e)
	}
	// Renaming the customer, product and operator must not change a posted note,
	// and the frozen snapshot keeps empty values instead of falling back.
	if r := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/partners/%d", partner), fmt.Sprintf(`{"code":"DN-%s","name":"送货客户改名","type":"COMPANY","isCustomer":true,"contact":"新联系人","phone":"99999","address":"新地址"}`, dialect), token); r.Code != 200 {
		t.Fatalf("rename partner: %d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/products/%d", product), fmt.Sprintf(`{"code":"DNG-%s","name":"送货商品改名","type":"GOODS","model":"M2","specification":"S2","unit":"台"}`, dialect), token); r.Code != 200 {
		t.Fatalf("rename product: %d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPut, "/api/v1/print-profile", `{"name":"经营者改名","phone":"13800000000","address":"新经营者地址"}`, token); r.Code != 200 {
		t.Fatalf("rename owner: %d %s", r.Code, r.Body.String())
	}
	posted := inventoryData[deliveryNoteResponse](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sales/%d/delivery-note", draftID), "", token), 200)
	if !posted.Posted || posted.Status != "POSTED" || posted.PartnerName != "送货客户原名" || posted.OwnerName != "经营者原名" || posted.Items[0].ProductName != "送货商品原名" || posted.Items[0].ProductModel == nil || *posted.Items[0].ProductModel != "M1" || posted.DeliveryContact == nil || *posted.DeliveryContact != "本次联系人" {
		t.Fatalf("posted delivery note did not keep its frozen snapshot: %+v", posted)
	}
	var stockAfter, receivableAfter int64
	_ = db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stockAfter).Error
	_ = db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&receivableAfter).Error
	// Reading the note again changed nothing at all.
	if stockAfter != stockBefore-5000 || receivableAfter != receivableBefore {
		t.Fatalf("delivery note mutated stock/receivable: stock %d/%d receivable %d/%d", stockBefore, stockAfter, receivableBefore, receivableAfter)
	}
	// Empty delivery contact stays empty on a posted note even though the partner
	// record now has a contact, proving no fallback to the current profile.
	blankPartner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", fmt.Sprintf(`{"code":"DNB-%s","name":"空值客户","type":"COMPANY","isCustomer":true}`, dialect), token), 200).ID
	blankSale := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","deliveryContact":"","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"1.00"}]}`, blankPartner, product), token), 200).ID
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", blankSale), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post blank sale: %d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/partners/%d", blankPartner), fmt.Sprintf(`{"code":"DNB-%s","name":"空值客户","type":"COMPANY","isCustomer":true,"contact":"后来补的联系人"}`, dialect), token); r.Code != 200 {
		t.Fatalf("add partner contact: %d %s", r.Code, r.Body.String())
	}
	blank := inventoryData[deliveryNoteResponse](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sales/%d/delivery-note", blankSale), "", token), 200)
	if blank.DeliveryContact != nil {
		t.Fatalf("posted note fell back to the current partner contact: %+v", blank.DeliveryContact)
	}
	// A cancelled sale must not pass itself off as an effective delivery note.
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", draftID), `{"version":2,"reason":"录错"}`, token); r.Code != 200 {
		t.Fatalf("cancel sale: %d %s", r.Code, r.Body.String())
	}
	cancelled := inventoryData[deliveryNoteResponse](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sales/%d/delivery-note", draftID), "", token), 200)
	if cancelled.Posted || cancelled.Status != "CANCELLED" {
		t.Fatalf("cancelled note must not look posted: %+v", cancelled)
	}
}
