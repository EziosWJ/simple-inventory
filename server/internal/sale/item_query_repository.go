package sale

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"gorm.io/gorm"
)

// saleReturnTotals aggregates the sale returns that are still posted for each
// original sale line. Aggregating before the join keeps several returns from
// multiplying the original quantity, amount or row count.
const saleReturnTotals = "(SELECT ri.sale_item_id AS sale_item_id,SUM(ri.quantity_milli) AS returned_quantity,SUM(ri.amount_cents) AS returned_amount FROM sale_return_document_item ri JOIN sale_return_document rd ON rd.id=ri.document_id WHERE rd.status='POSTED' GROUP BY ri.sale_item_id)"

const itemColumns = "i.id AS id,d.id AS sale_id,d.document_no AS document_no,d.partner_id AS partner_id," +
	"COALESCE(NULLIF(d.partner_name,''),p.name) AS partner_name,d.business_date AS business_date,d.status AS status," +
	"i.product_id AS product_id,i.product_code AS product_code,i.product_name AS product_name," +
	"i.product_model AS product_model,i.product_specification AS product_specification,i.product_type AS product_type," +
	"i.unit AS unit,i.quantity_milli AS quantity_milli,i.unit_price_cents AS unit_price_cents,i.amount_cents AS amount_cents," +
	"CAST(COALESCE(r.returned_quantity,0) AS BIGINT) AS returned_quantity_milli," +
	"CAST(COALESCE(r.returned_amount,0) AS BIGINT) AS returned_amount_cents"

type itemRow struct {
	BusinessDate          BusinessDate `gorm:"column:business_date"`
	ProductModel          *string      `gorm:"column:product_model"`
	ProductSpecification  *string      `gorm:"column:product_specification"`
	DocumentNo            string       `gorm:"column:document_no"`
	PartnerName           string       `gorm:"column:partner_name"`
	Status                string       `gorm:"column:status"`
	ProductCode           string       `gorm:"column:product_code"`
	ProductName           string       `gorm:"column:product_name"`
	ProductType           string       `gorm:"column:product_type"`
	Unit                  string       `gorm:"column:unit"`
	ID                    int64        `gorm:"column:id"`
	SaleID                int64        `gorm:"column:sale_id"`
	PartnerID             int64        `gorm:"column:partner_id"`
	ProductID             int64        `gorm:"column:product_id"`
	QuantityMilli         int64        `gorm:"column:quantity_milli"`
	UnitPriceCents        int64        `gorm:"column:unit_price_cents"`
	AmountCents           int64        `gorm:"column:amount_cents"`
	ReturnedQuantityMilli int64        `gorm:"column:returned_quantity_milli"`
	ReturnedAmountCents   int64        `gorm:"column:returned_amount_cents"`
	MatchesHistory        bool         `gorm:"column:matches_history"`
}

type itemAmountRow struct {
	Status         string `gorm:"column:status"`
	ItemCount      int64  `gorm:"column:item_count"`
	OriginalAmount int64  `gorm:"column:original_amount"`
	ReturnedAmount int64  `gorm:"column:returned_amount"`
}

type itemQuantityRow struct {
	Status           string `gorm:"column:status"`
	ProductCode      string `gorm:"column:product_code"`
	ProductName      string `gorm:"column:product_name"`
	ProductType      string `gorm:"column:product_type"`
	Unit             string `gorm:"column:unit"`
	ProductID        int64  `gorm:"column:product_id"`
	QuantityMilli    int64  `gorm:"column:quantity_milli"`
	ReturnedQuantity int64  `gorm:"column:returned_quantity_milli"`
}

// ItemPage reads the matching sale lines, their count and the summary of every
// matching line in one repeatable read-only transaction, so a concurrent
// posting, return or cancellation cannot make the three disagree.
func (r *Repository) ItemPage(ctx context.Context, q ItemQuery) (ItemPage, error) {
	out := ItemPage{Records: []ItemRecord{}, Page: q.Page, PageSize: q.PageSize, Summary: ItemSummary{Amounts: []ItemAmounts{}, Quantities: []ItemQuantity{}}}
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		count, _, _ := itemQuery(tx, q)
		if e := count.Count(&out.Total).Error; e != nil {
			return e
		}
		rows, history, historyArgs := itemQuery(tx, q)
		var items []itemRow
		if e := rows.Select(itemColumns+","+history+" AS matches_history", historyArgs...).Order(itemOrder(q)).Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&items).Error; e != nil {
			return e
		}
		for i := range items {
			out.Records = append(out.Records, itemRecord(&items[i]))
		}
		amounts, e := itemAmounts(tx, q)
		if e != nil {
			return e
		}
		quantities, e := itemQuantities(tx, q)
		if e != nil {
			return e
		}
		out.Summary = ItemSummary{Amounts: amounts, Quantities: quantities}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return out, e
	}
	return out, nil
}

