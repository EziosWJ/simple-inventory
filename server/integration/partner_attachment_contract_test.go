//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"github.com/EziosWJ/simple-inventory/server/internal/filemgmt"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/pressly/goose/v3"
)

func TestSQLitePartnerAttachmentContract(t *testing.T) {
	withRefundStatementDB(t, "sqlite", assertPartnerAttachmentContract)
}
func TestPostgresPartnerAttachmentContract(t *testing.T) {
	withRefundStatementDB(t, "postgres", assertPartnerAttachmentContract)
}

func attachmentPNG(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.White)
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
func attachmentPDF() string {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"} {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	b.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return b.String()
}
func assertPartnerAttachmentContract(t *testing.T, r http.Handler, db *platformdatabase.Database) {
	f := newFundsFixture(t, r)
	upload := func(kind, name, content string) filemgmt.File {
		t.Helper()
		return inventoryData[filemgmt.File](t, serveMultipart(t, r, "POST", "/api/system/file/upload", f.token, []filePart{{field: "file", filename: name, contentType: "application/octet-stream", content: content}}, filemgmt.AttachmentPrefix+kind, ""), 200)
	}
	get := func(id int64) []receivable.Attachment {
		t.Helper()
		return inventoryData[[]receivable.Attachment](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", id), "", f.token), 200)
	}
	add := func(id int64, files []int64, status int) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"fileIds": files})
		inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", id), string(body), f.token), status)
	}
	pngContent := attachmentPNG(t)
	// Content checks reject forged types, truncated PDFs and files over the business limit.
	for _, part := range []filePart{{field: "file", filename: "fake.png", content: "hello"}, {field: "file", filename: "fake.pdf", content: "%PDF-1.4\nstartxref\n0\n%%EOF"}, {field: "file", filename: "receipt.doc", content: "hello"}, {field: "file", filename: "large.pdf", content: strings.Repeat("x", filemgmt.MaxAttachmentSize+1)}} {
		res := serveMultipart(t, r, "POST", "/api/system/file/upload", f.token, []filePart{part}, filemgmt.AttachmentPrefix+"OPENING", "")
		var envelope struct{ Code int }
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Code != 400 {
			t.Fatalf("invalid attachment accepted: %s", res.Body.String())
		}
	}
	boundary := upload("OPENING", "boundary.png", pngContent+strings.Repeat("\x00", filemgmt.MaxAttachmentSize-len(pngContent)))
	if boundary.FileSize != filemgmt.MaxAttachmentSize {
		t.Fatal("exact size boundary rejected")
	}
	pdf := upload("OPENING", "opening.pdf", attachmentPDF())
	imageFile := upload("OPENING", "opening.png", pngContent)
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	jpgFile := upload("OPENING", "opening.jpg", jpg.String())
	if pdf.MimeType != "application/pdf" || imageFile.MimeType != "image/png" || jpgFile.MimeType != "image/jpeg" {
		t.Fatal("content type not canonical")
	}
	body := fmt.Sprintf(`{"requestKey":"attachment-opening","partnerId":%d,"direction":"CUSTOMER","amount":"100.00","businessDate":"2026-10-01","description":"历史欠款","attachmentIds":[%d,%d]}`, f.partner, pdf.ID, imageFile.ID)
	opening := inventoryData[receivable.Entry](t, serveJSON(r, "POST", openingPath, body, f.token), 200)
	retry := inventoryData[receivable.Entry](t, serveJSON(r, "POST", openingPath, body, f.token), 200)
	if opening.ID != retry.ID || len(get(opening.ID)) != 2 {
		t.Fatal("binding is not idempotent")
	}
	changed := strings.Replace(body, fmt.Sprintf("[%d,%d]", pdf.ID, imageFile.ID), fmt.Sprintf("[%d]", pdf.ID), 1)
	inventoryData[any](t, serveJSON(r, "POST", openingPath, changed, f.token), 409)
	add(opening.ID, []int64{jpgFile.ID}, 200)
	for i := 0; i < 2; i++ {
		file := upload("OPENING", fmt.Sprintf("extra-%d.png", i), pngContent)
		add(opening.ID, []int64{file.ID}, 200)
	}
	sixth := upload("OPENING", "sixth.png", pngContent)
	add(opening.ID, []int64{sixth.ID}, 400)
	attachments := get(opening.ID)
	inventoryData[any](t, serveJSON(r, "DELETE", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments/%d", opening.ID, attachments[0].ID), "", f.token), 200)
	add(opening.ID, []int64{sixth.ID}, 200)
	history := get(opening.ID)
	if len(history) != 6 || history[0].RemovedAt == nil || history[0].RemovedBy == nil || history[0].UploadedAt.IsZero() {
		t.Fatalf("missing attachment history: %+v", history)
	}
	// Removed files remain readable and protected against all generic mutation routes.
	for _, a := range history {
		for _, suffix := range []string{"view", "download"} {
			res := servePlain(t, r, "GET", fmt.Sprintf("/api/system/file/%d/%s", a.FileID, suffix), f.token)
			if res.Code != 200 || res.Body.Len() == 0 {
				t.Fatal(res.Body.String())
			}
		}
		inventoryData[any](t, serveJSON(r, "DELETE", fmt.Sprintf("/api/system/file/%d", a.FileID), "", f.token), 409)
		inventoryData[any](t, serveJSON(r, "PUT", fmt.Sprintf("/api/system/file/%d", a.FileID), `{"businessModule":"ordinary"}`, f.token), 409)
		inventoryData[any](t, serveJSON(r, "PATCH", fmt.Sprintf("/api/system/file/%d/status", a.FileID), `{"status":0}`, f.token), 409)
	}
	inventoryData[any](t, serveJSON(r, "POST", "/api/system/file/batch-delete", fmt.Sprintf(`{"ids":[%d]}`, pdf.ID), f.token), 409)
	page := inventoryData[receivable.Page](t, serveJSON(r, "GET", "/api/v1/partner-balances/entries?category=OPENING&page=1&pageSize=20", "", f.token), 200)
	if page.Records[0].AttachmentCount != 5 {
		t.Fatalf("active count=%d", page.Records[0].AttachmentCount)
	}
	f.balance("CUSTOMER", "100.00")
	// Invalid binding rolls back the whole new business operation.
	invalidBody := strings.Replace(body, "attachment-opening", "invalid-binding", 1)
	invalidBody = strings.Replace(invalidBody, fmt.Sprintf("[%d,%d]", pdf.ID, imageFile.ID), "[99999999]", 1)
	inventoryData[any](t, serveJSON(r, "POST", openingPath, invalidBody, f.token), 400)
	f.balance("CUSTOMER", "100.00")
	// Failure to write audit must roll back both ledger and attachment association.
	rollbackFile := upload("OPENING", "rollback.png", pngContent)
	rollbackBody := fmt.Sprintf(`{"requestKey":"audit-attachment-rollback","partnerId":%d,"direction":"CUSTOMER","amount":"1.00","businessDate":"2026-10-01","description":"审计失败","attachmentIds":[%d]}`, f.partner, rollbackFile.ID)
	if err := db.GORM.Migrator().RenameTable("sys_oper_log", "attachment_audit_hold"); err != nil {
		t.Fatal(err)
	}
	failed := serveJSON(r, "POST", openingPath, rollbackBody, f.token)
	if err := db.GORM.Migrator().RenameTable("attachment_audit_hold", "sys_oper_log"); err != nil {
		t.Fatal(err)
	}
	inventoryData[any](t, failed, 500)
	f.balance("CUSTOMER", "100.00")
	var rollbackCount int64
	if err := db.GORM.Table("partner_entry_attachment").Where("file_id=?", rollbackFile.ID).Count(&rollbackCount).Error; err != nil {
		t.Fatal(err)
	}
	if rollbackCount != 0 {
		t.Fatal("audit failure left a bound attachment")
	}
	if err := db.GORM.Table("partner_balance_entry").Where("request_key=?", "audit-attachment-rollback").Count(&rollbackCount).Error; err != nil {
		t.Fatal(err)
	}
	if rollbackCount != 0 {
		t.Fatal("audit failure left a business entry")
	}
	settlementFile := upload("SETTLEMENT", "receipt.pdf", attachmentPDF())
	settlementBody := f.fundsBody("CUSTOMER", "10.00", "attachment-settlement")
	settlementBody = settlementBody[:len(settlementBody)-1] + fmt.Sprintf(`,"attachmentIds":[%d]}`, settlementFile.ID)
	settled := inventoryData[receivable.Entry](t, serveJSON(r, "POST", "/api/v1/partner-balances/settlements", settlementBody, f.token), 200)
	if len(get(settled.ID)) != 1 {
		t.Fatal("settlement missing attachment")
	}
	f.balance("CUSTOMER", "90.00")
	// Existing balances can be refunded with a new attachment, both directions.
	purchase := f.trade("purchases", "2", "100.00")
	f.funds("settlements", "SUPPLIER", "200.00")
	f.returnGoods("purchase-returns", purchase, "2")
	refundFile := upload("REFUND", "refund.pdf", attachmentPDF())
	refundBody := f.fundsBody("SUPPLIER", "10.00", "attachment-refund")
	refundBody = refundBody[:len(refundBody)-1] + fmt.Sprintf(`,"attachmentIds":[%d]}`, refundFile.ID)
	refunded := inventoryData[receivable.Entry](t, serveJSON(r, "POST", "/api/v1/partner-balances/refunds", refundBody, f.token), 200)
	if len(get(refunded.ID)) != 1 {
		t.Fatal("refund missing attachment")
	}
	// Supplier opening and payment use the same attachment rules as the customer direction.
	supplierOpeningFile := upload("OPENING", "supplier-opening.pdf", attachmentPDF())
	supplierOpening := inventoryData[receivable.Entry](t, serveJSON(r, "POST", openingPath, fmt.Sprintf(`{"requestKey":"supplier-attachment-opening","partnerId":%d,"direction":"SUPPLIER","amount":"3.00","businessDate":"2026-10-01","description":"供应商期初","attachmentIds":[%d]}`, f.partner, supplierOpeningFile.ID), f.token), 200)
	if len(get(supplierOpening.ID)) != 1 {
		t.Fatal("supplier opening missing attachment")
	}
	// The supplier currently has a refundable balance. An additional purchase makes a positive debt.
	f.trade("purchases", "1", "300.00")
	supplierPaymentFile := upload("SETTLEMENT", "supplier-payment.pdf", attachmentPDF())
	paymentBody := f.fundsBody("SUPPLIER", "1.00", "supplier-attachment-payment")
	paymentBody = paymentBody[:len(paymentBody)-1] + fmt.Sprintf(`,"attachmentIds":[%d]}`, supplierPaymentFile.ID)
	paid := inventoryData[receivable.Entry](t, serveJSON(r, "POST", "/api/v1/partner-balances/settlements", paymentBody, f.token), 200)
	if len(get(paid.ID)) != 1 {
		t.Fatal("supplier payment missing attachment")
	}
	// Shared permission can manage another operator's bound file, but cannot steal temporary files.
	var peer auth.User
	if err := db.GORM.Where("username=?", "admin").Take(&peer).Error; err != nil {
		t.Fatal(err)
	}
	adminID := peer.ID
	peer.ID = 0
	peer.Username = "attachment-peer"
	peer.Nickname = "共同经营者"
	if err := db.GORM.Create(&peer).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role(user_id,role_id) SELECT ?,role_id FROM sys_user_role WHERE user_id=?", peer.ID, adminID).Error; err != nil {
		t.Fatal(err)
	}
	peerToken := loginUser(t, r, peer.Username, "admin123")
	peerList := inventoryData[[]receivable.Attachment](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", opening.ID), "", peerToken), 200)
	if len(peerList) != 6 {
		t.Fatal("shared history missing")
	}
	inventoryData[any](t, serveJSON(r, "DELETE", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments/%d", opening.ID, history[1].ID), "", peerToken), 200)
	temporary := upload("OPENING", "temporary.png", pngContent)
	inventoryData[any](t, serveJSON(r, "GET", fmt.Sprintf("/api/system/file/%d", temporary.ID), "", peerToken), 403)
	if err := db.GORM.Exec("DELETE FROM sys_user_role WHERE user_id=?", peer.ID).Error; err != nil {
		t.Fatal(err)
	}
	inventoryData[any](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", opening.ID), "", peerToken), 403)
	if res := servePlain(t, r, "GET", fmt.Sprintf("/api/system/file/%d/view", pdf.ID), peerToken); res.Code != 403 {
		t.Fatalf("direct file bypass: %s", res.Body.String())
	}
	inventoryData[any](t, serveMultipart(t, r, "POST", "/api/system/file/upload", peerToken, []filePart{{field: "file", filename: "no-permission.png", content: pngContent}}, filemgmt.AttachmentPrefix+"OPENING", ""), 403)
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", opening.ID), fmt.Sprintf(`{"fileIds":[%d]}`, temporary.ID), peerToken), 403)
	assertUnauthenticated(t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", opening.ID), "", ""))
	inventoryData[any](t, serveJSON(r, "POST", openingPath, body, peerToken), 403)
	// A ledger-only grant permits preview/history but cannot upload or mutate attachments.
	var readRoleID int64
	if err := db.GORM.Exec("INSERT INTO sys_role(role_name,role_code,status) VALUES ('附件只读','attachment_reader',1)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("sys_role").Select("id").Where("role_code=?", "attachment_reader").Scan(&readRoleID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role(user_id,role_id) VALUES (?,?)", peer.ID, readRoleID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_role_menu(role_id,menu_id) SELECT ?,id FROM sys_menu WHERE permission_code='business:partner-ledger' AND deleted=0", readRoleID).Error; err != nil {
		t.Fatal(err)
	}
	inventoryData[[]receivable.Attachment](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", opening.ID), "", peerToken), 200)
	if res := servePlain(t, r, "GET", fmt.Sprintf("/api/system/file/%d/view", pdf.ID), peerToken); res.Code != 200 {
		t.Fatal("ledger reader cannot preview")
	}
	inventoryData[any](t, serveJSON(r, "DELETE", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments/%d", opening.ID, history[2].ID), "", peerToken), 403)
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", opening.ID), fmt.Sprintf(`{"fileIds":[%d]}`, temporary.ID), peerToken), 403)
	inventoryData[any](t, serveMultipart(t, r, "POST", "/api/system/file/upload", peerToken, []filePart{{field: "file", filename: "read-only.png", contentType: "image/png", content: pngContent}}, filemgmt.AttachmentPrefix+"OPENING", ""), 403)

	// Cleanup preserves every historical binding and a renewed lease, but expires abandoned uploads.
	expired := upload("OPENING", "expired.png", pngContent)
	cutoff := time.Now().UTC().Add(-48 * time.Hour)
	if err := db.GORM.Model(&filemgmt.File{}).Where("id IN ?", []int64{expired.ID, temporary.ID, pdf.ID}).Update("update_time", cutoff).Error; err != nil {
		t.Fatal(err)
	}
	inventoryData[filemgmt.File](t, serveJSON(r, "GET", fmt.Sprintf("/api/system/file/%d", temporary.ID), "", f.token), 200)
	upload("OPENING", "cleanup-trigger.png", pngContent)
	var remaining int64
	if err := db.GORM.Table("sys_file").Where("id=?", expired.ID).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("abandoned upload not cleaned")
	}
	for _, id := range []int64{temporary.ID, pdf.ID} {
		if res := servePlain(t, r, "GET", fmt.Sprintf("/api/system/file/%d/view", id), f.token); res.Code != 200 {
			t.Fatal("cleanup removed leased/bound file")
		}
	}
	// Concurrent additions must serialize at the balance gate and cannot exceed five.
	concurrent := f.post(openingPath, fmt.Sprintf(`{"requestKey":"attachment-concurrency","partnerId":%d,"direction":"CUSTOMER","amount":"1.00","businessDate":"2026-10-01","description":"并发附件"}`, f.partner))
	concurrentID := int64(concurrent["id"].(float64))
	files := []int64{}
	for i := 0; i < 7; i++ {
		files = append(files, upload("OPENING", fmt.Sprintf("concurrent-%d.png", i), pngContent).ID)
	}
	add(concurrentID, files[:3], 200)
	var wg sync.WaitGroup
	statuses := make(chan int, 4)
	for _, id := range files[3:] {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			res := serveJSON(r, "POST", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", concurrentID), fmt.Sprintf(`{"fileIds":[%d]}`, id), f.token)
			statuses <- res.Code
		}(id)
	}
	wg.Wait()
	close(statuses)
	success := 0
	for status := range statuses {
		if status == 200 {
			success++
		} else if status != 400 {
			t.Fatalf("concurrent status %d", status)
		}
	}
	if success != 2 || len(get(concurrentID)) != 5 {
		t.Fatal("concurrent limit not enforced")
	}
	// Race reversal against removal; either serial order is valid, but the final record is locked.
	target := get(concurrentID)[0]
	wg.Add(2)
	go func() {
		defer wg.Done()
		serveJSON(r, "POST", fmt.Sprintf("/api/v1/partner-balances/entries/%d/reverse", concurrentID), `{"reason":"纠正并发记录"}`, f.token)
	}()
	go func() {
		defer wg.Done()
		serveJSON(r, "DELETE", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments/%d", concurrentID, target.ID), "", f.token)
	}()
	wg.Wait()
	current := inventoryData[receivable.Entry](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d", concurrentID), "", f.token), 200)
	if current.ReversedByID == nil {
		t.Fatal("concurrent reversal failed")
	}
	add(concurrentID, []int64{temporary.ID}, 409)
	inventoryData[any](t, serveJSON(r, "DELETE", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments/%d", concurrentID, target.ID), "", f.token), 409)
	inventoryData[any](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", *current.ReversedByID), "", f.token), 400)
	for _, a := range get(concurrentID) {
		if res := servePlain(t, r, "GET", fmt.Sprintf("/api/system/file/%d/download", a.FileID), f.token); res.Code != 200 {
			t.Fatal("reversal lost file")
		}
	}
	// Customer refund uses an actual sale/receipt/return sequence, not a synthetic balance edit.
	sale := f.trade("sales", "1", "100.00")
	f.funds("settlements", "CUSTOMER", "190.00")
	f.returnGoods("sale-returns", sale, "1")
	customerRefundFile := upload("REFUND", "customer-refund.pdf", attachmentPDF())
	customerBody := f.fundsBody("CUSTOMER", "10.00", "customer-attachment-refund")
	customerBody = customerBody[:len(customerBody)-1] + fmt.Sprintf(`,"attachmentIds":[%d]}`, customerRefundFile.ID)
	customerRefund := inventoryData[receivable.Entry](t, serveJSON(r, "POST", "/api/v1/partner-balances/refunds", customerBody, f.token), 200)
	if len(get(customerRefund.ID)) != 1 {
		t.Fatal("customer refund missing attachment")
	}
	f.balance("CUSTOMER", "-90.00")

}

func TestSQLitePartnerAttachmentUpgrade(t *testing.T) { assertPartnerAttachmentUpgrade(t, "sqlite") }
func TestPostgresPartnerAttachmentUpgrade(t *testing.T) {
	assertPartnerAttachmentUpgrade(t, "postgres")
}
func assertPartnerAttachmentUpgrade(t *testing.T, dialect string) {
	var db *platformdatabase.Database
	var migrationRoot, gooseDialect string
	if dialect == "sqlite" {
		db = openSQLiteDatabase(t, filepath.Join(t.TempDir(), "upgrade.db"))
		migrationRoot = filepath.Join(projectRoot(t), "migrations", "sqlite")
		gooseDialect = "sqlite3"
	} else {
		pg := createTestDatabase(t)
		db = openTemporaryDatabase(t, pg.dsn)
		migrationRoot = filepath.Join(projectRoot(t), "migrations")
		gooseDialect = "postgres"
	}
	defer db.Close()
	if err := goose.SetDialect(gooseDialect); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpToContext(context.Background(), db.SQL, filepath.Join(migrationRoot, "schema"), 25); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_seed_db_version")
	if err := goose.UpContext(context.Background(), db.SQL, filepath.Join(migrationRoot, "seed")); err != nil {
		t.Fatal(err)
	}
	deps := sqliteDependencies(t, db, t.TempDir())
	if dialect == "postgres" {
		deps = testDependencies(t, db, t.TempDir())
	}
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	application, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	f := newFundsFixture(t, application)
	original := f.post(openingPath, fmt.Sprintf(`{"requestKey":"legacy-attachment","partnerId":%d,"direction":"CUSTOMER","amount":"12.00","businessDate":"2026-10-01","description":"旧记录"}`, f.partner))
	var before []map[string]any
	if err := db.GORM.Table("partner_balance_entry").Order("id").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpContext(context.Background(), db.SQL, filepath.Join(migrationRoot, "schema")); err != nil {
		t.Fatal(err)
	}
	var after []map[string]any
	if err := db.GORM.Table("partner_balance_entry").Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("migration changed legacy ledger")
	}
	entry := inventoryData[receivable.Entry](t, serveJSON(application, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%v", original["id"]), "", f.token), 200)
	if entry.AttachmentCount != 0 {
		t.Fatal("legacy records should start without attachments")
	}
}

func TestSQLitePartnerAttachmentBackupRestore(t *testing.T) {
	assertAttachmentBackupRestore(t, "sqlite")
}
func TestPostgresPartnerAttachmentBackupRestore(t *testing.T) {
	assertAttachmentBackupRestore(t, "postgres")
}
func assertAttachmentBackupRestore(t *testing.T, dialect string) {
	var db *platformdatabase.Database
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	dsn := ""
	if dialect == "sqlite" {
		runSQLiteMigrations(t, sourcePath)
		db = openSQLiteDatabase(t, sourcePath)
	} else {
		pg := createTestDatabase(t)
		dsn = pg.dsn
		runMigrations(t, projectRoot(t), dsn)
		db = openTemporaryDatabase(t, dsn)
	}
	defer db.Close()
	filesRoot := t.TempDir()
	deps := sqliteDependencies(t, db, filesRoot)
	if dialect == "postgres" {
		deps = testDependencies(t, db, filesRoot)
	}
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	f := newFundsFixture(t, router)
	snapshots := map[int64]string{}
	for i := 0; i < 3; i++ {
		content := attachmentPNG(t)
		uploaded := inventoryData[filemgmt.File](t, serveMultipart(t, router, "POST", "/api/system/file/upload", f.token, []filePart{{field: "file", filename: "snapshot.png", contentType: "image/png", content: content}}, filemgmt.AttachmentPrefix+"OPENING", ""), 200)
		entry := inventoryData[receivable.Entry](t, serveJSON(router, "POST", openingPath, fmt.Sprintf(`{"requestKey":"backup-%d","partnerId":%d,"direction":"CUSTOMER","amount":"1.00","businessDate":"2026-10-01","description":"备份附件","attachmentIds":[%d]}`, i, f.partner, uploaded.ID), f.token), 200)
		snapshots[entry.ID] = content
		if i == 1 {
			items := inventoryData[[]receivable.Attachment](t, serveJSON(router, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", entry.ID), "", f.token), 200)
			inventoryData[any](t, serveJSON(router, "DELETE", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments/%d", entry.ID, items[0].ID), "", f.token), 200)
		}
		if i == 2 {
			inventoryData[any](t, serveJSON(router, "POST", fmt.Sprintf("/api/v1/partner-balances/entries/%d/reverse", entry.ID), `{"reason":"备份冲销附件"}`, f.token), 200)
		}
	}
	// Database snapshot plus a full original-file tree, matching the existing backup boundary.
	restoredFiles := t.TempDir()
	if err := filepath.WalkDir(filesRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, e := filepath.Rel(filesRoot, path)
		if e != nil {
			return e
		}
		target := filepath.Join(restoredFiles, relative)
		if d.IsDir() {
			return os.MkdirAll(target, 0750)
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		return os.WriteFile(target, data, 0600)
	}); err != nil {
		t.Fatal(err)
	}
	var restored *platformdatabase.Database
	if dialect == "sqlite" {
		backupPath := filepath.Join(t.TempDir(), "restored.db")
		if err := platformdatabase.BackupSQLite(context.Background(), sourcePath, backupPath); err != nil {
			t.Fatal(err)
		}
		restored = openSQLiteDatabase(t, backupPath)
	} else {
		dump := filepath.Join(t.TempDir(), "database.sql")
		if output, err := exec.Command("pg_dump", "--no-owner", "--no-privileges", "--dbname", dsn, "--file", dump).CombinedOutput(); err != nil {
			t.Fatalf("pg_dump: %v %s", err, output)
		}
		target := createTestDatabase(t)
		if output, err := exec.Command("psql", "--dbname", target.dsn, "--set", "ON_ERROR_STOP=1", "--file", dump).CombinedOutput(); err != nil {
			t.Fatalf("restore: %v %s", err, output)
		}
		restored = openTemporaryDatabase(t, target.dsn)
	}
	defer restored.Close()
	restoredDeps := sqliteDependencies(t, restored, restoredFiles)
	if dialect == "postgres" {
		restoredDeps = testDependencies(t, restored, restoredFiles)
	}
	restoredDeps.Receivable = receivable.NewService(receivable.NewRepository(restored.GORM))
	restoredRouter, err := app.Build(testAPIConfig(), restored, restoredDeps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, restoredRouter)
	for id, content := range snapshots {
		items := inventoryData[[]receivable.Attachment](t, serveJSON(restoredRouter, "GET", fmt.Sprintf("/api/v1/partner-balances/entries/%d/attachments", id), "", token), 200)
		if len(items) != 1 {
			t.Fatal("snapshot lost attachment history")
		}
		res := servePlain(t, restoredRouter, "GET", fmt.Sprintf("/api/system/file/%d/download", items[0].FileID), token)
		if res.Code != 200 || res.Body.String() != content {
			t.Fatal("snapshot lost original file")
		}
	}
}
