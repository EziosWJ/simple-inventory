//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
)

// Sale item queries read the saved lines, the page count and the summary of
// every matching line through the public HTTP contract on both databases.

type queryItem struct {
	NetQuantity       *string `json:"netQuantity"`
	NetAmount         *string `json:"netAmount"`
	ProductModel      *string `json:"productModel"`
	ID                int64   `json:"id"`
	SaleID            int64   `json:"saleId"`
	DocumentNo        string  `json:"documentNo"`
	PartnerID         int64   `json:"partnerId"`
	PartnerName       string  `json:"partnerName"`
	BusinessDate      string  `json:"businessDate"`
	Status            string  `json:"status"`
	ProductID         int64   `json:"productId"`
	ProductCode       string  `json:"productCode"`
	ProductName       string  `json:"productName"`
	ProductType       string  `json:"productType"`
	Unit              string  `json:"unit"`
	Quantity          string  `json:"quantity"`
	UnitPrice         string  `json:"unitPrice"`
	Amount            string  `json:"amount"`
	ReturnedQuantity  string  `json:"returnedQuantity"`
	ReturnedAmount    string  `json:"returnedAmount"`
	ReturnsApplicable bool    `json:"returnsApplicable"`
	NetApplicable     bool    `json:"netApplicable"`
	MatchesHistory    bool    `json:"matchesHistory"`
}

type queryAmounts struct {
	ReturnedAmount *string `json:"returnedAmount"`
	NetAmount      *string `json:"netAmount"`
	Status         string  `json:"status"`
	ItemCount      int64   `json:"itemCount"`
	OriginalAmount string  `json:"originalAmount"`
}

type queryQuantity struct {
	ReturnedQuantity  *string `json:"returnedQuantity"`
	NetQuantity       *string `json:"netQuantity"`
	ProductID         int64   `json:"productId"`
	ProductCode       string  `json:"productCode"`
	ProductName       string  `json:"productName"`
	ProductType       string  `json:"productType"`
	Unit              string  `json:"unit"`
	Status            string  `json:"status"`
	Quantity          string  `json:"quantity"`
	ReturnsApplicable bool    `json:"returnsApplicable"`
}

