package purchase

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"gorm.io/gorm"
)

const purchaseReturnTotals = "(SELECT ri.purchase_item_id AS purchase_item_id,SUM(ri.quantity_milli) AS returned_quantity,SUM(ri.amount_cents) AS returned_amount FROM purchase_return_document_item ri JOIN purchase_return_document rd ON rd.id=ri.document_id WHERE rd.status='POSTED' GROUP BY ri.purchase_item_id)"

type purchaseItemRow struct {
	ID                   int64        `gorm:"column:id"`
	PurchaseID           int64        `gorm:"column:purchase_id"`
	PartnerID            int64        `gorm:"column:partner_id"`
	ProductID            int64        `gorm:"column:product_id"`
	DocumentNo           string       `gorm:"column:document_no"`
	PartnerName          string       `gorm:"column:partner_name"`
	BusinessDate         BusinessDate `gorm:"column:business_date"`
	Status               string       `gorm:"column:status"`
	ProductCode          string       `gorm:"column:product_code"`
	ProductName          string       `gorm:"column:product_name"`
	ProductModel         *string      `gorm:"column:product_model"`
	ProductSpecification *string      `gorm:"column:product_specification"`
	Unit                 string       `gorm:"column:unit"`
	Quantity             int64        `gorm:"column:quantity_milli"`
	UnitPrice            int64        `gorm:"column:unit_price_cents"`
	Amount               int64        `gorm:"column:amount_cents"`
	ReturnedQuantity     int64        `gorm:"column:returned_quantity"`
	ReturnedAmount       int64        `gorm:"column:returned_amount"`
	MatchesHistory       bool         `gorm:"column:matches_history"`
}
type purchaseAmountRow struct {
	Status             string
	ItemCount          int64
	Original, Returned int64
}
type purchaseQuantityRow struct {
	Status, ProductCode, ProductName, Unit string
	ProductID, Quantity, Returned          int64
}

func (r *Repository) ItemPage(ctx context.Context, q ItemQuery) (ItemPage, error) {
	out := ItemPage{Records: []ItemRecord{}, Page: q.Page, PageSize: q.PageSize, Summary: ItemSummary{Amounts: []ItemAmounts{}, Quantities: []ItemQuantity{}}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		count, _, _ := purchaseItemQuery(tx, q)
		if err := count.Count(&out.Total).Error; err != nil {
			return err
		}
		rows, historySQL, historyArgs := purchaseItemQuery(tx, q)
		var found []purchaseItemRow
		selectSQL := "i.id,i.document_id AS purchase_id,d.partner_id,d.document_no,COALESCE(p.name,'') AS partner_name,d.business_date,d.status,i.product_id,i.product_code,i.product_name,i.product_model,i.product_specification,i.unit,i.quantity_milli,i.unit_price_cents,i.amount_cents,CAST(COALESCE(r.returned_quantity,0) AS BIGINT) AS returned_quantity,CAST(COALESCE(r.returned_amount,0) AS BIGINT) AS returned_amount," + historySQL + " AS matches_history"
		if err := rows.Select(selectSQL, historyArgs...).Order(purchaseItemOrder(q)).Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&found).Error; err != nil {
			return err
		}
		for _, row := range found {
			out.Records = append(out.Records, purchaseItemRecord(row))
		}
		amounts, err := purchaseItemAmounts(tx, q)
		if err != nil {
			return err
		}
		quantities, err := purchaseItemQuantities(tx, q)
		if err != nil {
			return err
		}
		out.Summary = ItemSummary{Amounts: amounts, Quantities: quantities}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}

func purchaseItemQuery(tx *gorm.DB, q ItemQuery) (*gorm.DB, string, []any) {
	d := tx.Table("purchase_document d").Joins("JOIN purchase_document_item i ON i.document_id=d.id").Joins("LEFT JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN " + purchaseReturnTotals + " r ON r.purchase_item_id=i.id")
	if q.DocumentNo != "" {
		d = platformdatabase.LiteralContains(d, q.DocumentNo, "d.document_no")
	}
	if q.Status != "" {
		d = d.Where("d.status=?", q.Status)
	}
	if q.DirectDelivery != "" {
		d = d.Where("d.direct_delivery=?", q.DirectDelivery == "true")
	}
	if q.PartnerID > 0 {
		d = d.Where("d.partner_id=?", q.PartnerID)
	}
	if q.BusinessFrom != "" {
		d = d.Where("d.business_date>=?", q.BusinessFrom)
	}
	if q.BusinessTo != "" {
		d = d.Where("d.business_date<=?", q.BusinessTo)
	}
	if q.ProductID > 0 {
		d = d.Where("i.product_id=?", q.ProductID)
	}
	history := []string{"FALSE"}
	historyArgs := []any{}
	if strings.TrimSpace(q.PartnerKeyword) != "" {
		match, args := platformdatabase.WordsCondition(d, q.PartnerKeyword, "p.code", "p.name", "p.contact", "p.phone")
		d = d.Where("EXISTS(SELECT 1 FROM partner pp WHERE pp.id=d.partner_id AND "+match+")", args...)
	}
	if strings.TrimSpace(q.ProductKeyword) != "" {
		cols := []string{"pr.code", "pr.name", "pr.brand", "pr.model", "pr.specification"}
		current, ca := platformdatabase.WordsCondition(d, q.ProductKeyword, cols...)
		all := append(append([]string{}, cols...), "i.product_code", "i.product_name", "i.product_model", "i.product_specification")
		matched, args := platformdatabase.WordsCondition(d, q.ProductKeyword, all...)
		d = d.Joins("JOIN product pr ON pr.id=i.product_id").Where(matched, args...)
		history = append(history, "NOT EXISTS(SELECT 1 FROM product pr WHERE pr.id=i.product_id AND "+current+")")
		historyArgs = append(historyArgs, ca...)
	}
	return d, "(" + strings.Join(history, " OR ") + ")", historyArgs
}

