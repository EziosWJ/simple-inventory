import { useCallback } from "react";
import { Link } from "react-router-dom";
import { purchaseItemPage, getPurchase, type PurchaseSourceInput, type PurchaseItemRecord } from "@/api/business";
import { PagedRecordSelect } from "@/components/common/paged-record-select";
import { Field } from "@/components/common/field";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { sourceQuantityMilli, purchaseSourcesInvalid } from "./purchase-source-validation";
export function PurchaseSourcesEditor({ label, productId, quantity, businessDate, value, disabled = false, onChange }: {
  label: string; productId: number; quantity: string; businessDate: string; value: PurchaseSourceInput[]; disabled?: boolean; onChange: (sources: PurchaseSourceInput[]) => void;
}) {
  const loadPage = useCallback((keyword: string, page: number, pageSize: number) => purchaseItemPage({ documentNo: keyword.trim(), productId, status: "POSTED", page, pageSize }), [productId]);
  const loadRecord = useCallback(async (id: number): Promise<PurchaseItemRecord> => {
    const known = value.find(s => s.purchaseItemId === id);
    if (!known?.purchaseId) throw new Error("请重新选择采购明细");
    const document = await getPurchase(known.purchaseId), line = document.items.find(l => l.id === id);
    if (!line) throw new Error("采购明细不存在，请核对来源");
    return { ...line, id, purchaseId: document.id, documentNo: document.documentNo, partnerId: document.partnerId, partnerName: document.partnerName, businessDate: document.businessDate, status: document.status };
  }, [value]);
  const cap = sourceQuantityMilli(quantity), used = value.reduce((n, s) => n + (sourceQuantityMilli(s.quantity) ?? 0n), 0n);
  const remaining = cap !== null && used <= cap ? `${(cap - used) / 1000n}.${String((cap - used) % 1000n).padStart(3, "0")}`.replace(/\.?0+$/, "") : null;
  function update(index: number, patch: Partial<PurchaseSourceInput>) { onChange(value.map((s, i) => i === index ? { ...s, ...patch } : s)); }
  return <section aria-label={label} className="space-y-space-2 border-t border-border pt-space-3">
    <p className="text-sm font-medium">采购来源（选填）</p>
    <p className="text-sm text-text-tertiary">人工追溯，不占用采购数量。未记录数量：{remaining ?? "待核对"}。</p>
    {value.map((source, index) => <div key={index} className="grid gap-space-2 md:grid-cols-[2fr_1fr_auto]">
      <Field label={`采购来源 ${index + 1}`}>
        <PagedRecordSelect label={`${label}来源 ${index + 1}`} value={source.purchaseItemId} disabled={disabled} fallback={source.documentNo} loadPage={loadPage} loadRecord={loadRecord}
          describe={item => `${item.documentNo} · ${item.partnerName} · ${item.productName} · ${item.businessDate} · 明细 #${item.id}`}
          warning={item => item.status !== "POSTED" ? "来源采购已取消，请核对；可保留原记录或重新选择。" : ""}
          onChange={item => item ? update(index, { purchaseItemId: item.id, purchaseId: item.purchaseId, documentNo: item.documentNo, partnerName: item.partnerName, businessDate: item.businessDate, status: item.status, confirmLaterDate: false }) : update(index, { purchaseItemId: 0, purchaseId: undefined, documentNo: undefined, businessDate: undefined, status: undefined, confirmLaterDate: false })} />
      </Field>
      <Field label="来源数量"><Input aria-label={`${label}来源 ${index + 1}数量`} inputMode="decimal" disabled={disabled} value={source.quantity} onChange={e => update(index, { quantity: e.target.value })} /></Field>
      <Button disabled={disabled} onClick={() => onChange(value.filter((_, i) => i !== index))}>移除来源</Button>
      {source.businessDate && source.businessDate > businessDate && <label className="flex items-center gap-space-2 text-warning md:col-span-3"><input type="checkbox" disabled={disabled} checked={source.confirmLaterDate} onChange={e => update(index, { confirmLaterDate: e.target.checked })} />采购日期晚于销售日期，已核对并确认关联</label>}
      {source.status === "CANCELLED" && <p role="alert" className="text-warning md:col-span-3">来源采购已取消，请核对。原关联保留，修改来源数量需重新选择有效采购。</p>}
      {source.returnedQuantity && source.returnedQuantity !== "0" && <p className="text-warning md:col-span-3">来源采购明细已有退货 {source.returnedQuantity}，请核对实际出处。</p>}
      {source.purchaseId && <Link className="text-primary hover:underline md:col-span-3" to={`/business/purchases?purchaseId=${source.purchaseId}`}>查看来源采购单</Link>}
    </div>)}
    {value.length > 0 && purchaseSourcesInvalid(value, quantity, businessDate) && <p role="alert" className="text-error">请核对来源选择、数量合计或采购日期确认。</p>}
    <Button disabled={disabled || !productId || value.length >= 200} onClick={() => onChange([...value, { purchaseItemId: 0, quantity: remaining && remaining !== "0" ? remaining : "1", confirmLaterDate: false }])}>添加采购来源</Button>
  </section>;
}
