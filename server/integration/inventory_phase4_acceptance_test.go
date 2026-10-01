//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
)

func TestSQLiteInventoryPhase4AcceptancePostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory-phase4.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	assertInventoryPhase4Acceptance(t, router, db, "sqlite")
}

func TestPostgresInventoryPhase4AcceptancePostgresSQLiteHTTPContract(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	assertInventoryPhase4Acceptance(t, router, db, "postgres")
}

func assertInventoryPhase4Acceptance(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", fmt.Sprintf(`{"code":"TRACE-%s","name":"追溯往来单位","type":"COMPANY","isCustomer":true,"isSupplier":true}`, dialect), token), 200).ID
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"TRACE-G-%s","name":"追溯实物","type":"GOODS","unit":"台","model":"","specification":""}`, dialect), token), 200).ID
	serviceProduct := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"TRACE-S-%s","name":"追溯服务","type":"SERVICE","unit":"次"}`, dialect), token), 200).ID

	adjustment := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, adjustmentPath, fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"10","reason":"OPENING"}]}`, product), token), 200)
	postAdjustment := func(id int64, version int64) {
		t.Helper()
		inventoryData[any](t, serveJSON(router, http.MethodPost, fmt.Sprintf("%s/%d/post", adjustmentPath, id), fmt.Sprintf(`{"version":%d}`, version), token), 200)
	}
	postAdjustment(adjustment.ID, adjustment.Version)
	toReverse := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, adjustmentPath, fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","reason":"SURPLUS"}]}`, product), token), 200)
	postAdjustment(toReverse.ID, toReverse.Version)
	inventoryData[any](t, serveJSON(router, http.MethodPost, fmt.Sprintf("%s/%d/cancel", adjustmentPath, toReverse.ID), fmt.Sprintf(`{"version":%d,"reason":"追溯反向流水"}`, toReverse.Version+1), token), 200)

	purchase := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/purchases", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"5.00"}]}`, partner, product), token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/post", purchase.ID), fmt.Sprintf(`{"version":%d}`, purchase.Version), token), 200)

	sale := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"3","unitPrice":"2.00"},{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","unitPrice":"20.00"}]}`, partner, product, serviceProduct), token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", sale.ID), fmt.Sprintf(`{"version":%d}`, sale.Version), token), 200)

	fund := func(path, key, direction, amount string) {
		t.Helper()
		body := fmt.Sprintf(`{"requestKey":%q,"partnerId":%d,"direction":%q,"amount":%q,"businessDate":"2026-10-01","paymentMethod":"BANK_TRANSFER","transactionNo":"%s","remark":"contract settlement"}`, key, partner, direction, amount, key)
		inventoryData[any](t, serveJSON(router, http.MethodPost, path, body, token), 200)
	}
	// Settle both documents first; posting their returns then creates the exact
	// negative balances covered by the public refund endpoint.
	fund("/api/v1/partner-balances/settlements", "trace-supplier-payment-"+dialect, "SUPPLIER", "10.00")
	fund("/api/v1/partner-balances/settlements", "trace-customer-receipt-"+dialect, "CUSTOMER", "26.00")

	purchaseSource := inventoryData[struct {
		Items []struct {
			PurchaseItemID int64 `json:"purchaseItemId"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchase-returns/source/%d", purchase.ID), "", token), 200)
	purchaseReturn := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"1"}]}`, purchase.ID, purchaseSource.Items[0].PurchaseItemID), token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/post", purchaseReturn.ID), fmt.Sprintf(`{"version":%d}`, purchaseReturn.Version), token), 200)

	saleSource := inventoryData[struct {
		Items []struct {
			SaleItemID int64 `json:"saleItemId"`
			ProductID  int64 `json:"productId"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sale-returns/source/%d", sale.ID), "", token), 200)
	var saleGoodsItemID int64
	for _, item := range saleSource.Items {
		if item.ProductID == product {
			saleGoodsItemID = item.SaleItemID
		}
	}
	if saleGoodsItemID == 0 {
		t.Fatalf("sale source did not include its posted goods line: %+v", saleSource.Items)
	}
	saleReturn := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"}]}`, sale.ID, saleGoodsItemID), token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", saleReturn.ID), fmt.Sprintf(`{"version":%d}`, saleReturn.Version), token), 200)

	fund("/api/v1/partner-balances/refunds", "trace-supplier-refund-"+dialect, "SUPPLIER", "5.00")
	fund("/api/v1/partner-balances/refunds", "trace-customer-refund-"+dialect, "CUSTOMER", "2.00")

	type ledgerRow struct {
		ID                   int64   `json:"id"`
		ProductID            int64   `json:"productId"`
		AdjustmentID         *int64  `json:"adjustmentId"`
		PurchaseID           *int64  `json:"purchaseId"`
		SaleID               *int64  `json:"saleId"`
		PurchaseReturnID     *int64  `json:"purchaseReturnId"`
		SaleReturnID         *int64  `json:"saleReturnId"`
		SourceType           string  `json:"sourceType"`
		EntryType            string  `json:"entryType"`
		Quantity             string  `json:"quantity"`
		DocumentNo           string  `json:"documentNo"`
		ProductName          string  `json:"productName"`
		ProductModel         *string `json:"productModel"`
		ProductSpecification *string `json:"productSpecification"`
	}
	type entryPage struct {
		Records []ledgerRow `json:"records"`
		Total   int64       `json:"total"`
	}
	entryPath := "/api/v1/inventory/entries"
	query := func(q string) entryPage {
		t.Helper()
		return inventoryData[entryPage](t, serveJSON(router, http.MethodGet, entryPath+q, "", token), 200)
	}
	baseQuery := fmt.Sprintf("?productId=%d&pageSize=500", product)
	all := query(baseQuery)
	if int64(len(all.Records)) != all.Total || all.Total != 7 {
		t.Fatalf("ledger query should preserve legacy no-source filtering: %+v", all)
	}
	legacyOriginals := query(baseQuery + "&entryType=ORIGINAL")
	if legacyOriginals.Total != 6 || int64(len(legacyOriginals.Records)) != 6 {
		t.Fatalf("legacy product+entryType query changed without sourceType: %+v", legacyOriginals)
	}
	legacyReversals := query(baseQuery + "&entryType=REVERSAL")
	if legacyReversals.Total != 1 || len(legacyReversals.Records) != 1 || legacyReversals.Records[0].SourceType != "ADJUSTMENT" {
		t.Fatalf("legacy reversal query should keep the adjustment reversal: %+v", legacyReversals)
	}
	counts := map[string]int64{"ADJUSTMENT": 3, "PURCHASE": 1, "SALE": 1, "PURCHASE_RETURN": 1, "SALE_RETURN": 1}
	seenSources := map[string]bool{}
	for sourceType, want := range counts {
		filtered := query(baseQuery + "&sourceType=" + sourceType)
		if filtered.Total != want || int64(len(filtered.Records)) != want {
			t.Fatalf("sourceType=%s rows=%d total=%d want=%d", sourceType, len(filtered.Records), filtered.Total, want)
		}
		originals := query(baseQuery + "&sourceType=" + sourceType + "&entryType=ORIGINAL")
		wantOriginals := want
		if sourceType == "ADJUSTMENT" {
			wantOriginals = 2
		}
		if originals.Total != wantOriginals || int64(len(originals.Records)) != wantOriginals {
			t.Fatalf("orthogonal original filter source=%s got=%+v want=%d", sourceType, originals, wantOriginals)
		}
		reversals := query(baseQuery + "&sourceType=" + sourceType + "&entryType=REVERSAL")
		wantReversals := int64(0)
		if sourceType == "ADJUSTMENT" {
			wantReversals = 1
		}
		if reversals.Total != wantReversals || int64(len(reversals.Records)) != wantReversals {
			t.Fatalf("orthogonal reversal filter source=%s got=%+v want=%d", sourceType, reversals, wantReversals)
		}
		for _, record := range filtered.Records {
			seenSources[sourceType] = true
			validSourceID := false
			switch sourceType {
			case "ADJUSTMENT":
				validSourceID = record.AdjustmentID != nil && (*record.AdjustmentID == adjustment.ID || *record.AdjustmentID == toReverse.ID)
			case "PURCHASE":
				validSourceID = record.PurchaseID != nil && *record.PurchaseID == purchase.ID
			case "SALE":
				validSourceID = record.SaleID != nil && *record.SaleID == sale.ID
			case "PURCHASE_RETURN":
				validSourceID = record.PurchaseReturnID != nil && *record.PurchaseReturnID == purchaseReturn.ID
			case "SALE_RETURN":
				validSourceID = record.SaleReturnID != nil && *record.SaleReturnID == saleReturn.ID
			}
			if !validSourceID {
				t.Fatalf("source filter %s returned a row with the wrong source identifier: %+v", sourceType, record)
			}
		}
	}
	if len(seenSources) != 5 {
		t.Fatalf("not all five source types were returned: %+v", seenSources)
	}
	if invalid := serveJSON(router, http.MethodGet, entryPath+baseQuery+"&sourceType=UNKNOWN", "", token); invalid.Code != 400 {
		t.Fatalf("invalid source filter status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	serviceEntries := query(fmt.Sprintf("?productId=%d&pageSize=500", serviceProduct))
	if serviceEntries.Total != 0 || len(serviceEntries.Records) != 0 {
		t.Fatalf("service product must not create stock ledger entries: %+v", serviceEntries)
	}
	for _, record := range all.Records {
		if record.ProductID != product || record.ProductName != "追溯实物" {
			t.Fatalf("stock ledger lost product snapshot or product identity: %+v", record)
		}
	}
	// The balance equals every original and reverse movement, across all sources.
	quantityMilli := int64(0)
	for _, record := range all.Records {
		quantityMilli += quantityToMilli(t, record.Quantity)
	}
	if quantityMilli != 9000 {
		t.Fatalf("inventory balance from public ledger=%d milli, want 9000", quantityMilli)
	}
	publicStock := inventoryData[struct {
		Records []struct {
			ProductID int64  `json:"productId"`
			Quantity  string `json:"quantity"`
		} `json:"records"`
	}](t, serveJSON(router, http.MethodGet, "/api/v1/inventory/balances?page=1&pageSize=500&stock=all", "", token), 200)
	var foundProduct bool
	for _, row := range publicStock.Records {
		if row.ProductID == product && quantityToMilli(t, row.Quantity) == quantityMilli {
			foundProduct = true
		}
	}
	if !foundProduct {
		t.Fatalf("public balance does not equal ledger: rows=%+v ledger=%d", publicStock.Records, quantityMilli)
	}

	// Source rows open their real, authenticated document APIs, whose responses
	// expose the entire document and its final lifecycle state.
	documents := []struct {
		path   string
		id     int64
		status string
	}{
		{adjustmentPath, toReverse.ID, "CANCELLED"}, {"/api/v1/purchases", purchase.ID, "POSTED"}, {"/api/v1/sales", sale.ID, "POSTED"},
		{"/api/v1/purchase-returns", purchaseReturn.ID, "POSTED"}, {"/api/v1/sale-returns", saleReturn.ID, "POSTED"},
	}
	for _, document := range documents {
		var detail struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
			Items  []any  `json:"items"`
		}
		path := fmt.Sprintf("%s/%d", document.path, document.id)
		response := serveJSON(router, http.MethodGet, path, "", token)
		if response.Code != 200 {
			t.Fatalf("open source document %s: %d %s", path, response.Code, response.Body.String())
		}
		detail = inventoryData[struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
			Items  []any  `json:"items"`
		}](t, response, 200)
		if detail.ID != document.id || detail.Status != document.status || len(detail.Items) == 0 {
			t.Fatalf("source detail omitted status or full lines: path=%s result=%+v", path, detail)
		}
	}

	for _, direction := range []string{"CUSTOMER", "SUPPLIER"} {
		balances := inventoryData[receivable.BalancePage](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/partner-balances?partnerId=%d&direction=%s", partner, direction), "", token), 200)
		if balances.Total != 1 || len(balances.Records) != 1 || balances.Records[0].Amount != "0.00" {
			t.Fatalf("%s balance should clear after settlement/return/refund: %+v", direction, balances)
		}
		entries := inventoryData[receivable.Page](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/partner-balances/entries?partnerId=%d&direction=%s&pageSize=500", partner, direction), "", token), 200)
		amountCents := int64(0)
		seenTypes := map[string]bool{}
		for _, entry := range entries.Records {
			amountCents += moneyToCents(t, entry.Amount)
			seenTypes[entry.EntryType] = true
		}
		if int64(len(entries.Records)) != entries.Total || amountCents != 0 {
			t.Fatalf("%s immutable balance entries=%+v sum=%d", direction, entries, amountCents)
		}
		refundType := direction + "_REFUND"
		settlementType := "RECEIPT"
		if direction == "SUPPLIER" {
			settlementType = "PAYMENT"
		}
		if !seenTypes[refundType] || !seenTypes[settlementType] {
			t.Fatalf("%s history should include settlement and refund: %+v", direction, seenTypes)
		}
	}
}

func quantityToMilli(t *testing.T, value string) int64 {
	t.Helper()
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	whole, fraction, _ := strings.Cut(value, ".")
	wholeMilli, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		t.Fatalf("invalid quantity %q: %v", value, err)
	}
	fraction += "000"
	fractionMilli, err := strconv.ParseInt(fraction[:3], 10, 64)
	if err != nil {
		t.Fatalf("invalid quantity fraction %q: %v", value, err)
	}
	result := wholeMilli*1000 + fractionMilli
	if negative {
		return -result
	}
	return result
}

func moneyToCents(t *testing.T, value string) int64 {
	t.Helper()
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	whole, fraction, _ := strings.Cut(value, ".")
	wholeCents, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		t.Fatalf("invalid amount %q: %v", value, err)
	}
	fraction += "00"
	fractionCents, err := strconv.ParseInt(fraction[:2], 10, 64)
	if err != nil {
		t.Fatalf("invalid amount fraction %q: %v", value, err)
	}
	result := wholeCents*100 + fractionCents
	if negative {
		return -result
	}
	return result
}
