package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
)

type purchaseItemsInput struct {
	pageInput
	DateFrom       string `json:"dateFrom"`
	DateTo         string `json:"dateTo"`
	PartnerID      int64  `json:"partnerId"`
	PartnerKeyword string `json:"partnerKeyword"`
	ProductID      int64  `json:"productId"`
	ProductKeyword string `json:"productKeyword"`
	DocumentNo     string `json:"documentNo"`
	Status         string `json:"status"`
	DirectDelivery string `json:"directDelivery"`
	SortBy         string `json:"sortBy"`
	SortDir        string `json:"sortDir"`
}

type purchaseDocumentInput struct {
	PurchaseID int64 `json:"purchaseId"`
}

// documentStatus turns the explicit tool choice into the service status. The
// business default is a posted document: drafts and cancelled documents are
// never mixed into an effective total by accident.
func documentStatus(status string) (string, error) {
	switch status {
	case "":
		return "POSTED", nil
	case "ALL":
		return "", nil
	case "DRAFT", "POSTED", "CANCELLED":
		return status, nil
	}
	return "", errInvalidArguments
}

func purchaseAmounts(summary purchase.ItemSummary) ([]map[string]any, []map[string]any) {
	amounts := []map[string]any{}
	for _, a := range summary.Amounts {
		amounts = append(amounts, map[string]any{"status": a.Status, "itemCount": a.ItemCount, "originalAmount": a.OriginalAmount, "returnedAmount": a.ReturnedAmount, "netAmount": a.NetAmount})
	}
	quantities := []map[string]any{}
	for _, q := range summary.Quantities {
		quantities = append(quantities, map[string]any{"status": q.Status, "productId": q.ProductID, "productCode": q.ProductCode, "productName": q.ProductName, "unit": q.Unit, "quantity": q.Quantity, "returnedQuantity": q.ReturnedQuantity, "netQuantity": q.NetQuantity})
	}
	return amounts, quantities
}

func (s Services) purchaseItems(ctx context.Context, raw json.RawMessage, maxPage int) (map[string]any, int, error) {
	var in purchaseItemsInput
	fields, err := decode(raw, &in)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize, err := resolvePage(in.pageInput, fields, maxPage)
	if err != nil {
		return nil, 0, err
	}
	status, err := documentStatus(in.Status)
	if err != nil {
		return nil, 0, err
	}
	if !enum(in.DirectDelivery, "", "true", "false") {
		return nil, 0, errInvalidArguments
	}
	if in.PartnerID < 0 || in.ProductID < 0 || len(in.PartnerKeyword) > 100 || len(in.ProductKeyword) > 100 || len(in.DocumentNo) > 100 {
		return nil, 0, errInvalidArguments
	}
	if (in.PartnerID > 0 && in.PartnerKeyword != "") || (in.ProductID > 0 && in.ProductKeyword != "") {
		return nil, 0, errInvalidArguments
	}
	from, to, _, err := businessRange(in.DateFrom, in.DateTo)
	if err != nil {
		return nil, 0, err
	}
	result, e := s.Purchase.ItemPage(ctx, purchase.ItemQuery{
		Query: purchase.Query{
			Page: page, PageSize: pageSize, DocumentNo: in.DocumentNo, Status: status,
			DirectDelivery: in.DirectDelivery, PartnerID: in.PartnerID, ProductID: in.ProductID,
			PartnerKeyword: in.PartnerKeyword, ProductKeyword: in.ProductKeyword,
			BusinessFrom: from, BusinessTo: to,
		},
		SortBy: in.SortBy, SortDir: in.SortDir,
	})
	if e != nil {
		return nil, 0, e
	}
	records := []map[string]any{}
	for _, r := range result.Records {
		row := map[string]any{
			"id": r.ID, "purchaseId": r.PurchaseID, "documentNo": r.DocumentNo,
			"partnerId": r.PartnerID, "partnerName": r.PartnerName, "businessDate": r.BusinessDate, "status": r.Status,
			"productId": r.ProductID, "productCode": r.ProductCode, "productName": r.ProductName,
			"productModel": r.ProductModel, "productSpecification": r.ProductSpecification,
			"unit": r.Unit, "quantity": r.Quantity, "unitPrice": r.UnitPrice, "amount": r.Amount,
			"returnedQuantity": r.ReturnedQuantity, "returnedAmount": r.ReturnedAmount,
			"netQuantity": r.NetQuantity, "netAmount": r.NetAmount,
		}
		if r.MatchesHistory {
			row["matchesHistory"] = true
		}
		records = append(records, row)
	}
	amounts, quantities := purchaseAmounts(result.Summary)
	return map[string]any{
		"timeZone":  toolTimeZone,
		"timeBasis": "采购业务日期，起止日均含",
		"queriedAt": nowISO(),
		"semantics": "每行 itemCount 统计的是明细行数，不是单据张数；数量按商品与计量单位分别表达，不混加不同商品。" +
			"原始数量金额、截至当前仍有效的采购退货与净数量／净额只对已过账行成立，不适用时返回 null 而不是 0。" +
			"summary 覆盖完整筛选范围并按状态分别汇总，不是当前页。",
		"filtersApplied": map[string]any{
			"dateFrom": from, "dateTo": to, "status": status, "partnerId": in.PartnerID, "partnerKeyword": in.PartnerKeyword,
			"productId": in.ProductID, "productKeyword": in.ProductKeyword, "documentNo": in.DocumentNo,
			"directDelivery": in.DirectDelivery, "sortBy": in.SortBy, "sortDir": in.SortDir,
		},
		"page": page, "pageSize": pageSize, "total": result.Total, "hasMore": int64(page*pageSize) < result.Total,
		"records": records,
		"summary": map[string]any{"amounts": amounts, "quantities": quantities},
	}, len(records), nil
}