// itemQuery returns a fresh query per caller: the count, the page and the two
// summaries each add their own select and grouping to the same filter.
func itemQuery(tx *gorm.DB, q ItemQuery) (*gorm.DB, string, []any) {
	d := tx.Table("sale_document d").
		Joins("JOIN sale_document_item i ON i.document_id=d.id").
		Joins("JOIN partner p ON p.id=d.partner_id").
		Joins("LEFT JOIN " + saleReturnTotals + " r ON r.sale_item_id=i.id")
	if strings.TrimSpace(q.DocumentNo) != "" {
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
		match, args := platformdatabase.WordsCondition(d, q.PartnerKeyword, "pp.code", "pp.name", "pp.contact", "pp.phone", "d.partner_name")
		d = d.Where("EXISTS(SELECT 1 FROM partner pp WHERE pp.id=d.partner_id AND "+match+")", args...)
		current, ca := platformdatabase.WordsCondition(d, q.PartnerKeyword, "pp.code", "pp.name", "pp.contact", "pp.phone")
		history = append(history, "NOT EXISTS(SELECT 1 FROM partner pp WHERE pp.id=d.partner_id AND "+current+")")
		historyArgs = append(historyArgs, ca...)
	}
	if strings.TrimSpace(q.ProductKeyword) != "" {
		match, args, historical, ha := productLineMatch(d, q.ProductKeyword)
		d = d.Joins("JOIN product pr ON pr.id=i.product_id").Where(match, args...)
		history = append(history, historical)
		historyArgs = append(historyArgs, ha...)
	}
	return d, "(" + strings.Join(history, " OR ") + ")", historyArgs
}

func itemOrder(q ItemQuery) string {
	stable := "d.business_date DESC,d.id DESC,i.id ASC"
	direction := "ASC"
	if q.SortDir == "desc" {
		direction = "DESC"
	}
	switch q.SortBy {
	case "unitPrice":
		return "i.unit_price_cents " + direction + "," + stable
	case "amount":
		return "i.amount_cents " + direction + "," + stable
	case "netAmount":
		// Lines whose net is not applicable stay behind the effective ones in
		// both directions instead of sorting as if their net were zero.
		return "(CASE WHEN d.status='POSTED' THEN 0 ELSE 1 END) ASC,(i.amount_cents-COALESCE(r.returned_amount,0)) " + direction + "," + stable
	case "businessDate":
		return "d.business_date " + direction + ",d.id DESC,i.id ASC"
	}
	return stable
}

func itemRecord(r *itemRow) ItemRecord {
	v := ItemRecord{
		MatchesHistory:       r.MatchesHistory,
		ID:                   r.ID,
		SaleID:               r.SaleID,
		DocumentNo:           r.DocumentNo,
		PartnerID:            r.PartnerID,
		PartnerName:          r.PartnerName,
		BusinessDate:         string(r.BusinessDate),
		Status:               r.Status,
		ProductID:            r.ProductID,
		ProductCode:          r.ProductCode,
		ProductName:          r.ProductName,
		ProductModel:         r.ProductModel,
		ProductSpecification: r.ProductSpecification,
		ProductType:          r.ProductType,
		Unit:                 r.Unit,
		Quantity:             milliText(r.QuantityMilli),
		UnitPrice:            moneyText(r.UnitPriceCents),
		Amount:               moneyText(r.AmountCents),
		ReturnsApplicable:    r.ProductType == "GOODS",
		ReturnedQuantity:     milliText(0),
		ReturnedAmount:       moneyText(0),
	}
	if v.ReturnsApplicable {
		v.ReturnedQuantity = milliText(r.ReturnedQuantityMilli)
		v.ReturnedAmount = moneyText(r.ReturnedAmountCents)
	}
	if r.Status == "POSTED" {
		quantity, amount := milliText(r.QuantityMilli-r.ReturnedQuantityMilli), moneyText(r.AmountCents-r.ReturnedAmountCents)
		v.NetApplicable, v.NetQuantity, v.NetAmount = true, &quantity, &amount
	}
	return v
}

