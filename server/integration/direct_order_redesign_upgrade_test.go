//go:build integration

package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/directorder"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/pressly/goose/v3"
)

func TestSQLiteDirectOrderRedesignUpgrade(t *testing.T) {
	assertDirectOrderRedesignUpgrade(t, "sqlite")
}
func TestPostgresDirectOrderRedesignUpgrade(t *testing.T) {
	assertDirectOrderRedesignUpgrade(t, "postgres")
}
func assertDirectOrderRedesignUpgrade(t *testing.T, dialect string) {
	var db *platformdatabase.Database
	root := filepath.Join(projectRoot(t), "migrations")
	gooseDialect := "postgres"
	if dialect == "sqlite" {
		db = openSQLiteDatabase(t, filepath.Join(t.TempDir(), "upgrade-27.db"))
		root = filepath.Join(root, "sqlite")
		gooseDialect = "sqlite3"
	} else {
		pg := createTestDatabase(t)
		db = openTemporaryDatabase(t, pg.dsn)
	}
	defer db.Close()
	if e := goose.SetDialect(gooseDialect); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "schema"), 27); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_seed_db_version")
	if e := goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "seed")); e != nil {
		t.Fatal(e)
	}
	r := directDraftRouter(t, db, dialect)
	token := loginAdmin(t, r)
	partner := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"UP27-P","name":"旧交易方","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	goods := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"UP27-G","name":"旧配件","type":"GOODS","unit":"条"}`, token), 200).ID
	pi := inventoryData[purchase.Draft](t, serveJSON(r, "POST", "/api/v1/purchases", fmt.Sprintf(`{"directDelivery":true,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"条","quantity":"2","unitPrice":"10.00"}]}`, partner, goods), token), 200)
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchases/%d/post", pi.ID), `{"version":1}`, token), 200)
	inventoryData[any](t, serveJSON(r, "POST", "/api/v1/sales", fmt.Sprintf(`{"directDelivery":true,"directPurchaseId":%d,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"条","quantity":"2","unitPrice":"15.00"}]}`, pi.ID, partner, goods), token), 200)
	if e := db.GORM.Exec("INSERT INTO direct_order_document(document_no,supplier_id,customer_id,business_date,status,version,created_by) VALUES ('DD-OLD27',?,?,'2026-10-01','DRAFT',1,1)", partner, partner).Error; e != nil {
		t.Fatal(e)
	}
	var directID int64
	db.GORM.Table("direct_order_document").Where("document_no='DD-OLD27'").Pluck("id", &directID)
	if e := db.GORM.Exec("INSERT INTO direct_order_item(document_id,product_id,product_type,unit,quantity_milli,purchase_price_cents,sale_price_cents,purchase_amount_cents,sale_amount_cents) VALUES (?,?,'GOODS','条',2000,1000,1500,2000,3000)", directID, goods).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec("INSERT INTO direct_order_save_request(actor_id,operation,request_key,fingerprint,document_id,saved_version) VALUES (1,'CREATE','old-27-receipt','legacy27',?,1)", directID).Error; e != nil {
		t.Fatal(e)
	}
	snapshot := func() map[string][]map[string]any {
		out := map[string][]map[string]any{}
		for _, table := range []string{"purchase_document", "purchase_document_item", "sale_document", "sale_document_item", "direct_order_document", "direct_order_item", "direct_order_save_request", "inventory_balance", "inventory_entry", "partner_balance", "partner_balance_entry"} {
			var rows []map[string]any
			order := "id"
			if table == "direct_order_save_request" {
				order = "request_key"
			}
			if e := db.GORM.Table(table).Order(order).Find(&rows).Error; e != nil {
				t.Fatal(e)
			}
			for _, row := range rows {
				if table == "purchase_document" || table == "sale_document" {
					delete(row, "source_order_id")
				}
				if table == "direct_order_document" {
					for _, column := range []string{"purchase_id", "sale_id", "posted_by", "posted_at", "cancelled_by", "cancelled_at", "cancel_reason", "supplier_snapshot", "customer_snapshot"} {
						delete(row, column)
					}
				}
				if table == "direct_order_item" {
					for _, column := range []string{"product_code", "product_name", "product_model", "product_specification"} {
						delete(row, column)
					}
				}
			}
			out[table] = rows
		}
		return out
	}
	before := snapshot()
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "schema")); e != nil {
		t.Fatal(e)
	}
	// Migration changes SELECT * result shapes; restart idle connections as the API does after upgrade.
	db.SQL.SetMaxIdleConns(0)
	db.SQL.SetMaxIdleConns(5)
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("27->29 upgrade changed old direct drafts, receipts or ledgers")
	}
	d := inventoryData[directorder.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/direct-orders/%d", directID), "", token), 200)
	if d.Status != "DRAFT" || d.Version != 1 || d.PurchaseID != nil || d.SaleID != nil {
		t.Fatal("old draft state changed")
	}
	receipt := inventoryData[directorder.SaveResult](t, serveJSON(r, "GET", "/api/v1/direct-orders/save-requests/CREATE/old-27-receipt", "", token), 200)
	if receipt.State != "COMMITTED" || receipt.Receipt.SavedVersion != 1 {
		t.Fatal("old receipt lost")
	}
	posted := inventoryData[directorder.Draft](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/direct-orders/%d/post", directID), `{"version":1}`, token), 200)
	if posted.PurchaseID == nil || posted.SaleID == nil {
		t.Fatal("old draft cannot confirm")
	}
	var qty int64
	db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", goods).Scan(&qty)
	if qty != 2000 {
		t.Fatal("upgraded draft changed net inventory")
	}
}