func purchaseItemOrder(q ItemQuery) string {
	dir := "ASC"
	if q.SortDir == "desc" {
		dir = "DESC"
	}
	stable := "d.business_date DESC,d.id DESC,i.id ASC"
	switch q.SortBy {
	case "unitPrice":
		return "i.unit_price_cents " + dir + "," + stable
	case "amount":
		return "i.amount_cents " + dir + "," + stable
	case "netAmount":
		return "(CASE WHEN d.status='POSTED' THEN 0 ELSE 1 END) ASC,(i.amount_cents-COALESCE(r.returned_amount,0)) " + dir + "," + stable
	default:
		return "d.business_date " + dir + ",d.id DESC,i.id ASC"
	}
}
func purchaseItemRecord(r purchaseItemRow) ItemRecord {
	v := ItemRecord{ID: r.ID, PurchaseID: r.PurchaseID, DocumentNo: r.DocumentNo, PartnerID: r.PartnerID, PartnerName: r.PartnerName, BusinessDate: string(r.BusinessDate), Status: r.Status, ProductID: r.ProductID, ProductCode: r.ProductCode, ProductName: r.ProductName, ProductModel: r.ProductModel, ProductSpecification: r.ProductSpecification, Unit: r.Unit, Quantity: milliText(r.Quantity), UnitPrice: moneyText(r.UnitPrice), Amount: moneyText(r.Amount), MatchesHistory: r.MatchesHistory}
	if r.Status == "POSTED" {
		returnedQ, returnedA, netQ, netA := milliText(r.ReturnedQuantity), moneyText(r.ReturnedAmount), milliText(r.Quantity-r.ReturnedQuantity), moneyText(r.Amount-r.ReturnedAmount)
		v.ReturnedQuantity = &returnedQ
		v.ReturnedAmount = &returnedA
		v.NetQuantity = &netQ
		v.NetAmount = &netA
	}
	return v
}
func purchaseItemAmounts(tx *gorm.DB, q ItemQuery) ([]ItemAmounts, error) {
	d, _, _ := purchaseItemQuery(tx, q)
	var rows []purchaseAmountRow
	if err := d.Select("d.status AS status,COUNT(*) AS item_count,CAST(COALESCE(SUM(i.amount_cents),0) AS BIGINT) AS original,CAST(COALESCE(SUM(CASE WHEN d.status='POSTED' THEN COALESCE(r.returned_amount,0) ELSE 0 END),0) AS BIGINT) AS returned").Group("d.status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	found := map[string]purchaseAmountRow{}
	for _, row := range rows {
		found[row.Status] = row
	}
	out := make([]ItemAmounts, 0, 3)
	for _, status := range []string{"DRAFT", "POSTED", "CANCELLED"} {
		row := found[status]
		v := ItemAmounts{Status: status, ItemCount: row.ItemCount, OriginalAmount: moneyText(row.Original)}
		if status == "POSTED" {
			ret, net := moneyText(row.Returned), moneyText(row.Original-row.Returned)
			v.ReturnedAmount = &ret
			v.NetAmount = &net
		}
		out = append(out, v)
	}
	return out, nil
}
func purchaseItemQuantities(tx *gorm.DB, q ItemQuery) ([]ItemQuantity, error) {
	d, _, _ := purchaseItemQuery(tx, q)
	var rows []purchaseQuantityRow
	if err := d.Select("d.status AS status,i.product_id AS product_id,MIN(i.product_code) AS product_code,MIN(i.product_name) AS product_name,i.unit AS unit,CAST(COALESCE(SUM(i.quantity_milli),0) AS BIGINT) AS quantity,CAST(COALESCE(SUM(CASE WHEN d.status='POSTED' THEN COALESCE(r.returned_quantity,0) ELSE 0 END),0) AS BIGINT) AS returned").Group("d.status,i.product_id,i.unit").Scan(&rows).Error; err != nil {
		return nil, err
	}
	rank := func(s string) int {
		switch s {
		case "DRAFT":
			return 0
		case "POSTED":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rank(rows[i].Status) != rank(rows[j].Status) {
			return rank(rows[i].Status) < rank(rows[j].Status)
		}
		if rows[i].ProductID != rows[j].ProductID {
			return rows[i].ProductID < rows[j].ProductID
		}
		return rows[i].Unit < rows[j].Unit
	})
	out := make([]ItemQuantity, 0, len(rows))
	for _, row := range rows {
		v := ItemQuantity{Status: row.Status, ProductID: row.ProductID, ProductCode: row.ProductCode, ProductName: row.ProductName, Unit: row.Unit, Quantity: milliText(row.Quantity)}
		if row.Status == "POSTED" {
			ret, net := milliText(row.Returned), milliText(row.Quantity-row.Returned)
			v.ReturnedQuantity = &ret
			v.NetQuantity = &net
		}
		out = append(out, v)
	}
	return out, nil
}
