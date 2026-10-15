//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/config"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The query contract drives every read-only tool over a real SDK session and
// checks dates, statuses, effective returns, net amounts, reversal retention,
// current archive versus historical snapshot, and pagination.

type mcpSearchProduct struct {
	Category      *string `json:"category"`
	Brand         *string `json:"brand"`
	Model         *string `json:"model"`
	Specification *string `json:"specification"`
	PurchasePrice *string `json:"purchasePrice"`
	SalePrice     *string `json:"salePrice"`
	Remark        *string `json:"remark"`
	ID            int64   `json:"id"`
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Unit          string  `json:"unit"`
	Status        int     `json:"status"`
}

type mcpSearchPartner struct {
	Contact           *string `json:"contact"`
	Phone             *string `json:"phone"`
	Address           *string `json:"address"`
	InvoiceName       *string `json:"invoiceName"`
	TaxNumber         *string `json:"taxNumber"`
	RegisteredAddress *string `json:"registeredAddress"`
	RegisteredPhone   *string `json:"registeredPhone"`
	BankName          *string `json:"bankName"`
	BankAccount       *string `json:"bankAccount"`
	Remark            *string `json:"remark"`
	ID                int64   `json:"id"`
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	IsCustomer        bool    `json:"isCustomer"`
	IsSupplier        bool    `json:"isSupplier"`
	Status            int     `json:"status"`
}

type mcpSearchResult[T any] struct {
	Items             []T    `json:"items"`
	Page              int    `json:"page"`
	PageSize          int    `json:"pageSize"`
	Total             int64  `json:"total"`
	SelectionRequired bool   `json:"selectionRequired"`
	TimeZone          string `json:"timeZone"`
}

type mcpItemRecord struct {
	NetQuantity          *string `json:"netQuantity"`
	ReturnedQuantity     *string `json:"returnedQuantity"`
	NetAmount            *string `json:"netAmount"`
	ReturnedAmount       *string `json:"returnedAmount"`
	ProductModel         *string `json:"productModel"`
	ProductSpecification *string `json:"productSpecification"`
	ID                   int64   `json:"id"`
	SaleID               int64   `json:"saleId"`
	PurchaseID           int64   `json:"purchaseId"`
	PartnerID            int64   `json:"partnerId"`
	BusinessDate         string  `json:"businessDate"`
	Status               string  `json:"status"`
	ProductID            int64   `json:"productId"`
	ProductCode          string  `json:"productCode"`
	ProductName          string  `json:"productName"`
	ProductType          string  `json:"productType"`
	DocumentNo           string  `json:"documentNo"`
	PartnerName          string  `json:"partnerName"`
	Unit                 string  `json:"unit"`
	Quantity             string  `json:"quantity"`
	UnitPrice            string  `json:"unitPrice"`
	Amount               string  `json:"amount"`
	ReturnsApplicable    bool    `json:"returnsApplicable"`
	NetApplicable        bool    `json:"netApplicable"`
}

type mcpAmountSummary struct {
	ReturnedAmount *string `json:"returnedAmount"`
	NetAmount      *string `json:"netAmount"`
	Status         string  `json:"status"`
	OriginalAmount string  `json:"originalAmount"`
	ItemCount      int64   `json:"itemCount"`
}

type mcpQuantitySummary struct {
	ReturnedQuantity *string `json:"returnedQuantity"`
	NetQuantity      *string `json:"netQuantity"`
	ProductID        int64   `json:"productId"`
	Unit             string  `json:"unit"`
	Status           string  `json:"status"`
	Quantity         string  `json:"quantity"`
}

type mcpItemQuery struct {
	Filters struct {
		DateFrom string `json:"dateFrom"`
		DateTo   string `json:"dateTo"`
		Status   string `json:"status"`
	} `json:"filtersApplied"`
	Summary struct {
		Amounts    []mcpAmountSummary   `json:"amounts"`
		Quantities []mcpQuantitySummary `json:"quantities"`
	} `json:"summary"`
	Records   []mcpItemRecord `json:"records"`
	TimeZone  string          `json:"timeZone"`
	TimeBasis string          `json:"timeBasis"`
	QueriedAt string          `json:"queriedAt"`
	Page      int             `json:"page"`
	PageSize  int             `json:"pageSize"`
	Total     int64           `json:"total"`
	HasMore   bool            `json:"hasMore"`
}