func (s Services) purchaseDocument(ctx context.Context, raw json.RawMessage) (map[string]any, int, error) {
	var in purchaseDocumentInput
	if _, err := decode(raw, &in); err != nil {
		return nil, 0, err
	}
	if in.PurchaseID < 1 || in.PurchaseID > 9007199254740991 {
		return nil, 0, errInvalidArguments
	}
	d, e := s.Purchase.Detail(ctx, in.PurchaseID)
	if e != nil {
		return nil, 0, e
	}
	items := []map[string]any{}
	for _, line := range d.Items {
		items = append(items, map[string]any{
			"id": line.ID, "productId": line.ProductID, "productCode": line.ProductCode, "productName": line.ProductName,
			"productModel": line.ProductModel, "productSpecification": line.ProductSpecification, "productType": line.ProductType,
			"unit": line.Unit, "quantity": line.Quantity, "unitPrice": line.UnitPrice, "amount": line.Amount,
			"remark": line.Remark,
		})
	}
	return map[string]any{
		"timeZone": toolTimeZone,
		"semantics": "商品编码、名称、型号、规格与单位是过账时的明细快照，含当时的空值，不因当前档案编辑而改变；" +
			"supplierName 来自当前供应商档案，不是历史快照。unitPrice 与 amount 是实际成交价，与商品参考价无关。",
		"document": map[string]any{
			"id": d.ID, "documentNo": d.DocumentNo, "partnerId": d.PartnerID, "partnerName": d.PartnerName,
			"businessDate": d.BusinessDate, "status": d.Status, "version": d.Version, "directDelivery": d.DirectDelivery,
			"postedBy": d.PostedBy, "postedByName": d.PostedByName, "postedAt": d.PostedAt,
			"createdBy": d.CreatedBy, "createdByName": d.CreatedByName, "createTime": d.CreateTime,
			"cancelledBy": d.CancelledBy, "cancelledByName": d.CancelledByName, "cancelReason": d.CancelReason, "cancelledAt": d.CancelledAt,
			"remark": d.Remark, "totalAmount": d.TotalAmount,
		},
		"items":          items,
		"filtersApplied": map[string]any{"purchaseId": in.PurchaseID},
	}, len(items), nil
}
