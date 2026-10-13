import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { postDirectOrder, cancelDirectOrder, getDirectOrder, type DirectOrder } from "@/api/direct-orders";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { DocumentSummary } from "@/components/business/document-presentation";
import { DataTable } from "@/components/common/data-table";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { useAuthStore } from "@/store/auth-store";
import { ApiError } from "@/lib/api-error";
type DirectOrderLine = DirectOrder["items"][number] & { id: number; productCode: string; productName: string; purchaseAmount: string; saleAmount: string };
type Attempt = { action: "POST" | "CANCEL"; id: number; version: number; reason: string };
export function DirectOrderActions({ document, disabled = false, onChange }: { document: DirectOrder; disabled?: boolean; onChange: (d: DirectOrder) => void }) {
  const user = useAuthStore(s => s.user);
  const key = `simple-inventory:direct-action:${user?.id ?? "unknown"}:${document.id}`;
  const [confirmation, setConfirmation] = useState<"POST" | "CANCEL" | null>(null);
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState<Attempt | null>(null);
  const [retryReady, setRetryReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [error, setError] = useState("");
  const { feedback, notify } = useBusinessFeedback();
  useEffect(() => {
    setPending(null); setRetryReady(false);
    try { const raw = localStorage.getItem(key); if (raw) { const value = JSON.parse(raw); if (value.id === document.id && ["POST", "CANCEL"].includes(value.action) && Number.isSafeInteger(value.version) && value.version > 0 && typeof value.reason === "string") setPending(value); else setError("恢复标识无效，请核对直送单状态。"); } }
    catch { setError("恢复标识无法读取，请核对直送单状态。"); }
  }, [key, document.id]);
  function clear() { localStorage.removeItem(key); setPending(null); setRetryReady(false); }
  async function perform(retry = false) {
    if (busyRef.current || (disabled && !retry)) return;
    const attempt = retry ? pending : confirmation ? { action: confirmation, id: document.id, version: document.version, reason: reason.trim() } : null;
    if (!attempt) return;
    try { localStorage.setItem(key, JSON.stringify(attempt)); } catch { setError("无法保存操作恢复标识，请检查浏览器存储。"); return; }
    busyRef.current = true; setBusy(true); setPending(attempt); setError("");
    try {
      const d = attempt.action === "POST" ? await postDirectOrder(attempt.id, attempt.version) : await cancelDirectOrder(attempt.id, attempt.version, attempt.reason);
      clear(); setConfirmation(null); onChange(d);
      notify({ type: "success", title: attempt.action === "POST" ? "直送已确认" : "直送已整笔取消", description: attempt.action === "POST" ? "采购与销售同时过账，库存净变化为零。" : "原单及流水保留，真实收付款保留；可按实际情况独立新建。", documentNo: d.documentNo });
    } catch (e) {
      if (e instanceof ApiError && e.status && e.status < 500) { clear(); setConfirmation(null); }
      else setConfirmation(null);
      setError(e instanceof Error ? e.message : "操作结果待核实");
    } finally { busyRef.current = false; setBusy(false); }
  }
  async function verify() {
    if (!pending || busyRef.current) return; busyRef.current = true; setBusy(true);
    try {
      const d = await getDirectOrder(pending.id); onChange(d);
      if ((pending.action === "POST" && d.status === "POSTED") || (pending.action === "CANCEL" && d.status === "CANCELLED")) {
        clear(); setError(""); notify({ type: "success", title: "已核实直送操作结果", description: d.status === "POSTED" ? "采购、销售已同时生效，无需重复确认。" : "原直送已取消，无需重复冲销。", documentNo: d.documentNo });
      } else if (d.version !== pending.version || d.status === "CANCELLED") { clear(); setError("单据状态或版本已变化，请核对当前单据后再操作。"); }
      else { setRetryReady(true); setError("尚未确认提交结果，可按原版本重试，系统不会重复记账。"); }
    } catch (e) { setError(e instanceof Error ? e.message : "核实失败，请保留恢复标识"); }
    finally { busyRef.current = false; setBusy(false); }
  }
  const hasReturns = document.activeSaleReturns.length > 0 || document.activePurchaseReturns.length > 0;
  const lines = document.items as DirectOrderLine[];
  return <div className="space-y-space-3">
    <BusinessFeedback feedback={feedback} />
    {error && <p role="alert" className="text-error">{error}</p>}
    {pending && <div className="space-y-space-2 rounded-control border border-border p-space-3"><p>直送操作结果待核实，保留原版本，不另建单据。</p><Button disabled={busy} onClick={() => void verify()}>核实直送操作结果</Button> {retryReady && <Button disabled={busy} onClick={() => void perform(true)}>按原版本重试</Button>}</div>}
    <div className="flex flex-wrap gap-space-2">
      {document.status === "DRAFT" && <Button variant="primary" disabled={disabled || busy || Boolean(pending)} onClick={() => setConfirmation("POST")}>确认直送</Button>}
      {document.status !== "CANCELLED" && <Button variant="danger" disabled={disabled || busy || Boolean(pending) || hasReturns} onClick={() => { setReason(""); setConfirmation("CANCEL"); }}>整笔取消直送</Button>}
      {document.purchaseId && <Link className="text-primary hover:underline" to={`/business/purchases?purchaseId=${document.purchaseId}`}>查看采购单</Link>}
      {document.saleId && <><Link className="text-primary hover:underline" to={`/business/sales?saleId=${document.saleId}`}>查看销售单</Link><Link className="text-primary hover:underline" to={`/business/sales/${document.saleId}/delivery-note`}>打印送货单</Link></>}
    </div>
    {document.status === "POSTED" && <div className="flex flex-wrap gap-space-3"><Link className="text-primary hover:underline" to={`/business/sale-returns?saleId=${document.saleId}`}>客户退回店内：销售退货</Link><Link className="text-primary hover:underline" to={`/business/purchase-returns?purchaseId=${document.purchaseId}`}>后续退供应商：采购退货</Link></div>}
    {hasReturns && <p role="alert" className="text-warning">存在生效退货，不能整笔取消。销售退货：{document.activeSaleReturns.join("、") || "无"}；采购退货：{document.activePurchaseReturns.join("、") || "无"}。</p>}
    <FormDialog documentLayout={confirmation === "POST"} open={confirmation !== null} title={confirmation === "POST" ? "确认客户已收货" : "整笔取消直送"} description={confirmation === "POST" ? `${document.documentNo} · ${document.supplierName} → ${document.customerName}` : undefined} cancelText={confirmation === "POST" ? "返回核对" : "取消"} submitText={confirmation === "POST" ? "确认并同时过账" : "确认整笔取消"} loadingText={confirmation === "POST" ? "正在同时过账采购与销售…" : "正在取消…"} submitDisabled={confirmation === "CANCEL" && !reason.trim()} loading={busy} onCancel={() => { if (!busy) setConfirmation(null); }} onSubmit={() => perform()}>
      {confirmation === "POST" ? <div className="space-y-space-5">
        {busy && <p role="status" className="rounded-control border border-border bg-neutral-background px-space-4 py-space-3 text-sm">正在按同一事务确认直送单 {document.documentNo}。</p>}
        <section aria-label="直送双方与业务日期" className="grid gap-space-3 text-sm sm:grid-cols-2"><p><span className="text-text-secondary">供应商：</span>{document.supplierName}</p><p><span className="text-text-secondary">客户：</span>{document.customerName}</p><p><span className="text-text-secondary">业务日期：</span>{document.businessDate}</p><p><span className="text-text-secondary">执行动作：</span>生成并同时过账采购单与销售单</p></section>
        <DocumentSummary totals={[{ label: "采购金额", value: `¥${document.purchaseTotal}` }, { label: "销售金额", value: `¥${document.saleTotal}` }]} hint={`${lines.length} 行 · ${lines.filter(item => item.productType === "GOODS").length} 行实物 · ${lines.filter(item => item.productType === "SERVICE").length} 行服务`}>
          <p>同一事务中，供应商方向往来余额增加采购金额 ¥{document.purchaseTotal}；客户方向往来余额增加销售金额 ¥{document.saleTotal}。零金额方向不会产生往来金额流水。</p>
        </DocumentSummary>
        <section className="space-y-space-3"><h3 className="text-base font-semibold">即将确认的直送明细</h3><DataTable columns={[
          { title: "商品 / 服务", key: "product", width: "30%", render: (_, item) => <div className="min-w-[12rem] space-y-space-1"><p className="font-medium">{item.productName}</p><p className="break-words text-xs text-text-secondary">{item.productCode}{item.productType === "SERVICE" ? " · 服务项目" : " · 实物商品"}</p>{item.remark?.trim() && <p className="whitespace-pre-wrap break-words text-sm text-text-secondary">备注：{item.remark.trim()}</p>}</div> },
          { title: "数量", key: "quantity", align: "right", nowrap: true, render: (_, item) => `${item.quantity} ${item.unit}` },
          { title: "采购单价 / 金额", key: "purchase", align: "right", render: (_, item) => item.productType === "SERVICE" ? "—（服务不计采购金额）" : <span className="tabular-nums">¥{item.purchasePrice}<span className="block text-sm">金额 ¥{item.purchaseAmount}</span></span> },
          { title: "销售单价 / 金额", key: "sale", align: "right", render: (_, item) => <span className="tabular-nums">¥{item.salePrice}<span className="block text-sm">金额 ¥{item.saleAmount}</span></span> },
        ]} dataSource={lines} rowKey="id" minWidth={820} /></section>
        <section aria-label="确认影响" className="space-y-space-2 border-t border-border pt-space-4"><h3 className="text-base font-semibold">确认后的业务影响</h3><p className="text-sm text-text-secondary">确认依据：客户已实际收到本次直送货物。采购与销售将同时生效；实物库存净变化为零，但保留采购增加、销售减少的成对库存流水。</p><ul className="list-inside list-disc space-y-space-1 text-sm">{lines.map(item => <li key={item.id}>{item.productType === "GOODS" ? `${item.productCode} · ${item.productName}：采购增加 ${item.quantity} ${item.unit}，销售减少 ${item.quantity} ${item.unit}，库存净变化为零。` : `${item.productCode} · ${item.productName}：服务项目，不产生采购库存影响。`}</li>)}</ul></section>
        <p className="text-sm text-text-tertiary">确认成功后两侧单据同时锁定；若请求结果暂不明确，可在当前页面核实原直送单，不要另建单据。</p>
      </div> : <>
        <p>{document.documentNo} · {document.supplierName} → {document.customerName}</p>
        <p>采购金额 ¥{document.purchaseTotal} / 销售金额 ¥{document.saleTotal}</p>
        <p>{document.status === "DRAFT" ? "取消草稿不改变库存或往来。" : "同时冲销采购、销售，保留原单据、流水及真实收付款；实际退款另行登记。"}</p>
        {confirmation === "CANCEL" && <Field label="取消原因" required><Textarea aria-label="直送取消原因" maxLength={500} value={reason} onChange={e => setReason(e.target.value)} /></Field>}
      </>}
    </FormDialog>
  </div>;
}