type mcpDocument struct {
	OwnerName         *string `json:"ownerName"`
	Remark            *string `json:"remark"`
	DeliveryContact   *string `json:"deliveryContact"`
	DeliveryAddress   *string `json:"deliveryAddress"`
	ID                int64   `json:"id"`
	DocumentNo        string  `json:"documentNo"`
	PartnerName       string  `json:"partnerName"`
	Status            string  `json:"status"`
	BusinessDate      string  `json:"businessDate"`
	TotalAmount       string  `json:"totalAmount"`
	SnapshotAvailable bool    `json:"snapshotAvailable"`
}

type mcpDocumentLine struct {
	Remark       *string `json:"remark"`
	ProductModel *string `json:"productModel"`
	ID           int64   `json:"id"`
	ProductID    int64   `json:"productId"`
	ProductCode  string  `json:"productCode"`
	ProductName  string  `json:"productName"`
	ProductType  string  `json:"productType"`
	Unit         string  `json:"unit"`
	Quantity     string  `json:"quantity"`
	UnitPrice    string  `json:"unitPrice"`
	Amount       string  `json:"amount"`
}

type mcpDocumentResult struct {
	Filters struct {
		PurchaseID int64 `json:"purchaseId"`
		SaleID     int64 `json:"saleId"`
	} `json:"filtersApplied"`
	Document  mcpDocument       `json:"document"`
	Items     []mcpDocumentLine `json:"items"`
	Semantics string            `json:"semantics"`
	TimeZone  string            `json:"timeZone"`
}

// mcpReturnSource covers the return-source projections of both trades.
type mcpReturnSource struct {
	Items []struct {
		SaleItemID     int64 `json:"saleItemId"`
		PurchaseItemID int64 `json:"purchaseItemId"`
	} `json:"items"`
}

type mcpInventoryResult struct {
	Product         mcpSearchProduct `json:"product"`
	InventoryStatus string           `json:"inventoryStatus"`
	Quantity        *string          `json:"quantity"`
	Unit            string           `json:"unit"`
}

type mcpEntry struct {
	PaymentMethod      *string `json:"paymentMethod"`
	TransactionNo      *string `json:"transactionNo"`
	ReversedDocumentNo *string `json:"reversedDocumentNo"`
	ReversesID         *int64  `json:"reversesId"`
	ReversedByID       *int64  `json:"reversedById"`
	ID                 int64   `json:"id"`
	PartnerID          int64   `json:"partnerId"`
	EntryType          string  `json:"entryType"`
	Direction          string  `json:"direction"`
	DocumentNo         string  `json:"documentNo"`
	Amount             string  `json:"amount"`
	BalanceBefore      string  `json:"balanceBefore"`
	BalanceAfter       string  `json:"balanceAfter"`
	BusinessDate       string  `json:"businessDate"`
	EffectiveAt        string  `json:"effectiveAt"`
	Description        string  `json:"description"`
}

type mcpEntryQuery struct {
	Filters struct {
		DateFrom string `json:"dateFrom"`
		DateTo   string `json:"dateTo"`
	} `json:"filtersApplied"`
	Records   []mcpEntry `json:"records"`
	TimeBasis string     `json:"timeBasis"`
	Page      int        `json:"page"`
	PageSize  int        `json:"pageSize"`
	Total     int64      `json:"total"`
	HasMore   bool       `json:"hasMore"`
}

type mcpStatement struct {
	Note           *string    `json:"note"`
	Direction      string     `json:"direction"`
	OpeningAmount  string     `json:"openingAmount"`
	IncreaseAmount string     `json:"increaseAmount"`
	DecreaseAmount string     `json:"decreaseAmount"`
	NetChange      string     `json:"netChange"`
	ClosingAmount  string     `json:"closingAmount"`
	Records        []mcpEntry `json:"records"`
	Total          int64      `json:"total"`
	Page           int        `json:"page"`
	PageSize       int        `json:"pageSize"`
	HasMore        bool       `json:"hasMore"`
}

type mcpStatementResult struct {
	Filters struct {
		PartnerID int64  `json:"partnerId"`
		Direction string `json:"direction"`
		DateFrom  string `json:"dateFrom"`
		DateTo    string `json:"dateTo"`
	} `json:"filtersApplied"`
	Statements []mcpStatement `json:"statements"`
	TimeBasis  string         `json:"timeBasis"`
}