type queryResult struct {
	Records  []queryItem `json:"records"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
	Summary  struct {
		Amounts    []queryAmounts  `json:"amounts"`
		Quantities []queryQuantity `json:"quantities"`
	} `json:"summary"`
}

func TestSQLiteSaleItemQueryHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sale-item-query.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertSaleItemQueryContract(t, router, "sqlite")
}

func TestPostgresSaleItemQueryHTTPContract(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	pg := createTestDatabase(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertSaleItemQueryContract(t, router, "postgres")
}

func assertSaleItemQueryContract(t *testing.T, router http.Handler, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		return serveJSON(router, method, path, body, token)
	}
	ok := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := call(method, path, body)
		if r.Code != 200 {
			t.Fatalf("%s %s=%d %s", method, path, r.Code, r.Body.String())
		}
		return r
	}
	idOf := func(r *httptest.ResponseRecorder) int64 {
		t.Helper()
		return inventoryData[struct {
			ID int64 `json:"id"`
		}](t, r, 200).ID
	}
	items := func(query string) queryResult {
		t.Helper()
		return inventoryData[queryResult](t, ok(http.MethodGet, "/api/v1/sales/items?"+query, ""), 200)
	}

	// Fixtures: two customers, two goods with different units, one service,
	// mixed and pure service documents, a zero-price line and every status.
	customer := idOf(ok(http.MethodPost, "/api/v1/partners", fmt.Sprintf(`{"code":"SIQ-C-%s","name":"销售查询客户","type":"COMPANY","isCustomer":true}`, dialect)))
	other := idOf(ok(http.MethodPost, "/api/v1/partners", fmt.Sprintf(`{"code":"SIQ-O-%s","name":"销售查询客户乙","type":"COMPANY","isCustomer":true}`, dialect)))
	productA := idOf(ok(http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"SIQ-A-%s","name":"甲商品A","type":"GOODS","unit":"台"}`, dialect)))
	productB := idOf(ok(http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"SIQ-B-%s","name":"乙商品B","type":"GOODS","unit":"支"}`, dialect)))
	service := idOf(ok(http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"SIQ-S-%s","name":"丙服务C","type":"SERVICE","unit":"次"}`, dialect)))
	for _, p := range []struct {
		id   int64
		unit string
	}{{productA, "台"}, {productB, "支"}} {
		adjustment := idOf(ok(http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":%q,"quantity":"100","reason":"OPENING"}]}`, p.id, p.unit)))
		ok(http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", adjustment), `{"version":1}`)
	}
	newSale := func(status, businessDate string, partner int64, body string) (int64, string) {
		t.Helper()
		v := inventoryData[struct {
			ID         int64  `json:"id"`
			DocumentNo string `json:"documentNo"`
			Items      []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}](t, ok(http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":%q,"items":[%s]}`, partner, businessDate, body)), 200)
		switch status {
		case "POSTED":
			ok(http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", v.ID), `{"version":1}`)
		case "CANCELLED":
			ok(http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", v.ID), `{"version":1,"reason":"录错取消"}`)
		}
		return v.ID, v.DocumentNo
	}
	line := func(product int64, kind, unit, quantity, price string) string {
		return fmt.Sprintf(`{"productId":%d,"productType":%q,"unit":%q,"quantity":%q,"unitPrice":%q}`, product, kind, unit, quantity, price)
	}
	saleOne, saleOneNo := newSale("POSTED", "2026-09-10", customer, line(productA, "GOODS", "台", "10", "100.00"))
	saleTwo, _ := newSale("POSTED", "2026-09-10", customer, strings.Join([]string{
		line(productA, "GOODS", "台", "3", "90.00"),
		line(productB, "GOODS", "支", "5", "2.50"),
		line(service, "SERVICE", "次", "1", "50.00"),
	}, ","))
	newSale("DRAFT", "2026-10-01", other, line(productA, "GOODS", "台", "2", "100.00"))
	newSale("CANCELLED", "2026-10-02", other, line(productA, "GOODS", "台", "4", "100.00"))
	newSale("POSTED", "2026-10-05", other, line(service, "SERVICE", "次", "2", "30.00"))
	newSale("POSTED", "2026-10-06", customer, line(productA, "GOODS", "台", "1", "0.00"))

	// Sale returns are keyed to the original line: one posted, one draft, one
	// posted and then cancelled, plus partial returns of two priced lines.
	saleLines := func(saleID int64) []int64 {
		t.Helper()
		v := inventoryData[struct {
			Items []struct {
				SaleItemID int64 `json:"saleItemId"`
			} `json:"items"`
		}](t, ok(http.MethodGet, fmt.Sprintf("/api/v1/sale-returns/source/%d", saleID), ""), 200)
		out := make([]int64, 0, len(v.Items))
		for _, item := range v.Items {
			out = append(out, item.SaleItemID)
		}
		return out
	}
	newReturn := func(saleID, itemID int64, businessDate, quantity string) int64 {
		t.Helper()
		return idOf(ok(http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":%q,"items":[{"saleItemId":%d,"quantity":%q}]}`, saleID, businessDate, itemID, quantity)))
	}
	oneLines, twoLines := saleLines(saleOne), saleLines(saleTwo)
	postedReturn := newReturn(saleOne, oneLines[0], "2026-10-20", "2")
	ok(http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", postedReturn), `{"version":1}`)
	draftReturn := newReturn(saleOne, oneLines[0], "2026-10-21", "3")
	_ = draftReturn
	cancelledReturn := newReturn(saleOne, oneLines[0], "2026-10-22", "1")
	ok(http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", cancelledReturn), `{"version":1}`)
	ok(http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", cancelledReturn), `{"version":2,"reason":"取消退货"}`)
	ok(http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", newReturn(saleTwo, twoLines[0], "2026-10-23", "1")), `{"version":1}`)
	ok(http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", newReturn(saleTwo, twoLines[1], "2026-10-24", "2")), `{"version":1}`)

	// The whole range: eight saved lines, effective returns only, and a summary
	// that covers every matching line rather than the current page.
	all := items("pageSize=50")
	if all.Total != 8 || len(all.Records) != 8 {
		t.Fatalf("total=%d rows=%d", all.Total, len(all.Records))
	}
	if got := []string{all.Records[0].DocumentNo, all.Records[1].Status}; all.Records[0].BusinessDate != "2026-10-06" || got[1] != "POSTED" {
		t.Fatalf("default order must be the newest business date first: %+v", all.Records[0])
	}
	first, last := all.Records[0], all.Records[7]
	if first.Quantity != "1" || first.Amount != "0.00" || first.NetAmount == nil || *first.NetAmount != "0.00" || first.ReturnsApplicable != true {
		t.Fatalf("zero-price posted line: %+v", first)
	}
	if last.SaleID != saleOne || last.DocumentNo != saleOneNo || last.Quantity != "10" || last.UnitPrice != "100.00" || last.Amount != "1000.00" {
		t.Fatalf("original line changed: %+v", last)
	}
	if !last.ReturnsApplicable || last.ReturnedQuantity != "2" || last.ReturnedAmount != "200.00" {
		t.Fatalf("effective returns must ignore draft and cancelled returns: %+v", last)
	}
	if !last.NetApplicable || last.NetQuantity == nil || *last.NetQuantity != "8" || last.NetAmount == nil || *last.NetAmount != "800.00" {
		t.Fatalf("net of a posted line: %+v", last)
	}
	for _, row := range all.Records {
		switch row.Status {
		case "POSTED":
			if !row.NetApplicable || row.NetQuantity == nil || row.NetAmount == nil {
				t.Fatalf("posted line without net: %+v", row)
			}
		default:
			if row.NetApplicable || row.NetQuantity != nil || row.NetAmount != nil {
				t.Fatalf("draft or cancelled line must not report a net: %+v", row)
			}
			if row.Quantity != "2" && row.Quantity != "4" {
				t.Fatalf("draft and cancelled keep their original quantity: %+v", row)
			}
		}
		if row.ProductType == "SERVICE" && (row.ReturnsApplicable || row.ReturnedQuantity != "0" || row.ReturnedAmount != "0.00") {
			t.Fatalf("service returns are not applicable: %+v", row)
		}
	}
	for _, row := range all.Records {
		if row.ProductType == "SERVICE" && row.NetQuantity != nil && *row.NetQuantity != row.Quantity {
			t.Fatalf("service net must equal its original quantity: %+v", row)
		}
	}
	wantAmounts := []queryAmounts{
		{Status: "DRAFT", ItemCount: 1, OriginalAmount: "200.00"},
		{Status: "POSTED", ItemCount: 6, OriginalAmount: "1392.50"},
		{Status: "CANCELLED", ItemCount: 1, OriginalAmount: "400.00"},
	}
	for i, want := range wantAmounts {
		got := all.Summary.Amounts[i]
		if got.Status != want.Status || got.ItemCount != want.ItemCount || got.OriginalAmount != want.OriginalAmount {
			t.Fatalf("amounts[%d]=%+v want %+v", i, got, want)
		}
		if want.Status == "POSTED" {
			if got.ReturnedAmount == nil || *got.ReturnedAmount != "295.00" || got.NetAmount == nil || *got.NetAmount != "1097.50" {
				t.Fatalf("posted returns/net: %+v", got)
			}
			continue
		}
		if got.ReturnedAmount != nil || got.NetAmount != nil {
			t.Fatalf("returns and net only apply to posted sales: %+v", got)
		}
	}
	wantQuantities := []struct {
		status, unit, quantity string
		product                int64
		returned, net          *string
	}{
		{status: "DRAFT", product: productA, unit: "台", quantity: "2", returned: strOf("0")},
		{status: "POSTED", product: productA, unit: "台", quantity: "14", returned: strOf("3"), net: strOf("11")},
		{status: "POSTED", product: productB, unit: "支", quantity: "5", returned: strOf("2"), net: strOf("3")},
		{status: "POSTED", product: service, unit: "次", quantity: "3", net: strOf("3")},
		{status: "CANCELLED", product: productA, unit: "台", quantity: "4", returned: strOf("0")},
	}
	if len(all.Summary.Quantities) != len(wantQuantities) {
		t.Fatalf("quantity groups=%+v", all.Summary.Quantities)
	}
	for i, want := range wantQuantities {
		got := all.Summary.Quantities[i]
		if got.Status != want.status || got.ProductID != want.product || got.Unit != want.unit || got.Quantity != want.quantity {
			t.Fatalf("quantities[%d]=%+v want %+v", i, got, want)
		}
		if (got.ReturnedQuantity == nil) != (want.returned == nil) || got.ReturnedQuantity != nil && *got.ReturnedQuantity != *want.returned {
			t.Fatalf("quantities[%d] returned=%v want %v", i, got.ReturnedQuantity, want.returned)
		}
		if (got.NetQuantity == nil) != (want.net == nil) || got.NetQuantity != nil && *got.NetQuantity != *want.net {
			t.Fatalf("quantities[%d] net=%v want %v", i, got.NetQuantity, want.net)
		}
		if got.ReturnsApplicable != (want.returned != nil) {
			t.Fatalf("quantities[%d] applicable=%v", i, got.ReturnsApplicable)
		}
	}

	// A stable sort per field, with lines whose net does not apply behind the
	// effective ones, and the same summary on every page.
	assertSorted := func(query, field string, ascending bool) []queryItem {
		t.Helper()
		page := items(query)
		previous := int64(0)
		compared := false
		seenInapplicable := false
		for _, row := range page.Records {
			// A line whose net does not apply has no net to compare; it only has
			// to stay behind every line that has one.
			if field == "netAmount" && !row.NetApplicable {
				seenInapplicable = true
				continue
			}
			if seenInapplicable {
				t.Fatalf("lines without an applicable net must sort last: %+v", page.Records)
			}
			value := fixedThousandths(t, queryField(row, field))
			if compared && (ascending && value < previous || !ascending && value > previous) {
				t.Fatalf("%s not sorted: %+v", query, page.Records)
			}
			previous, compared = value, true
		}
		return page.Records
	}
	if first := assertSorted("pageSize=50&sortBy=unitPrice&sortDir=desc", "unitPrice", false)[0]; first.UnitPrice != "100.00" {
		t.Fatalf("descending unit price starts at %s", first.UnitPrice)
	}
	assertSorted("pageSize=50&sortBy=unitPrice&sortDir=asc", "unitPrice", true)
	assertSorted("pageSize=50&sortBy=amount&sortDir=asc", "amount", true)
	assertSorted("pageSize=50&sortBy=amount&sortDir=desc", "amount", false)
	paged := assertSorted("pageSize=50&sortBy=netAmount&sortDir=desc", "netAmount", false)
	if !paged[0].NetApplicable || paged[len(paged)-1].NetApplicable {
		t.Fatalf("net sorting must separate the not applicable lines: %+v", paged)
	}
	assertSorted("pageSize=50&sortBy=netAmount&sortDir=asc", "netAmount", true)

	summary := jsonOf(t, all.Summary)
	seen := map[int64]bool{}
	var collected []int64
	for page := 1; page <= 4; page++ {
		part := items(fmt.Sprintf("pageSize=2&page=%d", page))
		if jsonOf(t, part.Summary) != summary {
			t.Fatalf("page %d summary changed: %+v", page, part.Summary)
		}
		if part.Total != 8 {
			t.Fatalf("page %d total=%d", page, part.Total)
		}
		for _, row := range part.Records {
			if seen[row.ID] {
				t.Fatalf("item %d repeated across pages", row.ID)
			}
			seen[row.ID] = true
			collected = append(collected, row.ID)
		}
	}
	for i, row := range all.Records {
		if collected[i] != row.ID {
			t.Fatalf("paged ids %v differ from the full order %v", collected, all.Records)
		}
	}

	// Filters keep documents and lines together: a product keyword only matches
	// its own lines, and the summary follows the same condition.
	byProduct := items(fmt.Sprintf("pageSize=50&productId=%d", productA))
	if byProduct.Total != 5 {
		t.Fatalf("product filter total=%d", byProduct.Total)
	}
	if byProduct.Summary.Amounts[1].OriginalAmount != "1270.00" || byProduct.Summary.Amounts[1].ReturnedAmount == nil || *byProduct.Summary.Amounts[1].ReturnedAmount != "290.00" {
		t.Fatalf("product summary: %+v", byProduct.Summary.Amounts)
	}
	for _, row := range byProduct.Records {
		if row.ProductID != productA {
			t.Fatalf("other lines leaked into the product filter: %+v", row)
		}
	}
	byKeyword := items(fmt.Sprintf("pageSize=50&productKeyword=%s", "乙商品B"))
	if byKeyword.Total != 1 || byKeyword.Summary.Amounts[1].OriginalAmount != "12.50" {
		t.Fatalf("product keyword must not add other lines of the same document: %+v", byKeyword)
	}
	byDocument := items(fmt.Sprintf("pageSize=50&documentNo=%s", saleOneNo))
	if byDocument.Total != 1 || byDocument.Records[0].SaleID != saleOne {
		t.Fatalf("document number filter: %+v", byDocument)
	}
	byPartner := items(fmt.Sprintf("pageSize=50&partnerId=%d&status=POSTED", customer))
	if byPartner.Total != 5 {
		t.Fatalf("partner filter total=%d", byPartner.Total)
	}
	byDates := items("pageSize=50&businessFrom=2026-09-10&businessTo=2026-09-10")
	if byDates.Total != 4 {
		t.Fatalf("date filter total=%d", byDates.Total)
	}
	empty := items("pageSize=50&businessFrom=2027-01-01")
	if empty.Total != 0 || len(empty.Records) != 0 || len(empty.Summary.Quantities) != 0 {
		t.Fatalf("empty result: %+v", empty)
	}
	for _, amounts := range empty.Summary.Amounts {
		if amounts.ItemCount != 0 || amounts.OriginalAmount != "0.00" {
			t.Fatalf("empty summary: %+v", empty.Summary.Amounts)
		}
	}
	for _, query := range []string{"sortBy=id", "sortDir=sideways", "pageSize=0", "page=0", "status=UNKNOWN", "businessFrom=2026-02-30", "businessFrom=2026-10-02&businessTo=2026-10-01", fmt.Sprintf("productId=%d&productKeyword=x", productA)} {
		if r := call(http.MethodGet, "/api/v1/sales/items?"+query, ""); r.Code != 400 {
			t.Fatalf("%s=%d %s", query, r.Code, r.Body.String())
		}
	}

	// Renaming and stopping an archive never relabels saved lines or splits one
	// product identity into two quantity groups.
	ok(http.MethodPut, fmt.Sprintf("/api/v1/products/%d", productA), fmt.Sprintf(`{"code":"SIQ-A-%s","name":"新甲商品Z","type":"GOODS","unit":"台"}`, dialect))
	ok(http.MethodPut, fmt.Sprintf("/api/v1/products/%d/status", productB), `{"status":0}`)
	renamed := items(fmt.Sprintf("pageSize=50&productKeyword=%s", "新甲商品Z"))
	if renamed.Total != 5 || len(renamed.Summary.Quantities) != 3 {
		t.Fatalf("renamed archive: total=%d groups=%+v", renamed.Total, renamed.Summary.Quantities)
	}
	for _, row := range renamed.Records {
		if row.ProductName != "甲商品A" || row.ProductCode != "SIQ-A-"+strings.ToUpper(dialect) || row.MatchesHistory {
			t.Fatalf("saved snapshot changed by a rename: %+v", row)
		}
	}
	for _, group := range renamed.Summary.Quantities {
		if group.ProductName != "甲商品A" || group.ProductID != productA {
			t.Fatalf("summary split one identity: %+v", renamed.Summary.Quantities)
		}
	}
	historical := items(fmt.Sprintf("pageSize=50&productKeyword=%s", "甲商品A"))
	if historical.Total != 5 {
		t.Fatalf("snapshot keyword total=%d", historical.Total)
	}
	for _, row := range historical.Records {
		if !row.MatchesHistory {
			t.Fatalf("snapshot-only match must be marked: %+v", row)
		}
	}
	stopped := items(fmt.Sprintf("pageSize=50&productId=%d", productB))
	if stopped.Total != 1 || stopped.Records[0].ProductName != "乙商品B" {
		t.Fatalf("stopped archive: %+v", stopped)
	}

	// Whole-document markers and item queries use the same server predicate.
	// Current brand, mixed current/snapshot words, case and literal wildcards
	// must highlight only A, while keeping every other saved line in the sale.
	ok(http.MethodPut, fmt.Sprintf("/api/v1/products/%d", productA), `{"name":"新甲商品Z","brand":"Brand%_X","model":"NEW-Model","type":"GOODS","unit":"台"}`)
	assertMarkers := func(condition string) {
		t.Helper()
		page := inventoryData[struct {
			Total   int64 `json:"total"`
			Records []struct {
				ID    int64 `json:"id"`
				Items []struct {
					ID             int64  `json:"id"`
					ProductID      int64  `json:"productId"`
					ProductName    string `json:"productName"`
					MatchesProduct bool   `json:"matchesProduct"`
				} `json:"items"`
			} `json:"records"`
		}](t, ok(http.MethodGet, "/api/v1/sales?pageSize=50&"+condition, ""), 200)
		matchedIDs := map[int64]bool{}
		foundMixed := false
		for _, doc := range page.Records {
			if doc.ID == saleTwo {
				foundMixed = true
				if len(doc.Items) != 3 {
					t.Fatalf("must preserve the whole mixed sale: %+v", doc)
				}
			}
			for _, row := range doc.Items {
				if row.MatchesProduct != (row.ProductID == productA) {
					t.Fatalf("incorrect line marker for %s: %+v", condition, row)
				}
				if row.MatchesProduct {
					matchedIDs[row.ID] = true
					if row.ProductName != "甲商品A" {
						t.Fatalf("marker overwrote snapshot: %+v", row)
					}
				}
			}
		}
		matchingItems := items("pageSize=50&" + condition)
		if !foundMixed || len(matchedIDs) != 5 || matchingItems.Total != 5 {
			t.Fatalf("document/item matching disagrees: %+v total=%d", matchedIDs, matchingItems.Total)
		}
		for _, row := range matchingItems.Records {
			if !matchedIDs[row.ID] {
				t.Fatalf("item not marked in document: %+v", row)
			}
		}
	}
	assertMarkers(fmt.Sprintf("productId=%d", productA))
	for _, keyword := range []string{"甲商品A", "新甲商品Z", "brand%_x", "new-model", "Brand%_X 甲商品A", "SIQ-A-" + strings.ToUpper(dialect)} {
		assertMarkers("productKeyword=" + url.QueryEscape(keyword))
	}
	unfiltered := inventoryData[struct {
		Records []struct {
			Items []struct {
				MatchesProduct bool `json:"matchesProduct"`
			} `json:"items"`
		} `json:"records"`
	}](t, ok(http.MethodGet, "/api/v1/sales?pageSize=50", ""), 200)
	for _, doc := range unfiltered.Records {
		for _, row := range doc.Items {
			if row.MatchesProduct {
				t.Fatal("unfiltered rows must not be marked")
			}
		}
	}

	// Selecting by the original sale business date keeps a later return in the
	// accounting of its own line, and cancelling that return restores the net.
	september := items(fmt.Sprintf("pageSize=50&productId=%d&businessFrom=2026-09-01&businessTo=2026-09-30", productA))
	if september.Total != 2 || september.Summary.Amounts[1].ItemCount != 2 {
		t.Fatalf("september only holds the two posted sales: %+v", september.Summary.Amounts)
	}
	if september.Summary.Amounts[1].ReturnedAmount == nil || *september.Summary.Amounts[1].ReturnedAmount != "290.00" || *september.Summary.Amounts[1].NetAmount != "980.00" {
		t.Fatalf("cross-month returns belong to their line: %+v", september.Summary.Amounts[1])
	}
	ok(http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", postedReturn), `{"version":2,"reason":"客户撤销退货"}`)
	restored := items(fmt.Sprintf("pageSize=50&productId=%d&businessFrom=2026-09-01&businessTo=2026-09-30", productA))
	if restored.Summary.Amounts[1].ReturnedAmount == nil || *restored.Summary.Amounts[1].ReturnedAmount != "90.00" || *restored.Summary.Amounts[1].NetAmount != "1180.00" {
		t.Fatalf("cancelling a return must restore the net: %+v", restored.Summary.Amounts[1])
	}
	if got := restored.Records[1]; got.SaleID != saleOne || got.ReturnedQuantity != "0" || got.ReturnedAmount != "0.00" || *got.NetQuantity != "10" || *got.NetAmount != "1000.00" {
		t.Fatalf("cancelled return still counted: %+v", got)
	}
}

func strOf(v string) *string { return &v }

func jsonOf(t *testing.T, value any) string {
	t.Helper()
	raw, e := json.Marshal(value)
	if e != nil {
		t.Fatal(e)
	}
	return string(raw)
}

func queryField(row queryItem, field string) string {
	switch field {
	case "unitPrice":
		return row.UnitPrice
	case "amount":
		return row.Amount
	case "netAmount":
		return *row.NetAmount
	}
	return row.BusinessDate
}

// fixedThousandths compares decimals by value instead of by text.
func fixedThousandths(t *testing.T, value string) int64 {
	t.Helper()
	whole, fraction := value, ""
	if i := strings.IndexByte(value, '.'); i >= 0 {
		whole, fraction = value[:i], value[i+1:]
	}
	if len(fraction) > 3 {
		fraction = fraction[:3]
	}
	for len(fraction) < 3 {
		fraction += "0"
	}
	number, e := strconv.ParseInt(whole+fraction, 10, 64)
	if e != nil {
		t.Fatalf("not a decimal: %q", value)
	}
	return number
}
