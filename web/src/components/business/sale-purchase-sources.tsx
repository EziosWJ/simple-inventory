import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getSale, updateSaleSources, saleSourceHistory, type SaleDraft, type PurchaseSourceInput, type PurchaseSourceHistory } from "@/api/business";
import { PurchaseSourcesEditor } from "./purchase-sources-editor";
import { purchaseSourcesInvalid, sourceQuantityMilli } from "./purchase-source-validation";
import { FormDialog } from "@/components/common/form-dialog";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/datetime";
function historyDescription(value: string) { try { const rows: {purchaseItemId:number;quantity:string}[] = JSON.parse(value); return rows.length ? rows.map(s => `采购明细 #${s.purchaseItemId}，数量 ${s.quantity}`).join("；") : "未记录来源"; } catch { return "历史记录暂无法显示"; } }
function remainingQuantity(quantity: string, sources: PurchaseSourceInput[]) { const cap = sourceQuantityMilli(quantity), used = sources.reduce((n,s) => n+(sourceQuantityMilli(s.quantity) ?? 0n),0n); if (cap === null || used > cap) return "待核对"; const rest = cap-used; return `${rest/1000n}.${String(rest%1000n).padStart(3,"0")}`.replace(/\.?0+$/, ""); }
export function SalePurchaseSources({ document, onChange }: { document: SaleDraft; onChange: (d: SaleDraft) => void }) {
  const [open, setOpen] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState("");
  const [sources, setSources] = useState<Record<number, PurchaseSourceInput[]>>({});
  const [history, setHistory] = useState<PurchaseSourceHistory[]>([]);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [notice, setNotice] = useState("");
  useEffect(() => { setOpen(false); setHistoryOpen(false); setError(""); setNotice(""); }, [document.id]);
  if (document.directDelivery) return null;
  const goods = document.items.filter(l => l.productType === "GOODS");
  async function save() {
    setBusy(true); setError("");
    try { const d = await updateSaleSources(document.id, { version: document.version, items: goods.map(l => ({ saleItemId: l.id, sources: sources[l.id] ?? [] })) }); onChange(d); setOpen(false); setNotice("采购来源已保存，库存和往来未变化。"); }
    catch (e) { setError(e instanceof Error ? e.message : "来源保存结果待核实，请重新读取后核对"); }
    finally { setBusy(false); }
  }
  async function inspect() {
    setBusy(true); setError("");
    try { onChange(await getSale(document.id)); setOpen(false); setNotice("已读取当前保存的采购来源，请核对操作结果。"); }
    catch (e) { setError(e instanceof Error ? e.message : "读取失败"); } finally { setBusy(false); }
  }
  return <section aria-label="销售采购来源" className="mt-space-4 space-y-space-3 border-t border-border pt-space-4">
    <h3 className="font-medium">销售采购来源</h3>
    <p className="text-sm text-text-tertiary">人工填写的出处参考，不占用采购数量；来源修改不改变库存或往来。</p>
    {notice && <p role="status">{notice}</p>}
    {error && <p role="alert" className="text-error">{error} <Button disabled={busy} onClick={() => void inspect()}>重新读取并核对来源</Button></p>}
    {goods.map(line => <div key={line.id} className="space-y-space-1"><p>{line.productName} · 销售 {line.quantity} {line.unit} · 未记录来源 {remainingQuantity(line.quantity,line.purchaseSources ?? [])} {line.unit}</p>{(line.purchaseSources ?? []).length === 0 ? <p className="text-text-tertiary">来源未记录</p> : (line.purchaseSources ?? []).map(s => <p key={s.purchaseItemId}><Link className="text-primary hover:underline" to={`/business/purchases?purchaseId=${s.purchaseId}`}>{s.documentNo}</Link> · {s.partnerName} · 来源数量 {s.quantity} {line.unit} {s.status === "CANCELLED" && <span className="text-warning">来源采购已取消，请核对</span>} {s.returnedQuantity && s.returnedQuantity !== "0" && <span className="text-warning">采购明细已退 {s.returnedQuantity}</span>}</p>)}</div>)}
    {goods.length > 0 && document.status !== "CANCELLED" && <Button disabled={busy} onClick={() => { setSources(Object.fromEntries(goods.map(l => [l.id, l.purchaseSources ?? []]))); setOpen(true); setError(""); }}>补填或更正采购来源</Button>}
    <Button disabled={busy} onClick={() => { setBusy(true); void saleSourceHistory(document.id).then(rows => { setHistory(rows); setHistoryOpen(true); }).catch(e => setError(e instanceof Error ? e.message : "历史读取失败")).finally(() => setBusy(false)); }}>查看来源修改历史</Button>
    {historyOpen && <div className="space-y-space-2">{history.length === 0 ? <p>暂无来源修改记录</p> : history.map(h => <div key={h.id}><p>{h.operatorName} · {formatDateTime(h.createTime)} · 销售明细 #{h.saleItemId}</p><p className="break-all text-sm">修改前：{historyDescription(h.beforeData)}</p><p className="break-all text-sm">修改后：{historyDescription(h.afterData)}</p></div>)}</div>}
    <FormDialog open={open} title="补填或更正采购来源" submitText="保存采购来源" loading={busy} submitDisabled={goods.some(l => purchaseSourcesInvalid(sources[l.id] ?? [], l.quantity, document.businessDate))} onCancel={() => { if (!busy) setOpen(false); }} onSubmit={save} contentClassName="max-w-modal-lg">
      {error && <p role="alert" className="text-error">{error}</p>}
      {goods.map(line => <div key={line.id}><p>{line.productName} · {line.quantity} {line.unit}</p><PurchaseSourcesEditor label={`销售明细 ${line.id}`} productId={line.productId} quantity={line.quantity} businessDate={document.businessDate} value={sources[line.id] ?? []} disabled={busy} onChange={v => setSources(current => ({ ...current, [line.id]: v }))} /></div>)}
    </FormDialog>
  </section>;
}
