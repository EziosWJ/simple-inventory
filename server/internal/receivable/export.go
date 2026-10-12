package receivable

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/platform/exportfile"
	"github.com/EziosWJ/simple-inventory/server/internal/platform/xlsx"
)

func (s *Service) ExportEntries(ctx context.Context, q EntryFilter, actorID int64) ([]byte, string, error) {
	q, err := normalizeFilter(q, false)
	if err != nil {
		return nil, "", err
	}
	data, err := s.store.LedgerExport(ctx, q, actorID)
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	rows := make([][]any, 0, len(data.Records)+1)
	total := new(big.Rat)
	type group struct {
		PartnerID       int64
		Name, Direction string
		Count           int
		Amount          *big.Rat
	}
	groups := map[string]*group{}
	for _, r := range data.Records {
		kind := entryExportLabel[r.EntryType]
		if kind == "" {
			kind = r.EntryType
		}
		actor := r.OperatorName
		if actor == "" {
			actor = fmt.Sprintf("用户 %d", r.OperatorID)
		}
		rows = append(rows, []any{r.PartnerName, exportDirection(r.Direction), kind, r.DocumentNo, r.BusinessDate, exportfile.BusinessTime(r.EffectiveAt).Format("2006-01-02 15:04:05"), actor, xlsx.Amount(r.Amount), xlsx.Amount(r.BalanceBefore), xlsx.Amount(r.BalanceAfter), r.ReversedDocumentNo, r.Description})
		amount, ok := new(big.Rat).SetString(r.Amount)
		if !ok {
			return nil, "", fmt.Errorf("invalid stored amount")
		}
		total.Add(total, amount)
		key := fmt.Sprintf("%d:%s", r.PartnerID, r.Direction)
		g := groups[key]
		if g == nil {
			g = &group{PartnerID: r.PartnerID, Name: r.PartnerName, Direction: r.Direction, Amount: new(big.Rat)}
			groups[key] = g
		}
		g.Count++
		g.Amount.Add(g.Amount, amount)
	}
	rows = append(rows, []any{"合计", fmt.Sprintf("共 %d 笔", len(data.Records)), nil, nil, nil, nil, nil, xlsx.Amount(total.FloatString(2))})
	summary := [][]any{{"导出时间", exportfile.BusinessTime(now).Format("2006-01-02 15:04:05")}, {"导出人", data.ExportedBy}, {"往来单位", data.PartnerName}, {"往来单位ID", q.PartnerID}, {"单位关键词", q.PartnerKeyword}, {"方向", exportDirection(q.Direction)}, {"生效起点（含）", exportInstant(q.From, false)}, {"生效终点（不含）", exportInstant(q.To, false)}, {"分类", q.Category}, {"来源单号", q.DocumentNo}, {"冲销状态", q.ReversalStatus}, {"业务日期起", q.BusinessFrom}, {"业务日期止", q.BusinessTo}, {"匹配明细笔数", len(data.Records)}, {}}
	headers := []int{}
	if data.Statement != nil {
		summary = append(summary, []any{"期间期初", "期间变动", "期间期末"})
		headers = append(headers, len(summary))
		statement := data.Statement
		summary = append(summary, []any{xlsx.Amount(statement.OpeningAmount), xlsx.Amount(statement.NetChange), xlsx.Amount(statement.ClosingAmount)})
	} else {
		summary = append(summary, []any{"往来单位", "方向", "笔数", "变动合计"})
		headers = append(headers, len(summary))
		ordered := make([]*group, 0, len(groups))
		for _, g := range groups {
			ordered = append(ordered, g)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].PartnerID != ordered[j].PartnerID {
				return ordered[i].PartnerID < ordered[j].PartnerID
			}
			return ordered[i].Direction < ordered[j].Direction
		})
		for _, g := range ordered {
			summary = append(summary, []any{g.Name, exportDirection(g.Direction), g.Count, xlsx.Amount(g.Amount.FloatString(2))})
		}
	}
	bytes, err := xlsx.Build(xlsx.Sheet{Name: "明细", Headers: []string{"往来单位", "方向", "类型", "来源单号", "业务日期", "实际生效", "实际操作人", "变动金额", "前余额", "后余额", "关联冲销单号", "说明"}, Rows: rows}, xlsx.Sheet{Name: "合计", Rows: summary, HeaderRows: headers})
	name := "往来明细_关键词_" + q.PartnerKeyword + "_" + exportfile.BusinessTime(now).Format("20060102")
	if q.PartnerID > 0 {
		name = "往来明细_" + exportDirection(q.Direction) + "_" + data.PartnerName + "_" + exportfile.ExportDateRange(exportInstant(q.From, true), exportEndDate(q.To), now)
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune("/\\:*?\"<>|", r) || r < 32 {
			return '_'
		}
		return r
	}, name)
	return bytes, name + ".xlsx", err
}
func exportDirection(direction string) string {
	switch direction {
	case "CUSTOMER":
		return "客户"
	case "SUPPLIER":
		return "供应商"
	}
	return "全部方向"
}
func exportInstant(value string, date bool) string {
	if value == "" {
		return ""
	}
	t, _ := time.Parse(time.RFC3339Nano, value)
	if date {
		return exportfile.BusinessTime(t).Format("2006-01-02")
	}
	return exportfile.BusinessTime(t).Format("2006-01-02 15:04:05")
}
func exportEndDate(value string) string {
	if value == "" {
		return ""
	}
	t, _ := time.Parse(time.RFC3339Nano, value)
	return exportfile.BusinessTime(t.Add(-time.Nanosecond)).Format("2006-01-02")
}

var entryExportLabel = map[string]string{"OPENING": "期初", "RECEIPT": "客户收款", "PAYMENT": "供应商付款", "CUSTOMER_REFUND": "客户退款", "SUPPLIER_REFUND": "供应商退款", "PURCHASE": "采购入库", "SALE": "销售出库", "PURCHASE_RETURN": "采购退货", "SALE_RETURN": "销售退货", "REVERSAL": "取消 / 往来冲销"}
