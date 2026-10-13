import type { DirectTrace } from "@/api/business";
import { DocumentOperationRecords, DocumentSummary } from "@/components/business/document-presentation";
import { StatusTag } from "@/components/common/status-tag";
import { DirectDeliveryTrace } from "@/components/business/direct-delivery-trace";
import { DataTable } from "@/components/common/data-table";
import { Button } from "@/components/ui/button";

export type ReturnDetailData = {
  documentNo: string;
  sourceNo: string;
  sourceId: number;
  partnerId: number;
  partnerName: string;
  businessDate: string;
  status: "DRAFT" | "POSTED" | "CANCELLED";
  totalAmount: string;
  remark?: string | null;
  createdByName?: string;
  createTime?: string;
  postedByName?: string;
  postedAt?: string | null;
  cancelledByName?: string;
  cancelledAt?: string | null;
  cancelReason?: string | null;
  directTrace?: DirectTrace | null;
  items: {
    productCode: string;
    productName: string;
    productModel?: string | null;
    productSpecification?: string | null;
    unit: string;
    originalQuantity: string;
    returnedQuantity: string;
    remainingQuantity: string;
    quantity: string;
    unitPrice: string;
    amount: string;
    remark?: string | null;
  }[];
};

const statusLabel = { DRAFT: "草稿", POSTED: "已过账", CANCELLED: "已取消" };

export function ReturnDetail({ document, direction, onOpenSource }: {
  document: ReturnDetailData;
  direction: "PURCHASE" | "SALE";
  onOpenSource: () => void;
}) {
  const isPurchase = direction === "PURCHASE";
  const hasPriorSalesReturn = Boolean(document.directTrace?.saleReturns.some(item => item.status === "POSTED"));
  const directReturnWarning = isPurchase && document.directTrace?.purchase.status === "POSTED" && !hasPriorSalesReturn;
  return <div className="space-y-space-5">
    <div className="flex flex-wrap items-center gap-x-space-6 gap-y-space-2 text-sm"><span>{isPurchase ? "供应商" : "客户"}：{document.partnerName}</span><span>业务日期 {document.businessDate}</span><StatusTag tone={document.status === "DRAFT" ? "warning" : document.status === "POSTED" ? "success" : "neutral"}>{statusLabel[document.status]}</StatusTag></div>
    <DocumentSummary totals={[
      { label: document.status === "CANCELLED" ? "原退货金额" : "本次退货金额", value: `¥${document.totalAmount}` },
    ]} hint={`共 ${document.items.length} 项`}>
      <p>{document.status === "POSTED" ? `${isPurchase ? "库存已减少" : "库存已增加"}；${/^0(?:\.0+)?$/.test(document.totalAmount) ? "零金额，往来余额无变化。" : "退货金额已抵减往来余额，实际退款须另行办理。"}` : document.status === "DRAFT" ? "草稿尚未生效，不改变库存、往来余额或有效退货额度。" : document.postedAt ? "已冲销取消，原过账记录保留；下方展示原退货内容与金额。" : "草稿已取消，库存与往来余额未变化；原退货内容保留。"}</p>
      <div>原{isPurchase ? "采购" : "销售"}单：<Button variant="secondary" size="sm" onClick={onOpenSource}>{document.sourceNo}</Button></div>
      <div>本次数量按行展示；当前有效已退数量包含截至当前有效过账的全部退货。已过账本单已计入该累计值，不会重复加算。</div>
      {directReturnWarning && <p role="alert" className="font-medium text-error">直送业务须先过账关联销售退货，再过账采购退货。当前尚无已过账销售退货。</p>}
    </DocumentSummary>

    <section aria-label="退货商品明细" className="space-y-space-3">
      <div><h3 className="text-base font-semibold">商品明细</h3><p className="text-sm text-text-secondary">原成交价与本次行金额保留；各数量列按原单明细核对。</p></div>
      <DataTable columns={[
        { title: "商品快照", key: "product", width: "28%", render: (_, item) => <div className="min-w-[12rem] space-y-1"><p className="font-medium">{item.productName}</p><p className="break-words text-xs text-text-secondary">{[item.productCode, item.productModel, item.productSpecification].filter(Boolean).join(" · ")}</p>{item.remark?.trim() && <p className="whitespace-pre-wrap break-words text-sm text-text-secondary">明细备注：{item.remark.trim()}</p>}</div> },
        { title: "原单数量", dataIndex: "originalQuantity", align: "right", render: (value, item) => `${String(value)} ${item.unit}` },
        { title: "当前有效已退", dataIndex: "returnedQuantity", align: "right", render: (value, item) => `${String(value)} ${item.unit}` },
        { title: "当前剩余可退", dataIndex: "remainingQuantity", align: "right", render: (value, item) => `${String(value)} ${item.unit}` },
        { title: "本次退货数量", dataIndex: "quantity", align: "right", render: (value, item) => `${String(value)} ${item.unit}` },
        { title: "原成交单价", dataIndex: "unitPrice", align: "right", render: value => `¥${String(value)}` },
        { title: "本次行金额", dataIndex: "amount", align: "right", render: value => `¥${String(value)}` },
      ]} dataSource={document.items.map((item, index) => ({ ...item, rowKey: index }))} rowKey="rowKey" minWidth={1100} empty="暂无退货明细。" />
      <p className="text-right text-lg font-semibold tabular-nums">本次退货合计：¥{document.totalAmount}</p>
    </section>

    {document.cancelReason && <section aria-label="取消原因" className="rounded-control border border-error/30 bg-error/5 p-space-4"><h3 className="font-medium">取消原因</h3><p className="mt-1 whitespace-pre-wrap break-words text-sm">{document.cancelReason}</p></section>}
    {document.remark?.trim() && <section aria-label="单据备注" className="border-l-2 border-border pl-space-4"><h3 className="font-medium">单据备注</h3><p className="mt-space-2 whitespace-pre-wrap break-words text-sm text-text-secondary">{document.remark.trim()}</p></section>}
    {document.directTrace && <DirectDeliveryTrace trace={document.directTrace}/>}
    <DocumentOperationRecords document={document} />
  </div>;
}
