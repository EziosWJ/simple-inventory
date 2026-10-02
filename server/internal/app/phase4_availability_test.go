package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	platformerrors "github.com/EziosWJ/simple-inventory/server/internal/platform/errors"
	"github.com/EziosWJ/simple-inventory/server/internal/printprofile"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/purchasereturn"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
	"github.com/EziosWJ/simple-inventory/server/internal/salereturn"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type purchaseErrorStore struct{ err error }

func (s purchaseErrorStore) SaveResult(_ context.Context, _ int64, _, _ string) (purchase.SaveResult, error) {
	return purchase.SaveResult{}, s.err
}

func (s purchaseErrorStore) Create(_ context.Context, _ purchase.Draft, _ []purchase.Line, _ audit.Event) (purchase.Draft, error) {
	return purchase.Draft{}, s.err
}
func (s purchaseErrorStore) Edit(_ context.Context, _ int64, _ int64, _ purchase.Draft, _ []purchase.Line, _ audit.Event) (purchase.Draft, error) {
	return purchase.Draft{}, s.err
}
func (s purchaseErrorStore) Cancel(_ context.Context, _ int64, _ int64, _ string, _ audit.Event) (purchase.Draft, error) {
	return purchase.Draft{}, s.err
}
func (s purchaseErrorStore) Post(_ context.Context, _ int64, _ int64, _ audit.Event) (purchase.Draft, error) {
	return purchase.Draft{}, s.err
}
func (s purchaseErrorStore) Find(_ context.Context, _ int64) (*purchase.Draft, error) {
	return nil, s.err
}
func (s purchaseErrorStore) Page(_ context.Context, _ purchase.Query) (purchase.Page, error) {
	return purchase.Page{}, s.err
}

type saleErrorStore struct{ err error }

func (s saleErrorStore) SaveResult(_ context.Context, _ int64, _, _ string) (sale.SaveResult, error) {
	return sale.SaveResult{}, s.err
}
func (s saleErrorStore) ResolveSave(_ context.Context, _ audit.Metadata, _, _ string) (sale.SaveResult, error) {
	return sale.SaveResult{}, s.err
}

func (s purchaseErrorStore) ResolveSave(_ context.Context, _ audit.Metadata, _, _ string) (purchase.SaveResult, error) {
	return purchase.SaveResult{}, s.err
}

func (s saleErrorStore) Create(_ context.Context, _ sale.Draft, _ []sale.Line, _ audit.Event) (sale.Draft, error) {
	return sale.Draft{}, s.err
}
func (s saleErrorStore) Edit(_ context.Context, _ int64, _ int64, _ sale.Draft, _ []sale.Line, _ audit.Event) (sale.Draft, error) {
	return sale.Draft{}, s.err
}
func (s saleErrorStore) Cancel(_ context.Context, _ int64, _ int64, _ string, _ audit.Event) (sale.Draft, error) {
	return sale.Draft{}, s.err
}
func (s saleErrorStore) Post(_ context.Context, _ int64, _ int64, _ audit.Event) (sale.Draft, error) {
	return sale.Draft{}, s.err
}
func (s saleErrorStore) Find(_ context.Context, _ int64) (*sale.Draft, error) { return nil, s.err }
func (s saleErrorStore) Page(_ context.Context, _ sale.Query) (sale.Page, error) {
	return sale.Page{}, s.err
}
func (s saleErrorStore) DeliveryNote(_ context.Context, _ int64) (*sale.DeliveryNote, error) {
	return nil, s.err
}

type purchasereturnErrorStore struct{ err error }

func (s purchasereturnErrorStore) Source(_ context.Context, _ int64) (*purchasereturn.Document, error) {
	return nil, s.err
}
func (s purchasereturnErrorStore) Create(_ context.Context, _ purchasereturn.Document, _ []purchasereturn.Item, _ audit.Event) (purchasereturn.Document, error) {
	return purchasereturn.Document{}, s.err
}
func (s purchasereturnErrorStore) Edit(_ context.Context, _ int64, _ int64, _ purchasereturn.Document, _ []purchasereturn.Item, _ audit.Event) (purchasereturn.Document, error) {
	return purchasereturn.Document{}, s.err
}
func (s purchasereturnErrorStore) Cancel(_ context.Context, _ int64, _ int64, _ string, _ audit.Event) (purchasereturn.Document, error) {
	return purchasereturn.Document{}, s.err
}
func (s purchasereturnErrorStore) Post(_ context.Context, _ int64, _ int64, _ audit.Event) (purchasereturn.Document, error) {
	return purchasereturn.Document{}, s.err
}
func (s purchasereturnErrorStore) Find(_ context.Context, _ int64) (*purchasereturn.Document, error) {
	return nil, s.err
}
func (s purchasereturnErrorStore) Page(_ context.Context, _ purchasereturn.Query) (purchasereturn.Page, error) {
	return purchasereturn.Page{}, s.err
}

type salereturnErrorStore struct{ err error }

