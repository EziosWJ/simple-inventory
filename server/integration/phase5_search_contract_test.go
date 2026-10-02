//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/partner"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/product"
)

func TestSQLitePhase5SearchHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "search.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	r, err := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	assertPhase5Search(t, r, db)
}

func TestPostgresPhase5SearchHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	r, err := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	assertPhase5Search(t, r, db)
}

func assertPhase5Search(t *testing.T, r http.Handler, db *platformdatabase.Database) {
	token := loginAdmin(t, r)
	ptr := func(s string) *string { return &s }
	// The oldest choice lies beyond the previous UI's first 500 records.
	products := make([]product.Product, 505)
	partners := make([]partner.Partner, 505)
	for i := range products {
		products[i] = product.Product{Code: fmt.Sprintf("SEARCH-P-%04d", i), Name: "同名商品", Type: "GOODS", Unit: "台", Status: 1}
		partners[i] = partner.Partner{Code: fmt.Sprintf("SEARCH-S-%04d", i), Name: "同名供应商", Type: "COMPANY", IsSupplier: true, Status: 1}
	}
	products[0].Brand, products[0].Model, products[0].Specification = ptr("BrandX"), ptr("AbC%_!\\型号"), ptr("特有规格")
	partners[0].Contact, partners[0].Phone = ptr("联络AbC%_!\\"), ptr("13800123456")
	products[1].Model = ptr("AbC-anything")
	partners[1].Contact = ptr("联络AbC-anything")
	products[2].Model, products[2].Status = products[0].Model, 0
	products[3].Model, products[3].Type = products[0].Model, "SERVICE"
	partners[2].Contact, partners[2].Status = partners[0].Contact, 0
	partners[3].Contact, partners[3].IsSupplier, partners[3].IsCustomer = partners[0].Contact, false, true
	if e := db.GORM.CreateInBatches(&products, 100).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.CreateInBatches(&partners, 100).Error; e != nil {
		t.Fatal(e)
	}
	type record struct {
		ID     int64
		Code   string
		Status int
	}
	type page struct {
		Records        []record
		Total          int
		Page, PageSize int
	}
	query := func(kind, filters, keyword string, number, size int) page {
		t.Helper()
		path := fmt.Sprintf("/api/v1/%s?%s&keyword=%s&page=%d&pageSize=%d", kind, filters, url.QueryEscape(keyword), number, size)
		res := serveJSON(r, "GET", path, "", token)
		if res.Code != 200 {
			t.Fatalf("%s: %d %s", path, res.Code, res.Body.String())
		}
		var v struct{ Data page }
		if err := json.Unmarshal(res.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v.Data
	}
	for _, tc := range []struct {
		kind, filters string
		id            int64
		words         []string
	}{
		{"products", "status=1&type=GOODS", products[0].ID, []string{"search-p-0000", "brandx", "  aBc%_!\\型号  ", "特有规格", "%_"}},
		{"partners", "status=1&identity=SUPPLIER", partners[0].ID, []string{"search-s-0000", " 联络aBc%_!\\ ", "001234", "%_"}},
	} {
		for _, word := range tc.words {
			p := query(tc.kind, tc.filters, word, 1, 10)
			if p.Total != 1 || len(p.Records) != 1 || p.Records[0].ID != tc.id {
				t.Fatalf("%s %q: %+v", tc.kind, word, p)
			}
		}
		first := query(tc.kind, tc.filters, "同名", 1, 10)
		last := query(tc.kind, tc.filters, "同名", 51, 10)
		if first.Total != 503 || first.Page != 1 || first.PageSize != 10 || len(first.Records) != 10 || len(last.Records) != 3 || last.Records[2].ID != tc.id {
			t.Fatalf("%s pagination: first=%+v last=%+v", tc.kind, first, last)
		}
		for i := 1; i < len(first.Records); i++ {
			if first.Records[i-1].ID <= first.Records[i].ID {
				t.Fatal("unstable descending ID order")
			}
		}
		if p := query(tc.kind, tc.filters, "no-such-record", 1, 10); p.Total != 0 || len(p.Records) != 0 {
			t.Fatalf("empty search: %+v", p)
		}
		if p := query(tc.kind, tc.filters, "   ", 1, 10); p.Total != 503 {
			t.Fatalf("blank search: %+v", p)
		}
		if p := query(tc.kind, "", "%_", 1, 10); p.Total != 3 {
			t.Fatalf("historical search: %+v", p)
		}
	}
	for _, tc := range []struct {
		kind string
		id   int64
	}{{"products", products[2].ID}, {"partners", partners[2].ID}} {
		res := serveJSON(r, "GET", fmt.Sprintf("/api/v1/%s/%d", tc.kind, tc.id), "", token)
		var v struct{ Data record }
		_ = json.Unmarshal(res.Body.Bytes(), &v)
		if res.Code != 200 || v.Data.ID != tc.id || v.Data.Status != 0 {
			t.Fatalf("inactive identity unavailable: %s", res.Body.String())
		}
	}
}
