//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/xuri/excelize/v2"
)

func TestSQLiteExcelExportHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exports.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	assertExcelExportContract(t, router, db)
}
func TestPostgresExcelExportHTTPContract(t *testing.T) {
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
	assertExcelExportContract(t, router, db)
}

func assertExcelExportContract(t *testing.T, router http.Handler, db *platformdatabase.Database) {
	token := loginAdmin(t, router)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := serveJSON(router, method, path, body, token)
		if r.Code != 200 {
			t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body.String())
		}
		return r
	}
	idOf := func(r *httptest.ResponseRecorder) int64 {
		return inventoryData[struct {
			ID int64 `json:"id"`
		}](t, r, 200).ID
	}
	partner := idOf(call("POST", "/api/v1/partners", `{"code":"EXPORT-P","name":"对账公司甲","type":"COMPANY","isCustomer":true,"isSupplier":true}`))
	other := idOf(call("POST", "/api/v1/partners", `{"code":"EXPORT-O","name":"对账公司乙","type":"COMPANY","isCustomer":true,"isSupplier":true}`))
	goods := idOf(call("POST", "/api/v1/products", `{"code":"001","name":"历史商品","type":"GOODS","unit":"台","model":"M1","specification":"8G"}`))
	service := idOf(call("POST", "/api/v1/products", `{"code":"002","name":"安装","type":"SERVICE","unit":"次"}`))
	adjustment := idOf(call("POST", "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"100","reason":"OPENING"}]}`, goods)))
	call("POST", fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", adjustment), `{"version":1}`)

	line := func(product int64, kind, unit, quantity, price string) string {
		return fmt.Sprintf(`{"productId":%d,"productType":%q,"unit":%q,"quantity":%q,"unitPrice":%q}`, product, kind, unit, quantity, price)
	}
	var purchaseID, saleID, purchaseLine, saleLine int64
	for _, module := range []string{"purchases", "sales"} {
		body := line(goods, "GOODS", "台", "12.5", "2.40")
		if module == "sales" {
			body += "," + line(service, "SERVICE", "次", "1", "10.00")
		}
		document := inventoryData[struct {
			ID    int64
			Items []struct{ ID int64 }
		}](t, call("POST", "/api/v1/"+module, fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-08-01","items":[%s]}`, partner, body)), 200)
		call("POST", fmt.Sprintf("/api/v1/%s/%d/post", module, document.ID), `{"version":1}`)
		if module == "purchases" {
			purchaseID, purchaseLine = document.ID, document.Items[0].ID
		} else {
			saleID, saleLine = document.ID, document.Items[0].ID
		}
		for _, status := range []string{"DRAFT", "CANCELLED"} {
			draft := idOf(call("POST", "/api/v1/"+module, fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-09-01","items":[%s]}`, other, line(goods, "GOODS", "台", "1", "3.00"))))
			if status == "CANCELLED" {
				call("POST", fmt.Sprintf("/api/v1/%s/%d/cancel", module, draft), `{"version":1,"reason":"录错"}`)
			}
		}
	}
	// Return after the original business period: only still posted returns count.
	for _, ret := range []struct {
		module, source, item string
		doc, line            int64
	}{{"purchase", "purchaseId", "purchaseItemId", purchaseID, purchaseLine}, {"sale", "saleId", "saleItemId", saleID, saleLine}} {
		for _, status := range []string{"POSTED", "DRAFT", "CANCELLED"} {
			id := idOf(call("POST", "/api/v1/"+ret.module+"-returns", fmt.Sprintf(`{"%s":%d,"businessDate":"2026-10-01","items":[{"%s":%d,"quantity":"0.5"}]}`, ret.source, ret.doc, ret.item, ret.line)))
			if status != "DRAFT" {
				call("POST", fmt.Sprintf("/api/v1/%s-returns/%d/post", ret.module, id), `{"version":1}`)
			}
			if status == "CANCELLED" {
				call("POST", fmt.Sprintf("/api/v1/%s-returns/%d/cancel", ret.module, id), `{"version":2,"reason":"退货录错"}`)
			}
		}
	}
	direct := idOf(call("POST", "/api/v1/direct-orders", fmt.Sprintf(`{"requestKey":"export-direct","supplierId":%d,"customerId":%d,"businessDate":"2026-08-02","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","purchasePrice":"3.00","salePrice":"4.00"}]}`, partner, partner, goods)))
	call("POST", fmt.Sprintf("/api/v1/direct-orders/%d/post", direct), `{"version":1}`)
	call("PUT", fmt.Sprintf("/api/v1/products/%d", goods), `{"code":"001","name":"当前商品","type":"GOODS","unit":"台","model":"M2","specification":"16G"}`)
	opening := idOf(call("POST", "/api/v1/partner-balances/opening", fmt.Sprintf(`{"requestKey":"export-opening","partnerId":%d,"direction":"CUSTOMER","amount":"1.23","businessDate":"2026-07-01","description":"期初欠款"}`, other)))
	call("POST", fmt.Sprintf("/api/v1/partner-balances/entries/%d/reverse", opening), `{"reason":"期初录错"}`)

	export := func(path, filename string) *excelize.File {
		t.Helper()
		r := call("GET", path, "")
		if r.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
			t.Fatal(r.Header())
		}
		if !strings.Contains(r.Header().Get("Access-Control-Expose-Headers"), "Content-Disposition") {
			t.Fatal("filename inaccessible to cross-origin frontend")
		}
		disposition := r.Header().Get("Content-Disposition")
		prefix := "attachment; filename*=UTF-8''"
		if !strings.HasPrefix(disposition, prefix) {
			t.Fatal(disposition)
		}
		name, err := url.QueryUnescape(strings.TrimPrefix(disposition, prefix))
		if err != nil || !strings.Contains(name, filename) {
			t.Fatalf("filename=%q, want %q: %v", name, filename, err)
		}
		book, err := excelize.OpenReader(bytes.NewReader(r.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { book.Close() })
		if !reflect.DeepEqual(book.GetSheetList(), []string{"明细", "合计"}) {
			t.Fatal(book.GetSheetList())
		}
		return book
	}
	raw := func(book *excelize.File, sheet, cell string) string {
		t.Helper()
		v, err := book.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	check := func(book *excelize.File, sheet, cell, want string) {
		t.Helper()
		if got := raw(book, sheet, cell); got != want {
			t.Fatalf("%s!%s=%q want %q", sheet, cell, got, want)
		}
	}
	numeric := func(book *excelize.File, cell string) {
		t.Helper()
		typ, err := book.GetCellType("明细", cell)
		if err != nil || (typ != excelize.CellTypeNumber && typ != excelize.CellTypeUnset) {
			t.Fatalf("%s must be numeric, type=%v err=%v", cell, typ, err)
		}
		if _, err := strconv.ParseFloat(raw(book, "明细", cell), 64); err != nil {
			t.Fatalf("%s missing numeric value: %v", cell, err)
		}
	}
	text := func(book *excelize.File, cell string) {
		t.Helper()
		typ, err := book.GetCellType("明细", cell)
		if err != nil || (typ != excelize.CellTypeSharedString && typ != excelize.CellTypeInlineString) {
			t.Fatalf("%s must be text, type=%v err=%v", cell, typ, err)
		}
	}
	before := exportDatabaseState(t, db)
	for _, module := range []string{"purchases", "sales"} {
		label := "采购"
		last, amount := "Q", "M"
		ncols := 17
		if module == "sales" {
			label, last, amount, ncols = "销售", "R", "N", 18
		}
		path := "/api/v1/" + module + "/items"
		book := export(path+"/export?businessFrom=2026-08-01&businessTo=2026-10-12&page=99&pageSize=1&sortBy=unitPrice&sortDir=asc", label+"明细_20260801-20261012.xlsx")
		rows, err := book.GetRows("明细")
		if err != nil {
			t.Fatal(err)
		}
		wantCount := 4
		if module == "sales" {
			wantCount = 5
		}
		if len(rows) != wantCount+2 || len(rows[0]) != ncols {
			t.Fatalf("%s rows=%v", module, rows)
		}
		check(book, "明细", "A1", label+"单号")
		check(book, "明细", "D2", "2026-08-01")
		check(book, "明细", "F2", "001")
		check(book, "明细", "G2", "历史商品")
		check(book, "明细", "H2", "M1")
		check(book, "明细", "I2", "8G")
		check(book, "明细", last+"2", "28.80")
		check(book, "明细", "C5", "直送")
		check(book, "明细", fmt.Sprintf("B%d", len(rows)), fmt.Sprintf("共 %d 行", wantCount))
		if module == "purchases" {
			check(book, "明细", "N2", "0.5")
			check(book, "明细", "O2", "1.20")
			check(book, "明细", "P2", "12")
			check(book, "明细", "Q3", "")
			check(book, "明细", "Q4", "")
			numeric(book, "K2")
			check(book, "明细", amount+strconv.Itoa(len(rows)), "42.00")
		} else {
			check(book, "明细", "O2", "0.5")
			check(book, "明细", "P2", "1.20")
			check(book, "明细", "Q2", "12")
			check(book, "明细", "R3", "")
			check(book, "明细", "R4", "")
			check(book, "明细", "J6", "服务")
			check(book, "明细", "O6", "")
			check(book, "明细", "P6", "")
			check(book, "明细", "R6", "10.00")
			numeric(book, "L2")
			check(book, "明细", amount+strconv.Itoa(len(rows)), "54.00")
		}
		numeric(book, amount+"2")
		text(book, "D2")
		text(book, "F2")
		check(book, "合计", "A16", "状态")
		check(book, "合计", "A17", "草稿")
		check(book, "合计", "A18", "已过账")
		check(book, "合计", "A19", "已取消")
		check(book, "合计", "D17", "")
		check(book, "合计", "D18", "1.20")
		check(book, "合计", "A21", "状态")
		check(book, "合计", "D23", "台")
		check(book, "合计", "E23", "14.5")
		check(book, "合计", "F23", "0.5")
		check(book, "合计", "G23", "14")
		if raw(book, "合计", "B2") == "" {
			t.Fatal("missing exporter")
		}
		// Every sorting field and direction follows the corresponding list exactly.
		for _, field := range []string{"businessDate", "unitPrice", "amount", "netAmount"} {
			for _, dir := range []string{"asc", "desc"} {
				query := "sortBy=" + field + "&sortDir=" + dir
				list := inventoryData[struct{ Records []struct{ DocumentNo string } }](t, call("GET", path+"?pageSize=100&"+query, ""), 200)
				sorted := export(path+"/export?"+query, label+"明细_全部_")
				for i, r := range list.Records {
					check(sorted, "明细", fmt.Sprintf("A%d", i+2), r.DocumentNo)
				}
			}
		}
		empty := export(path+"/export?productKeyword=不存在", label+"明细_全部_")
		emptyRows, _ := empty.GetRows("明细")
		if len(emptyRows) != 2 {
			t.Fatal(emptyRows)
		}
		for _, query := range []string{"status=INVALID", "sortBy=sql", "businessFrom=bad", "businessFrom=2026-10-12&businessTo=2026-01-01", "partnerId=-1", "page=bad", "partnerId=1&partnerKeyword=甲"} {
			r := serveJSON(router, "GET", path+"/export?"+query, "", token)
			if r.Code != 400 {
				t.Fatalf("invalid %s %s=%d %s", module, query, r.Code, r.Body.String())
			}
		}
		filtered := export(path+fmt.Sprintf("/export?partnerId=%d&directDelivery=true", partner), label+"明细_全部_")
		filterRows, _ := filtered.GetRows("明细")
		if len(filterRows) != 3 {
			t.Fatal(filterRows)
		}
	}
	ledgerPath := "/api/v1/partner-balances/entries"
	query := fmt.Sprintf("partnerId=%d&direction=CUSTOMER&from=2000-01-01T00:00:00Z&to=2100-01-01T00:00:00Z", partner)
	period := export(ledgerPath+"/export?"+query, "往来明细_客户_对账公司甲_20000101-21000101.xlsx")
	statement := inventoryData[receivable.Statement](t, call("GET", "/api/v1/partner-balances/statement?"+query, ""), 200)
	check(period, "合计", "A16", "期间期初")
	check(period, "合计", "A17", statement.OpeningAmount)
	check(period, "合计", "B17", statement.NetChange)
	check(period, "合计", "C17", statement.ClosingAmount)
	ledger := inventoryData[receivable.Page](t, call("GET", ledgerPath+"?"+query+"&pageSize=500", ""), 200)
	for i, r := range ledger.Records {
		row := strconv.Itoa(i + 2)
		check(period, "明细", "D"+row, r.DocumentNo)
		check(period, "明细", "H"+row, r.Amount)
		check(period, "明细", "I"+row, r.BalanceBefore)
		check(period, "明细", "J"+row, r.BalanceAfter)
		numeric(period, "H"+row)
		text(period, "E"+row)
		text(period, "F"+row)
	}
	multiple := export(ledgerPath+"/export?partnerKeyword="+url.QueryEscape("对账公司"), "往来明细_关键词_对账公司_")
	check(multiple, "合计", "A16", "往来单位")
	check(multiple, "合计", "A17", "对账公司甲")
	check(multiple, "合计", "A19", "对账公司乙")
	check(multiple, "合计", "C19", "2")
	check(multiple, "合计", "D19", "0.00")
	multipleRows, _ := multiple.GetRows("明细")
	hasReversal := false
	for _, r := range multipleRows {
		if len(r) > 10 && r[2] == "取消 / 往来冲销" && r[10] != "" {
			hasReversal = true
		}
	}
	if !hasReversal {
		t.Fatal("missing reversal link")
	}
	noPeriod := export(ledgerPath+fmt.Sprintf("/export?partnerId=%d&direction=CUSTOMER", partner), "往来明细_客户_对账公司甲_全部_")
	check(noPeriod, "合计", "A16", "往来单位")
	partial := export(ledgerPath+fmt.Sprintf("/export?partnerId=%d&direction=CUSTOMER&from=2000-01-01T00:00:00Z", partner), "20000101-全部_")
	check(partial, "合计", "A16", "往来单位")
	emptyLedger := export(ledgerPath+"/export?partnerKeyword=不存在", "往来明细_关键词_不存在_")
	emptyRows, _ := emptyLedger.GetRows("明细")
	if len(emptyRows) != 2 {
		t.Fatal(emptyRows)
	}
	if after := exportDatabaseState(t, db); after != before {
		t.Fatalf("export changed business data\nbefore=%s\nafter=%s", before, after)
	}
	// Populate the isolated database directly only for the upper-limit fixture;
	// assertions still exercise the public HTTP endpoint, never internal helpers.
	for _, module := range []string{"purchase", "sale"} {
		columns := "document_id,product_id,product_code,product_name,product_model,product_specification,product_type,unit,quantity_milli,unit_price_cents,amount_cents,remark"
		sql := fmt.Sprintf(`WITH digit(n) AS (VALUES(0),(1),(2),(3),(4),(5),(6),(7),(8),(9)) INSERT INTO %s_document_item (%s) SELECT %s FROM %s_document_item original CROSS JOIN digit a CROSS JOIN digit b CROSS JOIN digit c CROSS JOIN digit d CROSS JOIN digit e WHERE original.id=?`, module, columns, columns, module)
		// Qualify source columns to avoid ambiguous names as fixtures evolve.
		sql = strings.Replace(sql, "SELECT "+columns, "SELECT original."+strings.ReplaceAll(columns, ",", ",original."), 1)
		lineID := purchaseLine
		if module == "sale" {
			lineID = saleLine
		}
		if err := db.GORM.Exec(sql, lineID).Error; err != nil {
			t.Fatal(err)
		}
		r := serveJSON(router, "GET", "/api/v1/"+module+"s/items/export", "", token)
		assertExportLimit(t, r)
	}
	// Unique source numbers and ids respect the ledger's schema constraints.
	sql := `WITH digit(n) AS (VALUES(0),(1),(2),(3),(4),(5),(6),(7),(8),(9)), numbers(n) AS (SELECT a.n+10*b.n+100*c.n+1000*d.n+10000*e.n FROM digit a CROSS JOIN digit b CROSS JOIN digit c CROSS JOIN digit d CROSS JOIN digit e) INSERT INTO partner_balance_entry (id,partner_id,direction,entry_type,amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,document_no,operator_id) SELECT 1000000+n,partner_id,direction,'OPENING',amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,'EXPORT-LIMIT-' || CAST(n AS TEXT),operator_id FROM partner_balance_entry original CROSS JOIN numbers WHERE original.id=?`
	if err := db.GORM.Exec(sql, opening).Error; err != nil {
		t.Fatal(err)
	}
	assertExportLimit(t, serveJSON(router, "GET", ledgerPath+"/export", "", token))
	// No authentication means no workbook, matching the existing list endpoints.
	for _, path := range []string{"/api/v1/purchases/items/export", "/api/v1/sales/items/export", ledgerPath + "/export"} {
		if r := serveJSON(router, "GET", path, "", ""); r.Code != 401 {
			t.Fatalf("unauthenticated export=%d", r.Code)
		}
	}
}
func assertExportLimit(t *testing.T, r *httptest.ResponseRecorder) {
	t.Helper()
	if r.Code < 400 || r.Code >= 500 || !strings.Contains(r.Body.String(), "匹配行数超过导出上限（100000 行），请缩小查询范围") {
		t.Fatalf("upper limit=%d %s", r.Code, r.Body.String())
	}
}
func exportDatabaseState(t *testing.T, db *platformdatabase.Database) string {
	t.Helper()
	state := map[string]any{}
	for _, table := range []string{"purchase_document", "purchase_document_item", "sale_document", "sale_document_item", "inventory_balance", "inventory_entry", "partner_balance", "partner_balance_entry", "sys_oper_log"} {
		var rows []map[string]any
		if err := db.GORM.Table(table).Order("id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		state[table] = rows
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