func (s salereturnErrorStore) Source(_ context.Context, _ int64) (*salereturn.Document, error) {
	return nil, s.err
}
func (s salereturnErrorStore) Create(_ context.Context, _ salereturn.Document, _ []salereturn.Item, _ audit.Event) (salereturn.Document, error) {
	return salereturn.Document{}, s.err
}
func (s salereturnErrorStore) Edit(_ context.Context, _ int64, _ int64, _ salereturn.Document, _ []salereturn.Item, _ audit.Event) (salereturn.Document, error) {
	return salereturn.Document{}, s.err
}
func (s salereturnErrorStore) Cancel(_ context.Context, _ int64, _ int64, _ string, _ audit.Event) (salereturn.Document, error) {
	return salereturn.Document{}, s.err
}
func (s salereturnErrorStore) Post(_ context.Context, _ int64, _ int64, _ audit.Event) (salereturn.Document, error) {
	return salereturn.Document{}, s.err
}
func (s salereturnErrorStore) Find(_ context.Context, _ int64) (*salereturn.Document, error) {
	return nil, s.err
}
func (s salereturnErrorStore) Page(_ context.Context, _ salereturn.Query) (salereturn.Page, error) {
	return salereturn.Page{}, s.err
}

type receivableErrorStore struct{ err error }

func (s receivableErrorStore) FilterPage(_ context.Context, _ receivable.EntryFilter) (receivable.Page, error) {
	return receivable.Page{}, s.err
}
func (s receivableErrorStore) Statement(_ context.Context, _ receivable.EntryFilter) (receivable.Statement, error) {
	return receivable.Statement{}, s.err
}
func (s receivableErrorStore) Create(_ context.Context, _ audit.Metadata, _ receivable.Input) (receivable.Entry, error) {
	return receivable.Entry{}, s.err
}
func (s receivableErrorStore) Find(_ context.Context, _ int64) (receivable.Entry, error) {
	return receivable.Entry{}, s.err
}
func (s receivableErrorStore) Page(_ context.Context, _ int64, _ string, _ int, _ int) (receivable.Page, error) {
	return receivable.Page{}, s.err
}
func (s receivableErrorStore) Balances(_ context.Context, _ int64, _ int, _ int, _ string) (receivable.BalancePage, error) {
	return receivable.BalancePage{}, s.err
}
func (s receivableErrorStore) Reverse(_ context.Context, _ audit.Metadata, _ int64, _ string) (receivable.Entry, error) {
	return receivable.Entry{}, s.err
}
func (s receivableErrorStore) Settle(_ context.Context, _ audit.Metadata, _ receivable.SettlementInput) (receivable.Entry, error) {
	return receivable.Entry{}, s.err
}
func (s receivableErrorStore) Refund(_ context.Context, _ audit.Metadata, _ receivable.SettlementInput) (receivable.Entry, error) {
	return receivable.Entry{}, s.err
}

type printprofileErrorStore struct{ err error }

func (s printprofileErrorStore) Get(_ context.Context) (printprofile.Profile, error) {
	return printprofile.Profile{}, s.err
}
func (s printprofileErrorStore) Update(_ context.Context, _ printprofile.Profile, _ audit.Event) (printprofile.Profile, error) {
	return printprofile.Profile{}, s.err
}

