import { useNavigate } from "react-router-dom";
import type { DirectTrace, DirectTraceDocument } from "@/api/business";
import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/common/data-table";

const statusText = (status: string) => ({ DRAFT: "草稿", POSTED: "已过账", CANCELLED: "已取消" }[status] ?? status);
const kindText = { PURCHASE: "原采购", SALE: "原销售", PURCHASE_RETURN: "采购退货", SALE_RETURN: "销售退货" };
const href = (d: DirectTraceDocument) => ({
  PURCHASE: `/business/purchases?purchaseId=${d.id}`,
  SALE: `/business/sales?saleId=${d.id}`,
  PURCHASE_RETURN: `/business/purchase-returns?purchaseReturnId=${d.id}`,
  SALE_RETURN: `/business/sale-returns?saleReturnId=${d.id}`,
}[d.kind]);

export function DirectDeliveryTrace({ trace }: { trace?: DirectTrace | null }) {
  const navigate = useNavigate();
  if (!trace) return null;
  const currentSales = trace.sales.filter(d => d.status === "POSTED");
  const hasSalesReturn = trace.saleReturns.some(d => d.status === "POSTED");
  const documents = [trace.purchase, ...trace.sales, ...trace.saleReturns, ...trace.purchaseReturns];
  return <section className="mt-space-4 space-y-space-3 border-t border-border pt-space-4" aria-label="直送业务与退货追溯">
    <h3 className="font-medium">直送业务与退货追溯</h3>
    <p className="text-sm text-text-secondary">客户直接退给供应商时，先单独过账销售退货增加库存，再单独过账采购退货扣减库存。两边按各自原明细和原价办理，可分次部分退货；客户应收和供应商应付分别抵减。每侧失败会保留另一侧已完成状态。</p>
    <div className="flex flex-wrap gap-2">
      {currentSales.map(d => <Button key={d.id} variant="secondary" onClick={() => navigate(`/business/sale-returns?saleId=${d.id}`)}>第一步：销售退货 · {d.documentNo}</Button>)}
      {hasSalesReturn && trace.purchase.status === "POSTED" && <Button variant="secondary" onClick={() => navigate(`/business/purchase-returns?purchaseId=${trace.purchase.id}`)}>第二步：采购退货</Button>}
    </div>
    <p className="text-sm text-text-tertiary">销售退货 {trace.saleReturns.filter(d => d.status === "POSTED").length} 张已过账；采购退货 {trace.purchaseReturns.filter(d => d.status === "POSTED").length} 张已过账。草稿与取消记录保留；退款须另行办理。</p>
    {documents.map(d => <div key={`${d.kind}-${d.id}`} className="space-y-2 border-t border-border pt-space-3">
      <div className="flex flex-wrap items-center justify-between gap-2"><Button variant="secondary" size="sm" onClick={() => navigate(href(d))}>{kindText[d.kind]} · {d.documentNo}</Button><span className="text-sm">{statusText(d.status)} · {d.partnerName} · {d.businessDate} · 金额 ¥{d.totalAmount}{d.status === "DRAFT" && "（草稿预览）"}</span></div>
      <DataTable columns={[
        { title: "商品 / 原明细", key: "product", render: (_, i) => `${i.productCode} ${i.productName}${i.originalItemId ? ` · 原明细 ${i.originalItemId}` : ""}` },
        { title: "数量", key: "quantity", render: (_, i) => `${i.quantity} ${i.unit}` },
        { title: "单价", dataIndex: "unitPrice", align: "right" },
        { title: "金额", dataIndex: "amount", align: "right" },
      ]} dataSource={d.items} rowKey="id" minWidth={540} empty="暂无明细" />
      <p className="text-sm text-text-tertiary">创建：{d.createdByName} · {new Date(d.createTime).toLocaleString()}{d.postedAt && `；过账：${d.postedByName} · ${new Date(d.postedAt).toLocaleString()}`}{d.cancelledAt && `；取消：${d.cancelledByName} · ${new Date(d.cancelledAt).toLocaleString()} · ${d.cancelReason ?? ""}`}</p>
    </div>)}
  </section>;
}
