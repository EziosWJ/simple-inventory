package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
)

type partnerEntriesInput struct {
	pageInput
	DateFrom       string `json:"dateFrom"`
	DateTo         string `json:"dateTo"`
	PartnerID      int64  `json:"partnerId"`
	PartnerKeyword string `json:"partnerKeyword"`
	DocumentNo     string `json:"documentNo"`
	Direction      string `json:"direction"`
	Category       string `json:"category"`
	ReversalStatus string `json:"reversalStatus"`
}

type partnerStatementInput struct {
	pageInput
	DateFrom  string `json:"dateFrom"`
	DateTo    string `json:"dateTo"`
	PartnerID int64  `json:"partnerId"`
	Direction string `json:"direction"`
}

func blank(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// entryView projects one ledger row. description carries the manual opening
// remark, the settlement funds note or the reversal reason; transactionNo is a
// payment reference, never a bank account.
func entryView(e receivable.Entry) map[string]any {
	return map[string]any{
		"id": e.ID, "partnerId": e.PartnerID, "partnerName": e.PartnerName, "direction": e.Direction,
		"entryType": e.EntryType, "businessDate": e.BusinessDate, "effectiveAt": e.EffectiveAt,
		"documentNo": e.DocumentNo, "amount": e.Amount, "balanceBefore": e.BalanceBefore, "balanceAfter": e.BalanceAfter,
		"description": e.Description, "operatorId": e.OperatorID, "operatorName": e.OperatorName,
		"paymentMethod": blank(e.PaymentMethod), "transactionNo": blank(e.TransactionNo),
		"reversesId": e.ReversesID, "reversedById": e.ReversedByID, "reversedDocumentNo": blank(e.ReversedDocumentNo),
		"purchaseId": e.PurchaseID, "saleId": e.SaleID,
		"purchaseReturnId": e.PurchaseReturnID, "saleReturnId": e.SaleReturnID,
		"attachmentCount": e.AttachmentCount,
	}
}

func entryViews(entries []receivable.Entry) []map[string]any {
	out := []map[string]any{}
	for _, e := range entries {
		out = append(out, entryView(e))
	}
	return out
}

const ledgerSemantics = "金额为带符号定点字符串，正负按方向语义解释；交易流水号是支付参考，不是银行账号。" +
	"余额按实际生效时间累计；记录同时返回原业务日期。已冲销的原始记录与反向记录都保留，不能删除原记录后计算对账。"

func (s Services) partnerEntries(ctx context.Context, raw json.RawMessage, maxPage int) (map[string]any, int, error) {
	var in partnerEntriesInput
	fields, err := decode(raw, &in)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize, err := resolvePage(in.pageInput, fields, maxPage)
	if err != nil {
		return nil, 0, err
	}
	if in.PartnerID < 0 || len(in.PartnerKeyword) > 100 || len(in.DocumentNo) > 100 ||
		!enum(in.Direction, "", "CUSTOMER", "SUPPLIER") ||
		!enum(in.Category, "", "OPENING", "SETTLEMENT", "REFUND") ||
		!enum(in.ReversalStatus, "", "EFFECTIVE", "REVERSED", "REVERSAL") {
		return nil, 0, errInvalidArguments
	}
	from, to, _, err := businessRange(in.DateFrom, in.DateTo)
	if err != nil {
		return nil, 0, err
	}
	effectiveFrom, effectiveTo, err := effectiveRange(from, to)
	if err != nil {
		return nil, 0, err
	}
	result, e := s.Receivable.FilterPage(ctx, receivable.EntryFilter{
		PartnerID: in.PartnerID, PartnerKeyword: in.PartnerKeyword, DocumentNo: in.DocumentNo,
		Direction: in.Direction, Category: in.Category, ReversalStatus: in.ReversalStatus,
		From: effectiveFrom, To: effectiveTo, Page: page, PageSize: pageSize,
	})
	if e != nil {
		return nil, 0, e
	}
	records := entryViews(result.Records)
	return map[string]any{
		"timeZone":  toolTimeZone,
		"timeBasis": "实际生效时刻，左闭右开 [dateFrom 00:00, dateTo 次日 00:00) Asia/Shanghai；记录同时返回原业务日期",
		"queriedAt": nowISO(),
		"semantics": ledgerSemantics,
		"filtersApplied": map[string]any{
			"dateFrom": from, "dateTo": to, "partnerId": in.PartnerID, "partnerKeyword": in.PartnerKeyword,
			"documentNo": in.DocumentNo, "direction": in.Direction, "category": in.Category, "reversalStatus": in.ReversalStatus,
		},
		"page": page, "pageSize": pageSize, "total": result.Total, "hasMore": int64(page*pageSize) < result.Total,
		"records": records,
	}, len(records), nil
}

func (s Services) partnerStatement(ctx context.Context, raw json.RawMessage, maxPage int) (map[string]any, int, error) {
	var in partnerStatementInput
	fields, err := decode(raw, &in)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize, err := resolvePage(in.pageInput, fields, maxPage)
	if err != nil {
		return nil, 0, err
	}
	if in.PartnerID < 1 || in.PartnerID > 9007199254740991 || !enum(in.Direction, "", "CUSTOMER", "SUPPLIER") {
		return nil, 0, errInvalidArguments
	}
	from, to, _, err := businessRange(in.DateFrom, in.DateTo)
	if err != nil {
		return nil, 0, err
	}
	effectiveFrom, effectiveTo, err := effectiveRange(from, to)
	if err != nil {
		return nil, 0, err
	}
	// Both identities stay separate: an omitted direction reports each one
	// rather than netting a customer balance against a supplier balance.
	directions := []string{in.Direction}
	if in.Direction == "" {
		directions = []string{"CUSTOMER", "SUPPLIER"}
	}
	statements := []map[string]any{}
	total := 0
	for _, direction := range directions {
		st, e := s.Receivable.Statement(ctx, receivable.EntryFilter{
			PartnerID: in.PartnerID, Direction: direction,
			From: effectiveFrom, To: effectiveTo, Page: page, PageSize: pageSize,
		})
		if e != nil {
			return nil, 0, e
		}
		records := entryViews(st.Records)
		total += len(records)
		row := map[string]any{
			"direction": direction, "openingAmount": st.OpeningAmount,
			"increaseAmount": st.IncreaseAmount, "decreaseAmount": st.DecreaseAmount,
			"netChange": st.NetChange, "closingAmount": st.ClosingAmount,
			"total": st.Total, "page": page, "pageSize": pageSize, "hasMore": int64(page*pageSize) < st.Total,
			"records": records,
		}
		if st.Total == 0 && st.OpeningAmount == "0.00" {
			row["note"] = "该方向截至期末没有往来记录；上一级 filtersApplied.partnerId 指向的单位本身存在。"
		}
		statements = append(statements, row)
		if in.Direction == "" {
			// Records of both directions share one response, so page the same
			// window for each and report the totals independently.
			continue
		}
	}
	return map[string]any{
		"timeZone":  toolTimeZone,
		"timeBasis": "实际生效时刻，左闭右开 [dateFrom 00:00, dateTo 次日 00:00) Asia/Shanghai；对账行同时返回原业务日期",
		"queriedAt": nowISO(),
		"semantics": ledgerSemantics + "期初、期间增加／减少、净变化与期末来自同一业务口径；合计覆盖完整期间，不是当前页；" +
			"余额不代表某张单据未结金额。客户正余额为客户欠经营者，供应商正余额为经营者欠供应商，两方向不自动抵扣。",
		"filtersApplied": map[string]any{"partnerId": in.PartnerID, "direction": in.Direction, "dateFrom": from, "dateTo": to},
		"statements":     statements,
	}, total, nil
}