func TestPhase4HandlersTemporaryUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	unavailable := fmt.Errorf("wrapped storage error: %w", platformerrors.ErrTemporarilyUnavailable)
	draft := `{"partnerId":1,"businessDate":"2026-10-01","items":[{"productId":1,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.00"}]}`
	funds := `{"partnerId":1,"direction":"CUSTOMER","amount":"1.00","requestKey":"test","businessDate":"2026-10-01","paymentMethod":"CASH","description":"期初"}`
	type request struct{ method, path, body string }
	cases := []struct {
		name                       string
		register                   func(*gin.Engine, error)
		requests                   []request
		invalid, missing, conflict error
	}{
		{name: "purchase", register: func(r *gin.Engine, e error) {
			purchase.RegisterRoutes(r, purchase.NewHandler(purchase.NewService(purchaseErrorStore{e})))
		}, requests: []request{{"POST", "/purchases", draft}, {"PUT", "/purchases/1", `{"version":1,` + strings.TrimPrefix(draft, "{")}, {"POST", "/purchases/1/post", `{"version":1}`}, {"POST", "/purchases/1/cancel", `{"version":1,"reason":"测试"}`}, {"GET", "/purchases", ""}, {"GET", "/purchases/1", ""}}, invalid: purchase.ErrInvalid, missing: purchase.ErrNotFound, conflict: purchase.ErrConflict},
		{name: "sale", register: func(r *gin.Engine, e error) {
			sale.RegisterRoutes(r, sale.NewHandler(sale.NewService(saleErrorStore{e})))
		}, requests: []request{{"POST", "/sales", draft}, {"PUT", "/sales/1", `{"version":1,` + strings.TrimPrefix(draft, "{")}, {"POST", "/sales/1/post", `{"version":1}`}, {"POST", "/sales/1/cancel", `{"version":1,"reason":"测试"}`}, {"GET", "/sales", ""}, {"GET", "/sales/1", ""}, {"GET", "/sales/1/delivery-note", ""}}, invalid: sale.ErrInvalid, missing: sale.ErrNotFound, conflict: sale.ErrConflict},
		{name: "purchasereturn", register: func(r *gin.Engine, e error) {
			purchasereturn.RegisterRoutes(r, purchasereturn.NewHandler(purchasereturn.NewService(purchasereturnErrorStore{e})))
		}, requests: []request{{"POST", "/purchase-returns", `{"purchaseId":1,"businessDate":"2026-10-01","items":[{"purchaseItemId":1,"quantity":"1"}]}`}, {"PUT", "/purchase-returns/1", `{"version":1,` + strings.TrimPrefix(`{"purchaseId":1,"businessDate":"2026-10-01","items":[{"purchaseItemId":1,"quantity":"1"}]}`, "{")}, {"POST", "/purchase-returns/1/post", `{"version":1}`}, {"POST", "/purchase-returns/1/cancel", `{"version":1,"reason":"测试"}`}, {"GET", "/purchase-returns", ""}, {"GET", "/purchase-returns/1", ""}, {"GET", "/purchase-returns/source/1", ""}}, invalid: purchasereturn.ErrInvalid, missing: purchasereturn.ErrNotFound, conflict: purchasereturn.ErrConflict},
		{name: "salereturn", register: func(r *gin.Engine, e error) {
			salereturn.RegisterRoutes(r, salereturn.NewHandler(salereturn.NewService(salereturnErrorStore{e})))
		}, requests: []request{{"POST", "/sale-returns", `{"saleId":1,"businessDate":"2026-10-01","items":[{"saleItemId":1,"quantity":"1"}]}`}, {"PUT", "/sale-returns/1", `{"version":1,` + strings.TrimPrefix(`{"saleId":1,"businessDate":"2026-10-01","items":[{"saleItemId":1,"quantity":"1"}]}`, "{")}, {"POST", "/sale-returns/1/post", `{"version":1}`}, {"POST", "/sale-returns/1/cancel", `{"version":1,"reason":"测试"}`}, {"GET", "/sale-returns", ""}, {"GET", "/sale-returns/1", ""}, {"GET", "/sale-returns/source/1", ""}}, invalid: salereturn.ErrInvalid, missing: salereturn.ErrNotFound, conflict: salereturn.ErrConflict},
		{name: "receivable", register: func(r *gin.Engine, e error) {
			receivable.RegisterRoutes(r, receivable.NewHandler(receivable.NewService(receivableErrorStore{e})))
		}, requests: []request{
			{"POST", "/partner-balances/opening", funds}, {"POST", "/partner-balances/settlements", funds}, {"POST", "/partner-balances/refunds", funds}, {"POST", "/partner-balances/entries/1/reverse", `{"reason":"测试"}`}, {"GET", "/partner-balances", ""}, {"GET", "/partner-balances/entries", ""}, {"GET", "/partner-balances/entries/1", ""}, {"GET", "/partner-balances/statement?partnerId=1&direction=CUSTOMER&from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z", ""}}, invalid: receivable.ErrInvalid, missing: receivable.ErrNotFound, conflict: receivable.ErrConflict},
		{name: "printprofile", register: func(r *gin.Engine, e error) {
			printprofile.RegisterRoutes(r, printprofile.NewHandler(printprofile.NewService(printprofileErrorStore{e})))
		}, requests: []request{{"GET", "/print-profile", ""}, {"PUT", "/print-profile", `{"name":"经营者"}`}}, invalid: printprofile.ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := func(req request, err error, status int) {
				t.Helper()
				router := gin.New()
				tc.register(router, err)
				w := httptest.NewRecorder()
				r := httptest.NewRequest(req.method, req.path, strings.NewReader(req.body))
				r.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, r)
				var v struct {
					Code    int
					Message string
					Data    any
				}
				if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
					t.Fatal(e)
				}
				if w.Code != status || v.Code != status || v.Data != nil {
					t.Fatalf("%s %s: %d %s want %d", req.method, req.path, w.Code, w.Body.String(), status)
				}
				if status == 503 && v.Message != "service temporarily unavailable" {
					t.Fatalf("availability envelope=%s", w.Body.String())
				}
			}
			for _, req := range tc.requests {
				check(req, unavailable, 503)
			}
			check(tc.requests[0], tc.invalid, 400)
			if tc.missing != nil {
				for _, req := range tc.requests {
					if req.method == "GET" && strings.HasSuffix(req.path, "/1") {
						check(req, tc.missing, 404)
						break
					}
				}
			}
			if tc.conflict != nil {
				check(tc.requests[0], tc.conflict, 409)
			}
		})
	}
}
