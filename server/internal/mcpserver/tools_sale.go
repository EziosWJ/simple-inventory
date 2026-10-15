package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/EziosWJ/simple-inventory/server/internal/sale"
)

type saleItemsInput struct {
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

type saleDocumentInput struct {
	SaleID int64 `json:"saleId"`
}

func (s Services) saleItems(ctx context.Context, raw json.RawMessage, maxPage int) (map[string]any, int, error) {
	var in saleItemsInput
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
	result, e := s.Sale.ItemPage(ctx, sale.ItemQuery{
		Query: sale.Query{
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
			"id": r.ID, "saleId": r.SaleID, "documentNo": r.DocumentNo,
			"partnerId": r.PartnerID, "partnerName": r.PartnerName, "businessDate": r.BusinessDate, "status": r.Status,
			"productId": r.ProductID, "productCode": r.ProductCode, "productName": r.ProductName, "productType": r.ProductType,
			"productModel": r.ProductModel, "productSpecification": r.ProductSpecification,
			"unit": r.Unit, "quantity": r.Quantity, "unitPrice": r.UnitPrice, "amount": r.Amount,
			"returnsApplicable": r.ReturnsApplicable, "netApplicable": r.NetApplicable,
			"returnedQuantity": r.ReturnedQuantity, "returnedAmount": r.ReturnedAmount,
			"netQuantity": r.NetQuantity, "netAmount": r.NetAmount,
		}
		if r.MatchesHistory {
			row["matchesHistory"] = true
		}
		records = append(records, row)
	}
	amounts := []map[string]any{}
	for _, a := range result.Summary.Amounts {
		amounts = append(amounts, map[string]any{"status": a.Status, "itemCount": a.ItemCount, "originalAmount": a.OriginalAmount, "returnedAmount": a.ReturnedAmount, "netAmount": a.NetAmount})
	}
	quantities := []map[string]any{}
	for _, q := range result.Summary.Quantities {
		quantities = append(quantities, map[string]any{"status": q.Status, "productId": q.ProductID, "productCode": q.ProductCode, "productName": q.ProductName, "productType": q.ProductType, "unit": q.Unit, "returnsApplicable": q.ReturnsApplicable, "quantity": q.Quantity, "returnedQuantity": q.ReturnedQuantity, "netQuantity": q.NetQuantity})
	}
	return map[string]any{
		"timeZone":  toolTimeZone,
		"timeBasis": "原销售业务日期，起止日均含",
		"queriedAt": nowISO(),
		"semantics": "每行 itemCount 统计的是明细行数，不是单据张数；不同商品或计量单位的数量分别表达，不混加。" +
			"netQuantity／netAmount 是原数量金额减去截至当前仍有效（已过账且未取消）的销售退货，9月销售10件在10月有效退回2件时查9月得到原始10、有效退货2、净8；不是收款、退款现金流或利润。" +
			"服务行不可退货（returnsApplicable=false）；草稿与取消行净额不适用，返回 null 而不是 0。summary 覆盖完整筛选范围。",
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

func (s Services) saleDocument(ctx context.Context, raw json.RawMessage) (map[string]any, int, error) {
	var in saleDocumentInput
	if _, err := decode(raw, &in); err != nil {
		return nil, 0, err
	}
	if in.SaleID < 1 || in.SaleID > 9007199254740991 {
		return nil, 0, errInvalidArguments
	}
	d, e := s.Sale.Detail(ctx, in.SaleID)
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
	// A posted or cancelled sale carries the frozen customer name, delivery
	// details and printer header. A draft has frozen nothing yet: the customer
	// name and delivery details fall back to the current archive, while the
	// printer header has no fallback at all and must not be reported as an
	// empty snapshot.
	frozen := d.Status != "DRAFT"
	owner := func(value string) any {
		if !frozen {
			return nil
		}
		return value
	}
	return map[string]any{
		"timeZone": toolTimeZone,
		"semantics": "已过账或已取消单据的 customerName、送货资料与 owner* 是过账时冻结的快照，含当时的空值；snapshotAvailable=false 表示草稿尚未冻结，" +
			"此时 customerName 与送货资料回退当前档案，owner* 因为没有回退来源而返回 null（不是「当时为空」）。" +
			"商品编码、名称、型号、规格与单位是明细快照。当前商品参考价与往来银行资料不会写入本响应。",
		"document": map[string]any{
			"id": d.ID, "documentNo": d.DocumentNo, "partnerId": d.PartnerID, "partnerName": d.PartnerName,
			"snapshotAvailable": frozen,
			"deliveryContact":   d.DeliveryContact, "deliveryPhone": d.DeliveryPhone, "deliveryAddress": d.DeliveryAddress,
			"ownerName": owner(d.OwnerName), "ownerPhone": owner(d.OwnerPhone), "ownerAddress": owner(d.OwnerAddress),
			"businessDate": d.BusinessDate, "status": d.Status, "version": d.Version,
			"directDelivery": d.DirectDelivery, "directPurchaseId": d.DirectPurchaseID,
			"postedBy": d.PostedBy, "postedByName": d.PostedByName, "postedAt": d.PostedAt,
			"createdBy": d.CreatedBy, "createdByName": d.CreatedByName, "createTime": d.CreateTime,
			"cancelledBy": d.CancelledBy, "cancelledByName": d.CancelledByName, "cancelReason": d.CancelReason, "cancelledAt": d.CancelledAt,
			"remark": d.Remark, "totalAmount": d.TotalAmount,
		},
		"items":          items,
		"filtersApplied": map[string]any{"saleId": in.SaleID},
	}, len(items), nil
}
