import type { DirectTrace } from "@/api/business";
import { DocumentSummary } from "@/components/business/document-presentation";
import { DirectDeliveryTrace } from "@/components/business/direct-delivery-trace";
import { DataTable } from "@/components/common/data-table";
import { FormDialog } from "@/components/common/form-dialog";
import { Button } from "@/components/ui/button";

export type ReturnPostingData = {
  documentNo: string;
  sourceNo: string;
  businessDate: string;
  partnerName: string;
  totalAmount: string;
  directTrace?: DirectTrace | null;
  items: {
    productId: number;
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
  }[];
};

export function ReturnPostingConfirmation({ open, document, direction, loading, error, onCancel, onConfirm, onOpenDetail, onOpenSource }: {
  open: boolean;
  document: ReturnPostingData | null;
  direction: "PURCHASE" | "SALE";
  loading: boolean;
  error?: string;
  onCancel: () => void;
  onConfirm: () => void;
  onOpenDetail: () => void;
  onOpenSource: () => void;
}) {
  if (!document) return null;
  const purchase = direction === "PURCHASE";
  const directWarning = purchase && document.directTrace?.purchase.status === "POSTED" && !document.directTrace.saleReturns.some(item => item.status === "POSTED");
  const inventoryLines = aggregateInventory(document.items).map(item => `${item.label}：${purchase ? "减少" : "增加"} ${item.quantity} ${item.unit}`);
  return <FormDialog
    open={open}
    title={`确认过账 · ${document.documentNo}`}
    description={`${document.partnerName} · 业务日期 ${document.businessDate} · 原单 ${document.sourceNo} · 过账后退货内容锁定`}
    documentLayout
    loading={loading}
    loadingText="正在过账…"
    submitText="确认过账"
    submitDisabled={Boolean(error)}
    cancelText="返回核对"
    closeOnEscape={!loading}
    closeOnOverlayClick={!loading}
    onCancel={onCancel}
    onSubmit={onConfirm}
  >
    <div className="space-y-space-5">
      {error && <p role="alert" className="rounded-control border border-error/30 bg-error/5 p-space-3 text-sm text-error">{error}</p>}
      <DocumentSummary totals={[
        { label: "本次退货金额", value: `¥${document.totalAmount}` },
      ]} hint={`共 ${document.items.length} 行；过账时重新核对原单退货额度。`}>
        <p><strong>库存影响：</strong>确认后按商品和单位{purchase ? "减少" : "增加"}库存。</p>
        <p><strong>往来影响：</strong>{/^0(?:\.0+)?$/.test(document.totalAmount) ? "零金额，往来余额无变化。" : purchase ? `供应商方向往来余额减少 ¥${document.totalAmount}，超出欠款部分形成应收供应商退款。` : `客户方向往来余额减少 ¥${document.totalAmount}，超出欠款部分形成应退客户款。`}</p>
        <p>退货不会自动退款，实际退款须另行办理；也不会逐单结清原单。</p>
        {directWarning && <p role="alert" className="font-medium text-error">直送业务须先过账关联销售退货，再过账采购退货。</p>}
        {error && <p>若请求结果不确定，请先重新查询单据状态核实，不要重复提交。</p>}
        {error && <div className="flex flex-wrap gap-2"><Button variant="secondary" onClick={onOpenDetail}>核对退货单详情 · {document.documentNo}</Button><Button variant="secondary" onClick={onOpenSource}>查看原单 · {document.sourceNo}</Button></div>}
      </DocumentSummary>
      <section aria-label="库存影响" className="space-y-space-2"><h3 className="font-semibold">确认后库存影响</h3><ul className="list-inside list-disc space-y-space-1 text-sm">{inventoryLines.map(line => <li key={line}>{line}</li>)}</ul></section>
      <section aria-label="即将过账的退货明细" className="space-y-space-3">
        <h3 className="text-base font-semibold">本次退货明细</h3>
        <DataTable columns={[
          { title: "商品", key: "product", width: "30%", render: (_, item) => <div className="min-w-[11rem]"><p className="font-medium">{item.productName}</p><p className="break-words text-xs text-text-secondary">{[item.productCode, item.productModel, item.productSpecification].filter(Boolean).join(" · ")}</p></div> },
          { title: "原单数量", dataIndex: "originalQuantity", align: "right", render: (value, item) => `${value} ${item.unit}` },
          { title: "当前有效已退", dataIndex: "returnedQuantity", align: "right", render: (value, item) => `${value} ${item.unit}` },
          { title: "当前剩余可退", dataIndex: "remainingQuantity", align: "right", render: (value, item) => `${value} ${item.unit}` },
          { title: "本次数量", dataIndex: "quantity", align: "right", render: (value, item) => `${value} ${item.unit}` },
          { title: "原价", dataIndex: "unitPrice", align: "right", render: value => `¥${value}` },
          { title: "本次行金额", dataIndex: "amount", align: "right", render: value => `¥${value}` },
        ]} dataSource={document.items.map((item, index) => ({ ...item, rowKey: index }))} rowKey="rowKey" minWidth={1100} empty="暂无退货明细。" />
        <p className="text-right text-lg font-semibold tabular-nums">本次退货合计：¥{document.totalAmount}</p>
      </section>
      {document.directTrace && <DirectDeliveryTrace trace={document.directTrace}/>}
    </div>
  </FormDialog>;
}

function aggregateInventory(items: ReturnPostingData["items"]) {
  const totals = new Map<string, { label: string; unit: string; milli: bigint }>();
  for (const item of items) {
    const milli = parseScaled(item.quantity, 3);
    if (milli === null) continue;
    const key = `${item.productId}:${item.unit}`;
    const current = totals.get(key) ?? {
      label: [item.productCode, item.productName, item.productModel, item.productSpecification].filter(Boolean).join(" · "),
      unit: item.unit,
      milli: 0n,
    };
    current.milli += milli;
    totals.set(key, current);
  }
  return [...totals.values()].map(item => ({ ...item, quantity: formatScaled(item.milli) }));
}

function parseScaled(value: string, scale: number): bigint | null {
  if (!/^\d+(?:\.\d+)?$/.test(value)) return null;
  const [whole, fraction = ""] = value.split(".");
  if (fraction.length > scale) return null;
  return BigInt(whole) * BigInt(10 ** scale) + BigInt((fraction + "0".repeat(scale)).slice(0, scale) || "0");
}

function formatScaled(value: bigint) {
  const whole = value / 1000n;
  const fraction = (value % 1000n).toString().padStart(3, "0").replace(/0+$/, "");
  return `${whole}${fraction ? `.${fraction}` : ""}`;
}
