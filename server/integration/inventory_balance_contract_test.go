//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

func TestSQLiteInventoryBalanceContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory-balances.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryBalanceContract(t, router, db)
}
func TestPostgresInventoryBalanceContract(t *testing.T) {
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
	assertInventoryBalanceContract(t, router, db)
}

type balanceDTO struct {
	ProductID     int64   `json:"productId"`
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Model         *string `json:"model"`
	Specification *string `json:"specification"`
	Category      *string `json:"category"`
	Unit          string  `json:"unit"`
	Status        int     `json:"status"`
	Quantity      string  `json:"quantity"`
}
type balancePageDTO struct {
	Records  []balanceDTO `json:"records"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}

func assertInventoryBalanceContract(t *testing.T, router http.Handler, db *platformdatabase.Database) {
	t.Helper()
	token := loginAdmin(t, router)
	const balancePath = "/api/v1/inventory/balances"
	page := func(query string) balancePageDTO {
		return inventoryData[balancePageDTO](t, serveJSON(router, http.MethodGet, balancePath+query, "", token), 200)
	}
	codes := func(result balancePageDTO) []string {
		out := make([]string, 0, len(result.Records))
		for _, record := range result.Records {
			out = append(out, record.Code)
		}
		return out
	}
	balances := func() int64 {
		var count int64
		if e := db.GORM.Table("inventory_balance").Count(&count).Error; e != nil {
			t.Fatal(e)
		}
		return count
	}
	products, units := map[string]int64{}, map[string]string{}
	for _, spec := range []struct{ code, kind, unit, extra string }{
		{"BAL-A", "GOODS", "台", `,"category":"耗材"`}, {"BAL-B", "GOODS", "支", `,"category":"耗材"`},
		{"BAL-C", "GOODS", "台", `,"category":"设备"`}, {"BAL-D", "GOODS", "个", `,"category":"设备"`},
		{"BAL-SERVICE", "SERVICE", "次", ""}, {"BAL-NAME", "GOODS", "台", ""},
	} {
		units[spec.code] = spec.unit
		products[spec.code] = inventoryData[struct {
			ID int64 `json:"id"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":%q,"name":%q,"type":%q,"unit":%q,"model":"型号丙","specification":"规格丙"%s}`, spec.code, spec.code+"名称", spec.kind, spec.unit, spec.extra), token), 200).ID
	}
	// BAL-NAME keeps the same model/specification as BAL-A on purpose: the
	// keyword filter must not collapse rows that only share those fields.
	item := func(code, quantity, reason string) adjustmentItemInput {
		return adjustmentItemInput{ProductID: products[code], ProductType: "GOODS", Unit: units[code], Quantity: quantity, Reason: reason}
	}
	create := func(items ...adjustmentItemInput) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(items...), token), 200)
	}
	post := func(id int64) adjustmentDTO {
		return inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath+"/"+itoa(id)+"/post", `{"version":1}`, token), 200)
	}

	assertUnauthenticated(t, serveJSON(router, http.MethodGet, balancePath, "", ""))
	if balances() != 0 {
		t.Fatal("the current-stock view requires no pre-created balance rows")
	}
	for _, query := range []string{"?page=0", "?pageSize=0", "?page=abc", "?pageSize=-1", "?status=2", "?status=-1", "?status=x", "?stock=some", fmt.Sprintf("?keyword=%s", strings.Repeat("长", 101))} {
		inventoryData[any](t, serveJSON(router, http.MethodGet, balancePath+query, "", token), 400)
	}

	// Before any posting every physical product is zero stock, and drafts alone
	// never create a balance row.
	draft := create(item("BAL-A", "6", "OPENING"))
	draftsOnly := page("?stock=all")
	if draftsOnly.Total != 5 || len(draftsOnly.Records) != 5 || balances() != 0 {
		t.Fatalf("all view after drafting=%+v balances=%d", draftsOnly, balances())
	}
	for _, record := range draftsOnly.Records {
		if record.Quantity != "0" || record.Code == "BAL-SERVICE" {
			t.Fatalf("unsaved stock or hidden service leaked into the view: %+v", record)
		}
	}
	if empty := page(""); empty.Total != 0 || len(empty.Records) != 0 {
		t.Fatalf("default view before posting=%+v", empty)
	}
	if zero := page("?stock=zero"); zero.Total != 5 {
		t.Fatalf("zero view before posting=%+v", zero)
	}

	// Posting makes the real quantity visible, with thousandths preserved.
	post(draft.ID)
	post(create(item("BAL-B", "1.001", "OPENING")).ID)
	post(create(item("BAL-C", "0.001", "SURPLUS")).ID)
	if balances() != 3 {
		t.Fatalf("balances after posting=%d", balances())
	}
	defaultView := page("")
	if defaultView.Total != 3 || len(defaultView.Records) != 3 {
		t.Fatalf("default nonzero view=%+v", defaultView)
	}
	if !containsCode(defaultView.Records, "BAL-A") || !containsCode(defaultView.Records, "BAL-B") || !containsCode(defaultView.Records, "BAL-C") {
		t.Fatalf("default view rows=%v", codes(defaultView))
	}
	for _, record := range defaultView.Records {
		switch record.Code {
		case "BAL-A":
			if record.Quantity != "6" || record.Unit != "台" || record.Model == nil || *record.Model != "型号丙" || record.Category == nil || *record.Category != "耗材" {
				t.Fatalf("balance row=%+v", record)
			}
		case "BAL-B":
			if record.Quantity != "1.001" {
				t.Fatalf("thousandth balance=%+v", record)
			}
		}
	}
	// A product reduced to zero leaves the default view but is still listed, and
	// it keeps its stored zero balance row instead of losing it.
	post(create(item("BAL-C", "-0.001", "SHORTAGE")).ID)
	if after := page(""); containsCode(after.Records, "BAL-C") {
		t.Fatalf("zeroed product still in the nonzero view=%v", codes(after))
	}
	if value := balanceRows(t, db, products["BAL-C"]); len(value) != 1 || value[0] != 0 {
		t.Fatalf("zeroed product balance rows=%v", value)
	}
	// Five physical products exist, two hold stock, so exactly three are zero:
	// one with a stored zero balance and two that never had a balance row.
	if zero := page("?stock=zero"); zero.Total != 3 || len(zero.Records) != 3 || !containsCode(zero.Records, "BAL-C") || !containsCode(zero.Records, "BAL-D") || !containsCode(zero.Records, "BAL-NAME") {
		t.Fatalf("zero view=%+v", zero)
	}
	// Two different products both at zero are equally present, whether the zero
	// came from posting or from never having a balance row.
	for _, record := range zeroZero(t, page("?stock=zero")) {
		if record.Quantity != "0" {
			t.Fatalf("zero view row without zero quantity=%+v", record)
		}
	}

	// A disabled product stays visible by default, and status filters apply.
	// Posting and renaming happen before this, so the stored balance is unchanged
	// by either catalog operation.
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["BAL-B"])+"/status", `{"status":0}`, token), 200)
	if visible := page(""); !containsCode(visible.Records, "BAL-B") {
		t.Fatalf("disabled product missing from the default view=%v", codes(visible))
	}
	if enabled, disabled := page("?status=1"), page("?status=0"); containsCode(enabled.Records, "BAL-B") || !containsCode(disabled.Records, "BAL-B") {
		t.Fatalf("status filter enabled=%v disabled=%v", codes(enabled), codes(disabled))
	}
	// The list shows the live catalog text: renaming a product changes this view
	// while the stored quantity and identity stay untouched.
	inventoryData[any](t, serveJSON(router, http.MethodPut, "/api/v1/products/"+itoa(products["BAL-A"]), `{"name":"改名后的A","type":"GOODS","unit":"台","model":"型号丙","specification":"规格丙","category":"耗材"}`, token), 200)
	if renamed := page("?keyword=改名后的A"); renamed.Total != 1 || renamed.Records[0].Code != "BAL-A" || renamed.Records[0].Name != "改名后的A" || renamed.Records[0].Quantity != "6" {
		t.Fatalf("current stock must show the live description=%+v", renamed)
	}
	if value := balanceRows(t, db, products["BAL-A"]); len(value) != 1 || value[0] != 6000 {
		t.Fatalf("renaming changed the stored balance=%v", value)
	}

	// The category filter uses the same predicate as the total.
	consumables := page("?stock=all&category=耗材")
	if consumables.Total != 2 || len(consumables.Records) != 2 {
		t.Fatalf("category filter=%+v", consumables)
	}
	// Keyword matching spans code, name, model and specification, and rows that
	// only share a model stay distinct.
	if named := page("?stock=all&keyword=BAL-D名称"); named.Total != 1 || named.Records[0].Code != "BAL-D" {
		t.Fatalf("keyword by name=%+v", named)
	}
	if spec := page("?stock=all&keyword=规格丙&pageSize=500"); spec.Total != 5 || len(spec.Records) != 5 {
		t.Fatalf("keyword by specification=%+v", spec)
	}
	if escaped := page("?stock=all&keyword=%25"); escaped.Total != 0 || len(escaped.Records) != 0 {
		t.Fatalf("LIKE wildcards must be escaped: %+v", escaped)
	}

	// Paging keeps totals and rows consistent and never repeats a row.
	seen := map[string]bool{}
	for p := 1; p <= 3; p++ {
		result := page(fmt.Sprintf("?stock=all&page=%d&pageSize=2", p))
		if result.Total != 5 || result.Page != p || result.PageSize != 2 || len(result.Records) > 2 {
			t.Fatalf("page %d=%+v", p, result)
		}
		for _, record := range result.Records {
			if seen[record.Code] {
				t.Fatalf("product %s appeared on two pages", record.Code)
			}
			seen[record.Code] = true
		}
	}
	if len(seen) != 5 || page("?stock=all&page=4&pageSize=2").Total != 5 || len(page("?stock=all&page=4&pageSize=2").Records) != 0 {
		t.Fatalf("paging over all rows covered %d products", len(seen))
	}
	// Services never appear, in any view or filter.
	for _, query := range []string{"", "?stock=all", "?stock=zero", "?keyword=BAL-SERVICE", "?keyword=次"} {
		if result := page(query); containsCode(result.Records, "BAL-SERVICE") {
			t.Fatalf("service product listed for %q", query)
		}
	}
	// Reading current stock changes nothing: no write endpoint is exposed and the
	// stored balances are the ones posting produced.
	if balances() != 3 {
		t.Fatalf("viewing stock changed stored balances=%d", balances())
	}
	for _, method := range []string{http.MethodPut, http.MethodPost, http.MethodDelete, http.MethodPatch} {
		inventoryData[any](t, serveJSON(router, method, balancePath, `{"quantity":"999"}`, token), 404)
	}
	if balances() != 3 {
		t.Fatalf("a direct stock write succeeded: balances=%d", balances())
	}
}

func zeroZero(t *testing.T, result balancePageDTO) []balanceDTO {
	t.Helper()
	if len(result.Records) == 0 {
		t.Fatal("zero view returned no rows")
	}
	return result.Records
}

func containsCode(records []balanceDTO, code string) bool {
	for _, record := range records {
		if record.Code == code {
			return true
		}
	}
	return false
}