func itemAmounts(tx *gorm.DB, q ItemQuery) ([]ItemAmounts, error) {
	d, _, _ := itemQuery(tx, q)
	var rows []itemAmountRow
	if e := d.Select("d.status AS status,COUNT(*) AS item_count," +
		"CAST(COALESCE(SUM(i.amount_cents),0) AS BIGINT) AS original_amount," +
		"CAST(COALESCE(SUM(CASE WHEN d.status='POSTED' THEN COALESCE(r.returned_amount,0) ELSE 0 END),0) AS BIGINT) AS returned_amount").
		Group("d.status").Scan(&rows).Error; e != nil {
		return nil, e
	}
	found := map[string]itemAmountRow{}
	for _, row := range rows {
		found[row.Status] = row
	}
	out := make([]ItemAmounts, 0, 3)
	for _, status := range []string{"DRAFT", "POSTED", "CANCELLED"} {
		row := found[status]
		v := ItemAmounts{Status: status, ItemCount: row.ItemCount, OriginalAmount: moneyText(row.OriginalAmount)}
		if status == "POSTED" {
			returned, net := moneyText(row.ReturnedAmount), moneyText(row.OriginalAmount-row.ReturnedAmount)
			v.ReturnedAmount, v.NetAmount = &returned, &net
		}
		out = append(out, v)
	}
	return out, nil
}

func itemQuantities(tx *gorm.DB, q ItemQuery) ([]ItemQuantity, error) {
	d, _, _ := itemQuery(tx, q)
	var rows []itemQuantityRow
	// The description comes from the group's own saved snapshots, so a renamed
	// archive never relabels history and one identity keeps one group.
	if e := d.Select("d.status AS status,i.product_id AS product_id,MIN(i.product_code) AS product_code," +
		"MIN(i.product_name) AS product_name,MIN(i.product_type) AS product_type,i.unit AS unit," +
		"CAST(COALESCE(SUM(i.quantity_milli),0) AS BIGINT) AS quantity_milli," +
		"CAST(COALESCE(SUM(CASE WHEN d.status='POSTED' THEN COALESCE(r.returned_quantity,0) ELSE 0 END),0) AS BIGINT) AS returned_quantity_milli").
		Group("d.status,i.product_id,i.unit").Order("i.product_id").Order("i.unit").Scan(&rows).Error; e != nil {
		return nil, e
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if statusRank(a.Status) != statusRank(b.Status) {
			return statusRank(a.Status) < statusRank(b.Status)
		}
		if a.ProductID != b.ProductID {
			return a.ProductID < b.ProductID
		}
		return a.Unit < b.Unit
	})
	out := make([]ItemQuantity, 0, len(rows))
	for _, row := range rows {
		v := ItemQuantity{ProductID: row.ProductID, ProductCode: row.ProductCode, ProductName: row.ProductName, ProductType: row.ProductType, Unit: row.Unit, Status: row.Status, Quantity: milliText(row.QuantityMilli), ReturnsApplicable: row.ProductType == "GOODS"}
		if v.ReturnsApplicable {
			returned := milliText(row.ReturnedQuantity)
			v.ReturnedQuantity = &returned
		}
		if row.Status == "POSTED" {
			net := milliText(row.QuantityMilli - row.ReturnedQuantity)
			v.NetQuantity = &net
		}
		out = append(out, v)
	}
	return out, nil
}

func concatArgs(a, b []any) []any {
	out := make([]any, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}

// productLineMatch is shared by document selection, line markers and item
// queries. All words may match current archive fields or the saved snapshot.
func productLineMatch(db *gorm.DB, keyword string) (string, []any, string, []any) {
	columns := []string{"pr.code", "pr.name", "pr.brand", "pr.model", "pr.specification"}
	current, ca := platformdatabase.WordsCondition(db, keyword, columns...)
	columns = append(columns, "i.product_code", "i.product_name", "i.product_model", "i.product_specification")
	match, args := platformdatabase.WordsCondition(db, keyword, columns...)
	return match, args, "(" + match + " AND NOT COALESCE(" + current + ",FALSE))", concatArgs(args, ca)
}