type mcpBalanceResult struct {
	Items []struct {
		Direction  string `json:"direction"`
		Amount     string `json:"amount"`
		Currency   string `json:"currency"`
		Meaning    string `json:"meaning"`
		HasRecords bool   `json:"hasRecords"`
	} `json:"items"`
	TimeZone string `json:"timeZone"`
}

// adminData reports the failing path because business failures answer with
// HTTP 200 and a non-200 envelope code.
func adminData[T any](t *testing.T, f *mcpFixture, method, path, body string) T {
	t.Helper()
	response := serveJSON(f.handler, method, path, body, f.adminToken)
	if response.Code != http.StatusOK {
		t.Fatalf("%s %s -> HTTP %d %s", method, path, response.Code, response.Body.String())
	}
	if code := envelopeCode(t, response); code != http.StatusOK {
		t.Fatalf("%s %s -> code %d %s", method, path, code, response.Body.String())
	}
	return inventoryData[T](t, response, 200)
}

func callToolData[T any](t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args any) T {
	t.Helper()
	var out T
	text := toolText(t, ctx, session, name, args)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%s decode: %v (%s)", name, err, text)
	}
	return out
}

func callToolError(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args any) string {
	t.Helper()
	var out struct {
		Error string `json:"error"`
	}
	text := toolText(t, ctx, session, name, args)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%s decode: %v (%s)", name, err, text)
	}
	if out.Error == "" {
		t.Fatalf("%s returned no error payload: %s", name, text)
	}
	return out.Error
}

func findAmount(amounts []mcpAmountSummary, status string) *mcpAmountSummary {
	for index := range amounts {
		if amounts[index].Status == status {
			return &amounts[index]
		}
	}
	return nil
}

// currentShanghaiMonth is the range the tools default to when no date is given.
func currentShanghaiMonth() (string, string) {
	now := time.Now().In(shanghaiZone)
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, shanghaiZone)
	return start.Format("2006-01-02"), start.AddDate(0, 1, -1).Format("2006-01-02")
}

func TestSQLiteMCPQueryContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-query.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	assertMCPQueryContract(t, db, true)
}

func TestPostgresMCPQueryContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	assertMCPQueryContract(t, db, false)
}

