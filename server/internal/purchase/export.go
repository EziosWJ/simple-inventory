package purchase

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/platform/exportfile"
	"github.com/EziosWJ/simple-inventory/server/internal/platform/xlsx"
)

func (s *Service) ExportItems(ctx context.Context, q ItemQuery, actorID int64) ([]byte, string, error) {
	q, err := normalizeItemQuery(q)
	if err != nil {
		return nil, "", err
	}
	data, err := s.store.ItemExport(ctx, q, actorID)
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	rows := make([][]any, 0, len(data.Records)+1)
	total := new(big.Rat)
	for _, r := range data.Records {
		delivery := "普通"
		if r.DirectDelivery {
			delivery = "直送"
		}

		rows = append(rows, []any{r.DocumentNo, r.PartnerName, delivery, r.BusinessDate, exportStatus(r.Status), r.ProductCode, r.ProductName, r.ProductModel, r.ProductSpecification, r.Unit, xlsx.Decimal(r.Quantity), xlsx.Amount(r.UnitPrice), xlsx.Amount(r.Amount), xlsx.Optional(r.ReturnedQuantity, false), xlsx.Optional(r.ReturnedAmount, true), xlsx.Optional(r.NetQuantity, false), xlsx.Optional(r.NetAmount, true)})
		amount, ok := new(big.Rat).SetString(r.Amount)
		if !ok {
			return nil, "", fmt.Errorf("invalid stored amount")
		}
		total.Add(total, amount)
	}
	footer := make([]any, 17)
	footer[0] = "合计"
	footer[1] = fmt.Sprintf("共 %d 行", data.Total)
	footer[12] = xlsx.Amount(total.FloatString(2))
	rows = append(rows, footer)
	summary := [][]any{
		{"导出时间", exportfile.BusinessTime(now).Format("2006-01-02 15:04:05")},
		{"导出人", data.ExportedBy},
		{"单号", q.DocumentNo}, {"供应商ID", q.PartnerID}, {"供应商关键词", q.PartnerKeyword}, {"商品ID", q.ProductID}, {"商品关键词", q.ProductKeyword},
		{"状态", exportStatus(q.Status)}, {"交付方式", exportDelivery(q.DirectDelivery)}, {"业务日期起", q.BusinessFrom}, {"业务日期止", q.BusinessTo},
		{"排序字段", q.SortBy}, {"排序方向", q.SortDir}, {"匹配明细行数", data.Total}, {},
		{"状态", "明细行数", "原金额", "有效退货金额", "净金额"},
	}
	for _, r := range data.Summary.Amounts {
		summary = append(summary, []any{exportStatus(r.Status), r.ItemCount, xlsx.Amount(r.OriginalAmount), xlsx.Optional(r.ReturnedAmount, true), xlsx.Optional(r.NetAmount, true)})
	}
	summary = append(summary, []any{}, []any{"状态", "商品编码", "商品名称", "单位", "数量", "有效退货数量", "净数量"})
	quantityHeader := len(summary)
	for _, r := range data.Summary.Quantities {
		summary = append(summary, []any{exportStatus(r.Status), r.ProductCode, r.ProductName, r.Unit, xlsx.Decimal(r.Quantity), xlsx.Optional(r.ReturnedQuantity, false), xlsx.Optional(r.NetQuantity, false)})
	}
	bytes, err := xlsx.Build(xlsx.Sheet{Name: "明细", Headers: []string{"采购单号", "供应商", "交付方式", "业务日期", "状态", "商品编码", "商品名称", "型号", "规格", "单位", "数量", "成交单价", "原金额", "有效退货数量", "有效退货金额", "净采购数量", "净采购金额"}, Rows: rows}, xlsx.Sheet{Name: "合计", Rows: summary, HeaderRows: []int{16, quantityHeader}})
	return bytes, "采购明细_" + exportfile.ExportDateRange(q.BusinessFrom, q.BusinessTo, now) + ".xlsx", err
}
func exportStatus(status string) string {
	switch status {
	case "DRAFT":
		return "草稿"
	case "POSTED":
		return "已过账"
	case "CANCELLED":
		return "已取消"
	}
	return "全部"
}
func exportDelivery(value string) string {
	switch value {
	case "true":
		return "直送"
	case "false":
		return "普通"
	}
	return "全部"
}