func assertMCPQueryContract(t *testing.T, db *platformdatabase.Database, sqlite bool) {
	t.Helper()
	f := newMCPFixture(t, db, sqlite)
	_, raw := f.issue(t, "query-contract", nil)
	ctx := context.Background()
	session := f.session(t, ctx, tokenHeader(raw))
	post := func(path, body string) map[string]any {
		t.Helper()
		return adminData[map[string]any](t, f, http.MethodPost, path, body)
	}
	idOf := func(path, body string) int64 {
		t.Helper()
		number, ok := post(path, body)["id"].(float64)
		if !ok {
			t.Fatalf("%s did not return an id", path)
		}
		return int64(number)
	}
	const productAName = "契约甲商品"
	goods := idOf("/api/v1/products", fmt.Sprintf(`{"code":"MQ-A","name":%q,"type":"GOODS","unit":"台","brand":"品牌甲","model":"M1","specification":"S1","category":"耗材","purchasePrice":"10.20","salePrice":"30.00","remark":"参考价说明"}`, productAName))
	idOf("/api/v1/products", `{"code":"MQ-B","name":"契约乙商品","type":"GOODS","unit":"支"}`)
	service := idOf("/api/v1/products", `{"code":"MQ-S","name":"契约丙服务","type":"SERVICE","unit":"次"}`)
	adjustment := idOf("/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"100","reason":"OPENING"}]}`, goods))
	post(fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", adjustment), `{"version":1}`)
	customer := idOf("/api/v1/partners", `{"code":"MQ-C","name":"契约客户","type":"COMPANY","isCustomer":true,"isSupplier":true,"contact":"张三","phone":"13800000000","address":"上海市某路1号","remark":"客户备注","invoiceName":"契约客户有限公司","taxNumber":"91310000TEST","registeredAddress":"注册地址","registeredPhone":"021-0000","bankName":"某银行","bankAccount":"6222000000000000"}`)
	supplier := idOf("/api/v1/partners", `{"code":"MQ-S1","name":"契约供应商","type":"COMPANY","isSupplier":true}`)

	line := func(product int64, kind, unit, quantity, price, remark string) string {
		return fmt.Sprintf(`{"productId":%d,"productType":%q,"unit":%q,"quantity":%q,"unitPrice":%q,"remark":%q}`, product, kind, unit, quantity, price, remark)
	}
	trade := func(kind, status, businessDate string, partner int64, items string) int64 {
		t.Helper()
		id := idOf("/api/v1/"+kind, fmt.Sprintf(`{"partnerId":%d,"businessDate":%q,"items":[%s]}`, partner, businessDate, items))
		switch status {
		case "POSTED":
			post(fmt.Sprintf("/api/v1/%s/%d/post", kind, id), `{"version":1}`)
		case "CANCELLED":
			post(fmt.Sprintf("/api/v1/%s/%d/cancel", kind, id), `{"version":1,"reason":"录错取消"}`)
		}
		return id
	}
	salePosted := trade("sales", "POSTED", "2026-09-10", customer, line(goods, "GOODS", "台", "10", "100.00", "行备注甲"))
	saleDraft := trade("sales", "DRAFT", "2026-09-15", customer, line(goods, "GOODS", "台", "2", "100.00", ""))
	trade("sales", "CANCELLED", "2026-09-16", customer, line(service, "SERVICE", "次", "1", "50.00", ""))
	// The September sale is returned in October: the later return is still
	// effective when September is queried again.
	saleSource := adminData[mcpReturnSource](t, f, http.MethodGet, fmt.Sprintf("/api/v1/sale-returns/source/%d", salePosted), "")
	saleReturn := idOf("/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-20","items":[{"saleItemId":%d,"quantity":"2"}]}`, salePosted, saleSource.Items[0].SaleItemID))
	post(fmt.Sprintf("/api/v1/sale-returns/%d/post", saleReturn), `{"version":1}`)

	purchasePosted := trade("purchases", "POSTED", "2026-09-12", supplier, line(goods, "GOODS", "台", "5", "80.00", "采购行备注"))
	trade("purchases", "DRAFT", "2026-09-18", supplier, line(goods, "GOODS", "台", "1", "80.00", ""))
	purchaseSource := adminData[mcpReturnSource](t, f, http.MethodGet, fmt.Sprintf("/api/v1/purchase-returns/source/%d", purchasePosted), "")
	purchaseReturn := idOf("/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-21","items":[{"purchaseItemId":%d,"quantity":"1"}]}`, purchasePosted, purchaseSource.Items[0].PurchaseItemID))
	post(fmt.Sprintf("/api/v1/purchase-returns/%d/post", purchaseReturn), `{"version":1}`)

	// Ledger entries become effective at the posting instant, which is why the
	// September period below must stay empty even though the business dates
	// sit in September.
	post("/api/v1/partner-balances/opening", fmt.Sprintf(`{"partnerId":%d,"direction":"CUSTOMER","amount":"1000.00","requestKey":"mq-opening","businessDate":"2026-09-01","description":"期初应收"}`, customer))
	receipt := adminData[mcpEntry](t, f, http.MethodPost, "/api/v1/partner-balances/settlements", fmt.Sprintf(`{"partnerId":%d,"direction":"CUSTOMER","amount":"300.00","requestKey":"mq-receipt","businessDate":"2026-09-15","paymentMethod":"BANK_TRANSFER","transactionNo":"FLOW-1","remark":"收到货款"}`, customer))
	reversal := adminData[mcpEntry](t, f, http.MethodPost, fmt.Sprintf("/api/v1/partner-balances/entries/%d/reverse", receipt.ID), `{"reason":"金额录错"}`)
	if reversal.ReversesID == nil || *reversal.ReversesID != receipt.ID {
		t.Fatalf("reversal link: %+v", reversal)
	}
	septemberFrom, septemberTo := "2026-09-01", "2026-09-30"
	monthFrom, monthTo := currentShanghaiMonth()

	// search_products now carries the reference prices and the archive remark.
	products := callToolData[mcpSearchResult[mcpSearchProduct]](t, ctx, session, "search_products", map[string]any{"keyword": productAName})
	if products.Total != 1 || products.SelectionRequired || products.TimeZone != "Asia/Shanghai" {
		t.Fatalf("search_products: %+v", products)
	}
	archiveProduct := products.Items[0]
	if archiveProduct.PurchasePrice == nil || *archiveProduct.PurchasePrice != "10.20" || archiveProduct.SalePrice == nil || *archiveProduct.SalePrice != "30.00" {
		t.Fatalf("reference prices: %+v", archiveProduct)
	}
	if archiveProduct.Category == nil || *archiveProduct.Category != "耗材" || archiveProduct.Brand == nil || archiveProduct.Remark == nil || *archiveProduct.Remark != "参考价说明" {
		t.Fatalf("product archive: %+v", archiveProduct)
	}
	// search_partners now carries contact, invoice and bank details.
	partners := callToolData[mcpSearchResult[mcpSearchPartner]](t, ctx, session, "search_partners", map[string]any{"keyword": "契约客户"})
	if partners.Total != 1 {
		t.Fatalf("search_partners: %+v", partners)
	}
	archivePartner := partners.Items[0]
	if archivePartner.BankAccount == nil || *archivePartner.BankAccount != "6222000000000000" || archivePartner.TaxNumber == nil || *archivePartner.TaxNumber != "91310000TEST" {
		t.Fatalf("partner bank and tax: %+v", archivePartner)
	}
	if archivePartner.InvoiceName == nil || archivePartner.Contact == nil || archivePartner.Phone == nil || archivePartner.Address == nil || archivePartner.Remark == nil {
		t.Fatalf("partner contact details: %+v", archivePartner)
	}

	stock := callToolData[mcpInventoryResult](t, ctx, session, "get_inventory_balances", map[string]any{"productId": goods})
	if stock.InventoryStatus != "GOODS" || stock.Quantity == nil || stock.Product.PurchasePrice == nil {
		t.Fatalf("inventory: %+v", stock)
	}
	serviceStock := callToolData[mcpInventoryResult](t, ctx, session, "get_inventory_balances", map[string]any{"productId": service})
	if serviceStock.InventoryStatus != "NOT_APPLICABLE_SERVICE" || serviceStock.Quantity != nil {
		t.Fatalf("service inventory: %+v", serviceStock)
	}

	balances := callToolData[mcpBalanceResult](t, ctx, session, "get_partner_balances", map[string]any{"partnerId": customer})
	if len(balances.Items) != 2 {
		t.Fatalf("partner balances: %+v", balances)
	}
	for _, item := range balances.Items {
		if item.Direction == "CUSTOMER" && item.Amount != "1800.00" {
			t.Fatalf("customer balance: %+v", item)
		}
		if item.Direction == "SUPPLIER" && item.HasRecords {
			t.Fatalf("supplier direction invented records: %+v", item)
		}
	}

	// September sales: the raw line, the October return and the net.
	sales := callToolData[mcpItemQuery](t, ctx, session, "query_sale_items", map[string]any{"dateFrom": septemberFrom, "dateTo": septemberTo})
	if sales.Total != 1 || len(sales.Records) != 1 || sales.Filters.Status != "POSTED" {
		t.Fatalf("september sales: %+v", sales)
	}
	if sales.Filters.DateFrom != septemberFrom || sales.Filters.DateTo != septemberTo || sales.PageSize != 20 || sales.TimeZone != "Asia/Shanghai" || sales.QueriedAt == "" {
		t.Fatalf("reported filters: %+v", sales)
	}
	row := sales.Records[0]
	if row.Quantity != "10" || row.Amount != "1000.00" || row.UnitPrice != "100.00" {
		t.Fatalf("original line: %+v", row)
	}
	if row.ReturnedQuantity == nil || *row.ReturnedQuantity != "2" || row.ReturnedAmount == nil || *row.ReturnedAmount != "200.00" {
		t.Fatalf("effective return: %+v", row)
	}
	if row.NetQuantity == nil || *row.NetQuantity != "8" || row.NetAmount == nil || *row.NetAmount != "800.00" || !row.NetApplicable || !row.ReturnsApplicable {
		t.Fatalf("net of the september sale: %+v", row)
	}
	postedAmount := findAmount(sales.Summary.Amounts, "POSTED")
	if postedAmount == nil || postedAmount.ItemCount != 1 || postedAmount.NetAmount == nil || *postedAmount.NetAmount != "800.00" {
		t.Fatalf("posted summary: %+v", sales.Summary.Amounts)
	}

	// All three statuses are reported separately, a draft never claims a net,
	// and the summary covers the whole filter set rather than one page.
	all := callToolData[mcpItemQuery](t, ctx, session, "query_sale_items", map[string]any{"dateFrom": septemberFrom, "dateTo": septemberTo, "status": "ALL", "pageSize": 1})
	if all.Total != 3 || len(all.Records) != 1 || all.PageSize != 1 || !all.HasMore || all.Filters.Status != "" {
		t.Fatalf("explicit page size and ALL status: %+v", all)
	}
	for _, status := range []string{"DRAFT", "POSTED", "CANCELLED"} {
		amounts := findAmount(all.Summary.Amounts, status)
		if amounts == nil || amounts.ItemCount != 1 {
			t.Fatalf("summary must cover every matching status: %+v", all.Summary.Amounts)
		}
	}
	if draftAmounts := findAmount(all.Summary.Amounts, "DRAFT"); draftAmounts.NetAmount != nil || draftAmounts.ReturnedAmount != nil {
		t.Fatalf("draft summary must not claim a net: %+v", draftAmounts)
	}
	drafts := callToolData[mcpItemQuery](t, ctx, session, "query_sale_items", map[string]any{"dateFrom": septemberFrom, "dateTo": septemberTo, "status": "DRAFT"})
	if drafts.Total != 1 || drafts.Records[0].NetApplicable || drafts.Records[0].NetAmount != nil || drafts.Records[0].Quantity != "2" {
		t.Fatalf("draft line: %+v", drafts.Records)
	}
	serviceItems := callToolData[mcpItemQuery](t, ctx, session, "query_sale_items", map[string]any{"dateFrom": septemberFrom, "dateTo": septemberTo, "status": "ALL", "productId": service})
	if serviceItems.Total != 1 || serviceItems.Records[0].ReturnsApplicable {
		t.Fatalf("service line: %+v", serviceItems.Records)
	}
	purchases := callToolData[mcpItemQuery](t, ctx, session, "query_purchase_items", map[string]any{"dateFrom": septemberFrom, "dateTo": septemberTo})
	if purchases.Total != 1 {
		t.Fatalf("september purchases: %+v", purchases)
	}
	purchaseRow := purchases.Records[0]
	if purchaseRow.Quantity != "5" || purchaseRow.ReturnedQuantity == nil || *purchaseRow.ReturnedQuantity != "1" || purchaseRow.NetQuantity == nil || *purchaseRow.NetQuantity != "4" {
		t.Fatalf("purchase net: %+v", purchaseRow)
	}
	defaulted := callToolData[mcpItemQuery](t, ctx, session, "query_sale_items", map[string]any{})
	if defaulted.Filters.DateFrom != monthFrom || defaulted.Filters.DateTo != monthTo || defaulted.Total != 0 {
		t.Fatalf("default range: %+v", defaulted)
	}
	partial := callToolData[mcpItemQuery](t, ctx, session, "query_sale_items", map[string]any{"dateFrom": "2026-09-10"})
	if partial.Filters.DateFrom != "2026-09-10" || partial.Filters.DateTo != septemberTo {
		t.Fatalf("partial range completion: %+v", partial.Filters)
	}

	// Document details keep the posting snapshot, including line remarks and
	// the real transaction prices.
	saleDocument := callToolData[mcpDocumentResult](t, ctx, session, "get_sale_document", map[string]any{"saleId": salePosted})
	if saleDocument.Document.Status != "POSTED" || !saleDocument.Document.SnapshotAvailable || saleDocument.Document.TotalAmount != "1000.00" {
		t.Fatalf("sale document: %+v", saleDocument.Document)
	}
	if len(saleDocument.Items) != 1 || saleDocument.Items[0].Remark == nil || *saleDocument.Items[0].Remark != "行备注甲" {
		t.Fatalf("sale line remark: %+v", saleDocument.Items)
	}
	if saleDocument.Items[0].UnitPrice != "100.00" || saleDocument.Items[0].Amount != "1000.00" {
		t.Fatalf("sale line amounts: %+v", saleDocument.Items[0])
	}
	purchaseDocument := callToolData[mcpDocumentResult](t, ctx, session, "get_purchase_document", map[string]any{"purchaseId": purchasePosted})
	if len(purchaseDocument.Items) != 1 || purchaseDocument.Items[0].Remark == nil || *purchaseDocument.Items[0].Remark != "采购行备注" {
		t.Fatalf("purchase line remark: %+v", purchaseDocument.Items)
	}
	if purchaseDocument.Filters.PurchaseID != purchasePosted || purchaseDocument.Document.PartnerName != "契约供应商" {
		t.Fatalf("purchase document: %+v", purchaseDocument)
	}
	// Renaming the archives must not rewrite a posted sale, while the current
	// archive and the purchase supplier projection follow the new name.
	adminData[map[string]any](t, f, http.MethodPut, fmt.Sprintf("/api/v1/products/%d", goods), fmt.Sprintf(`{"code":"MQ-A","name":"契约甲商品改名","type":"GOODS","unit":"台","purchasePrice":"10.20","salePrice":"30.00"}`))
	adminData[map[string]any](t, f, http.MethodPut, fmt.Sprintf("/api/v1/partners/%d", customer), `{"code":"MQ-C","name":"契约客户改名","type":"COMPANY","isCustomer":true,"isSupplier":true,"bankAccount":"9999888877776666"}`)
	afterRename := callToolData[mcpDocumentResult](t, ctx, session, "get_sale_document", map[string]any{"saleId": salePosted})
	if afterRename.Document.PartnerName != saleDocument.Document.PartnerName || afterRename.Items[0].ProductName != saleDocument.Items[0].ProductName {
		t.Fatalf("posted sale changed with the current archive: %+v", afterRename)
	}
	archive := callToolData[mcpSearchResult[mcpSearchPartner]](t, ctx, session, "search_partners", map[string]any{"keyword": "契约客户改名"})
	if archive.Total != 1 || archive.Items[0].BankAccount == nil || *archive.Items[0].BankAccount != "9999888877776666" {
		t.Fatalf("current archive after renaming: %+v", archive)
	}
	draftDocument := callToolData[mcpDocumentResult](t, ctx, session, "get_sale_document", map[string]any{"saleId": saleDraft})
	if draftDocument.Document.SnapshotAvailable || draftDocument.Document.PartnerName != "契约客户改名" {
		t.Fatalf("draft must fall back to the current archive: %+v", draftDocument.Document)
	}
	// The printer header has no fallback for a draft, so it must read as
	// unavailable rather than as an empty frozen value.
	if draftDocument.Document.OwnerName != nil {
		t.Fatalf("draft printer header must not pose as a snapshot: %+v", draftDocument.Document)
	}
	if saleDocument.Document.OwnerName == nil {
		t.Fatalf("posted printer header must be a frozen value: %+v", saleDocument.Document)
	}

	// Ledger queries use the effective instant and keep reversal pairs.
	entries := callToolData[mcpEntryQuery](t, ctx, session, "query_partner_entries", map[string]any{"partnerId": customer, "direction": "CUSTOMER"})
	if entries.Total != 5 || len(entries.Records) != 5 || entries.PageSize != 20 || entries.TimeBasis == "" {
		t.Fatalf("current month entries: %+v", entries)
	}
	empty := callToolData[mcpEntryQuery](t, ctx, session, "query_partner_entries", map[string]any{"partnerId": customer, "direction": "CUSTOMER", "dateFrom": septemberFrom, "dateTo": septemberTo})
	if empty.Total != 0 {
		t.Fatalf("the september period must hold no effective entry: %+v", empty)
	}
	var original, reversalRow *mcpEntry
	for index := range entries.Records {
		switch entries.Records[index].EntryType {
		case "RECEIPT":
			original = &entries.Records[index]
		case "REVERSAL":
			reversalRow = &entries.Records[index]
		}
	}
	if original == nil || reversalRow == nil {
		t.Fatalf("reversal pair missing: %+v", entries.Records)
	}
	if original.ReversedByID == nil || *original.ReversedByID != reversalRow.ID || reversalRow.ReversesID == nil || *reversalRow.ReversesID != original.ID {
		t.Fatalf("reversal links: %+v / %+v", original, reversalRow)
	}
	if reversalRow.Amount != "300.00" || reversalRow.Description != "金额录错" || original.Amount != "-300.00" {
		t.Fatalf("reversal amounts: %+v / %+v", original, reversalRow)
	}
	if original.Description != "收到货款" || original.PaymentMethod == nil || *original.PaymentMethod != "BANK_TRANSFER" || original.TransactionNo == nil || *original.TransactionNo != "FLOW-1" {
		t.Fatalf("funds details: %+v", original)
	}
	for _, record := range entries.Records {
		if record.EntryType == "OPENING" && record.BusinessDate != "2026-09-01" {
			t.Fatalf("business date must survive beside the effective instant: %+v", record)
		}
	}
	settlements := callToolData[mcpEntryQuery](t, ctx, session, "query_partner_entries", map[string]any{"partnerId": customer, "category": "SETTLEMENT"})
	if settlements.Total != 2 {
		t.Fatalf("settlement category must keep the reversal: %+v", settlements)
	}
	effectiveOnly := callToolData[mcpEntryQuery](t, ctx, session, "query_partner_entries", map[string]any{"partnerId": customer, "reversalStatus": "EFFECTIVE"})
	if effectiveOnly.Total != 3 {
		t.Fatalf("effective filter: %+v", effectiveOnly)
	}

	// The statement reports the whole period and pages only its lines.
	statement := callToolData[mcpStatementResult](t, ctx, session, "get_partner_statement", map[string]any{"partnerId": customer, "direction": "CUSTOMER", "dateFrom": monthFrom, "dateTo": monthTo})
	if len(statement.Statements) != 1 || statement.TimeBasis == "" {
		t.Fatalf("statement directions: %+v", statement)
	}
	sheet := statement.Statements[0]
	if sheet.OpeningAmount != "0.00" || sheet.IncreaseAmount != "2300.00" || sheet.DecreaseAmount != "500.00" || sheet.NetChange != "1800.00" || sheet.ClosingAmount != "1800.00" || sheet.Total != 5 {
		t.Fatalf("statement totals: %+v", sheet)
	}
	paged := callToolData[mcpStatementResult](t, ctx, session, "get_partner_statement", map[string]any{"partnerId": customer, "direction": "CUSTOMER", "dateFrom": monthFrom, "dateTo": monthTo, "pageSize": 2})
	if len(paged.Statements[0].Records) != 2 || paged.Statements[0].Total != 5 || !paged.Statements[0].HasMore || paged.Statements[0].ClosingAmount != "1800.00" {
		t.Fatalf("paged statement: %+v", paged.Statements[0])
	}
	both := callToolData[mcpStatementResult](t, ctx, session, "get_partner_statement", map[string]any{"partnerId": customer, "dateFrom": monthFrom, "dateTo": monthTo})
	if len(both.Statements) != 2 || both.Statements[0].Direction == both.Statements[1].Direction {
		t.Fatalf("an omitted direction must report both identities: %+v", both.Statements)
	}
	if both.Statements[1].ClosingAmount != "0.00" || both.Statements[1].Note == nil {
		t.Fatalf("direction without history: %+v", both.Statements[1])
	}

	// Argument boundaries stay explicit.
	for _, spec := range []struct {
		name string
		args map[string]any
	}{
		{"query_sale_items", map[string]any{"dateFrom": "2026-09-30", "dateTo": "2026-09-01"}},
		{"query_sale_items", map[string]any{"dateFrom": "2026-9-1"}},
		{"query_sale_items", map[string]any{"status": "BOGUS"}},
		{"query_sale_items", map[string]any{"page": 0}},
		{"query_sale_items", map[string]any{"pageSize": 51}},
		{"query_sale_items", map[string]any{"unknown": "x"}},
		{"query_sale_items", map[string]any{"partnerId": customer, "partnerKeyword": "契约"}},
		{"query_purchase_items", map[string]any{"dateTo": "2026-9-1"}},
		{"query_partner_entries", map[string]any{"direction": "ALL"}},
		{"query_partner_entries", map[string]any{"category": "BOGUS"}},
		{"get_partner_statement", map[string]any{"partnerId": 0}},
		{"get_sale_document", map[string]any{"saleId": 999999}},
		{"get_purchase_document", map[string]any{"purchaseId": 0}},
		{"search_products", map[string]any{"keyword": nil}},
	} {
		if got := callToolError(t, ctx, session, spec.name, spec.args); got != "invalid_arguments" && got != "not_found" {
			t.Fatalf("%s %v -> %s", spec.name, spec.args, got)
		}
	}
	// A result above the configured output limit is refused rather than
	// silently trimmed. The call is sent raw so the tiny limit cannot also
	// truncate the protocol handshake.
	small := newMCPFixture(t, db, sqlite, func(cfg *config.Config) { cfg.MCP.MaxOutputBytes = 400 })
	_, smallRaw := small.issue(t, "small-output", nil)
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_products","arguments":{"keyword":""}}}`
	response := small.request(t, http.MethodPost, call, tokenHeader(smallRaw))
	if response.status != http.StatusOK || !strings.Contains(string(response.body), "output_too_large") {
		t.Fatalf("output limit: %d %s", response.status, response.body)
	}
}
