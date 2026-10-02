import { businessReturnTo, withBusinessReturn } from "@/lib/business-navigation";
import { draftLineAmount as amount, draftMoney as money } from "@/components/business/draft-amount";
import { useCallback, useEffect, useRef, useState } from "react";
import { useBlocker, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { createPurchase, getPartner, getProduct, getPurchase, postPurchase, purchaseSaveResult, resolvePurchaseSave, updatePurchase, type PartnerRecord, type ProductRecord, type PurchaseDraft, type PurchaseInput, type PurchaseSaveResult } from "@/api/business";
import { PartnerSelect, ProductSelect } from "@/components/business/master-data-select";
import { DraftProductConfirmation } from "@/components/business/draft-product-confirmation";
import { PageHeader } from "@/components/common/page-header";
import { FormSection } from "@/components/common/form-section";
import { FormDialog } from "@/components/common/form-dialog";
import { Field } from "@/components/common/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api-error";
import { useAuthStore } from "@/store/auth-store";

type Line = { productId: number; productType: "GOODS"; unit: string; quantity: string; unitPrice: string; remark: string };
type Form = { partnerId: number; businessDate: string; directDelivery: boolean; remark: string; items: Line[] };
type Pending = { key: string; operation: "CREATE" | "EDIT"; id: number; version: number; postAfter?: boolean };
type PendingPost = { id: number; version: number };
const blankLine = (): Line => ({ productId: 0, productType: "GOODS", unit: "", quantity: "1", unitPrice: "0.00", remark: "" });
function initialForm(): Form {
  const now = new Date();
  const businessDate = new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
  return { partnerId: 0, businessDate, directDelivery: false, remark: "", items: [blankLine()] };
}
function fromDraft(d: PurchaseDraft): Form {
  return { partnerId: d.partnerId, businessDate: d.businessDate, directDelivery: d.directDelivery, remark: d.remark ?? "", items: d.items.map(i => ({ productId: i.productId, productType: i.productType, unit: i.unit, quantity: i.quantity, unitPrice: i.unitPrice, remark: i.remark ?? "" })) };
}
export function PurchaseFormPage() {
  const { id } = useParams();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const returnTo = businessReturnTo(params.get("returnTo"),"/business/purchases");
  const user = useAuthStore(s => s.user);
  const documentID = id ? Number(id) : 0;
  const storageKey = user ? `simple-inventory:purchase-save:${user.id}` : null;
  const postStorageKey = user ? `simple-inventory:purchase-post:${user.id}` : null;
  const [form, setForm] = useState<Form>(initialForm);
  const [baseline, setBaseline] = useState(() => JSON.stringify(form));
  const [document, setDocument] = useState<PurchaseDraft | null>(null);
  const [products, setProducts] = useState<ProductRecord[]>([]);
  const [partner, setPartner] = useState<PartnerRecord | null>(null);
  const [confirmation, setConfirmation] = useState<"SAVE_POST" | "RETRY_POST" | null>(null);
  const [postPending, setPostPending] = useState<PendingPost | null>(null);
  const [postRetryReady, setPostRetryReady] = useState(false);
  const [loading, setLoading] = useState(Boolean(documentID));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [conflict, setConflict] = useState(false);
  const [pending, setPending] = useState<Pending | null>(null);
  const [recoveryLoaded, setRecoveryLoaded] = useState(false);
  const pendingBody = useRef<PurchaseInput | null>(null);
  const savingRef = useRef(false);
  const loadGeneration = useRef(0);
  const savedRoute = useRef<number | null>(null);
  const invalidateLoad = useCallback(() => { ++loadGeneration.current; }, []);
  const dirty = JSON.stringify(form) !== baseline;
  const terminal = document !== null && document.status !== "DRAFT";
  const locked = loading || saving || Boolean(confirmation) || Boolean(pending) || Boolean(postPending) || terminal || Boolean(id && !document);
  const blocker = useBlocker(dirty || saving || Boolean(pending) || Boolean(postPending));

  const load = useCallback(async (target = documentID, clearMessage = false) => {
    const generation = ++loadGeneration.current;
    if (!Number.isSafeInteger(target) || target < 1) { setError("采购单 ID 无效"); setLoading(false); return; }
    setLoading(true);
    try {
      const d = await getPurchase(target);
      const selected = await Promise.all([...new Set(d.items.map(i => i.productId))].map(getProduct));
      const supplier = await getPartner(d.partnerId);
      if (generation !== loadGeneration.current) return;
      const next = fromDraft(d);
      setDocument(d); setProducts(selected); setPartner(supplier); setForm(next); setBaseline(JSON.stringify(next));
      setError(""); setConflict(false);
      if (clearMessage) setNotice("");
    } catch (e) { if (generation === loadGeneration.current) setError(e instanceof Error ? e.message : "采购草稿加载失败"); }
    finally { if (generation === loadGeneration.current) setLoading(false); }
  }, [documentID]);

  useEffect(() => {
    if (id && savedRoute.current === documentID) { savedRoute.current = null; setLoading(false); }
    else if (id) void load();
    else { const next = initialForm(); setForm(next); setBaseline(JSON.stringify(next)); setDocument(null); setProducts([]); setPartner(null); setConflict(false); setError(""); setNotice(""); }
    return invalidateLoad;
  }, [id, documentID, load, invalidateLoad]);
  useEffect(() => {
    if (!documentID && document && !dirty && !pending && !postPending && !saving && !conflict) navigate(withBusinessReturn(`/business/purchases/${document.id}/edit`, returnTo), { replace: true });
  }, [documentID, document, dirty, pending, postPending, saving, conflict, navigate, returnTo]);
  useEffect(() => {
    setPending(null); pendingBody.current = null; setRecoveryLoaded(false);
    if (!storageKey) return;
    try {
      const raw = localStorage.getItem(storageKey);
      if (raw) {
        const p: unknown = JSON.parse(raw);
        if (typeof p === "object" && p !== null && "key" in p && typeof p.key === "string" && "operation" in p && (p.operation === "CREATE" || p.operation === "EDIT") && "id" in p && typeof p.id === "number" && "version" in p && typeof p.version === "number") setPending(p as Pending);
      }
    } catch { setError("保存恢复标识无法读取，请保持当前页面并核实保存结果。"); }
    setRecoveryLoaded(true);
  }, [storageKey]);
  useEffect(() => {
    setPostPending(null); setPostRetryReady(false);
    if (!postStorageKey) return;
    try {
      const raw = localStorage.getItem(postStorageKey);
      if (raw) { const p = JSON.parse(raw); if (Number.isSafeInteger(p.id) && p.id > 0 && Number.isSafeInteger(p.version) && p.version > 0) setPostPending(p); }
    } catch { setError("过账恢复标识无法读取，请核实单据状态。"); }
  }, [postStorageKey]);
  useEffect(() => {
    if (!dirty && !pending && !postPending && !saving) return;
    const warn = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = ""; };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty, pending, postPending, saving]);

  function updateLine(index: number, patch: Partial<Line>) {
    setForm(current => ({ ...current, items: current.items.map((line, n) => n === index ? { ...line, ...patch } : line) }));
  }
  const unitPending = form.items.some(line => { const p = products.find(p => p.id === line.productId); return Boolean(p && (p.type !== line.productType || p.unit !== line.unit)); });
  const lineAmounts = form.items.map(line => amount(line.quantity, line.unitPrice));
  const total = lineAmounts.every(a => a !== null) ? lineAmounts.reduce<bigint>((sum, a) => sum + (a ?? 0n), 0n) : null;
  const invalid = !form.partnerId || !form.businessDate || !form.items.length || form.items.length > 200 || form.items.some(line => !line.productId || !line.unit) || total === null || total > 9223372036854775807n || unitPending;

  function clearPending() {
    if (storageKey) localStorage.removeItem(storageKey);
    setPending(null); pendingBody.current = null;
  }
  function acceptSaved(d: PurchaseDraft, originalVersion: number) {
    if (!documentID) savedRoute.current = d.id;
    clearPending(); setDocument(d);
    // Preserve the user's input after their save, unless another operation has
    // subsequently changed the document. Reload is explicit in that case.
    setBaseline(JSON.stringify(form)); setConflict(false);
    setNotice(`已保存 ${d.documentNo}，原保存版本 ${originalVersion}；当前版本 ${d.version}，状态 ${d.status === "DRAFT" ? "草稿（未生效）" : d.status === "POSTED" ? "已过账" : "已取消"}。`);
    if (d.version !== originalVersion) { setConflict(true); setError("原保存已成功，单据随后被修改。请重新加载最新内容后继续编辑。"); }
    else setError("");
  }
  function clearPost() {
    if (postStorageKey) localStorage.removeItem(postStorageKey);
    setPostPending(null); setPostRetryReady(false);
  }
  function showPostState(d: PurchaseDraft, expected: number, definiteFailure = false, failure = "") {
    setDocument(d);
    if (document?.id !== d.id) { const next = fromDraft(d); setForm(next); setBaseline(JSON.stringify(next)); }
    if (d.status === "POSTED") { clearPost(); setConflict(false); setError(""); setNotice(`已过账 ${d.documentNo}，当前版本 ${d.version}；库存与应付已生效。`); }
    else if (d.status === "CANCELLED" || d.version !== expected) { clearPost(); setNotice(""); setConflict(true); setError(`原过账已停止：${d.documentNo} 当前${d.status === "CANCELLED" ? "已取消" : `版本 ${d.version}，已被修改`}，请重新加载核对。`); }
    else if (definiteFailure) { clearPost(); setNotice(`已保存 ${d.documentNo}，仍为草稿，尚未过账。`); setError(`${failure}；已保存 ${d.documentNo}，仍为草稿，输入已保留，可修改后重新确认。`); }
    else { setNotice(`已保存 ${d.documentNo}，过账结果尚待核实。`); setPostRetryReady(true); setError(`${d.documentNo} 当前仍为草稿，原过账结果尚未确认；请再次核实，或重新核对后按同一 ID 和版本安全重试。`); }
  }
  async function postSaved(d: PurchaseDraft, version: number) {
    if (d.status === "POSTED") { showPostState(d, version); return; }
    if (d.status !== "DRAFT" || d.version !== version) { showPostState(d, version); return; }
    if (!postStorageKey) return;
    const target = { id: d.id, version };
    try { localStorage.setItem(postStorageKey, JSON.stringify(target)); }
    catch { setError(`已保存 ${d.documentNo}，无法保留过账恢复标识，尚未过账。`); return; }
    setPostPending(target); setPostRetryReady(false); setNotice(`已保存 ${d.documentNo}，正在过账…`);
    try { showPostState(await postPurchase(d.id, version), version); }
    catch (e) {
      const message = e instanceof Error ? e.message : "过账结果未知";
      setError(`${message}，正在核实单据实际状态。`);
      try { showPostState(await getPurchase(d.id), version, e instanceof ApiError && [400, 404, 409].includes(e.status ?? 0), message); }
      catch { setNotice(`已保存 ${d.documentNo}，过账结果尚待核实。`); setError(`${message}；暂时无法核实过账，请保留原单号并重查。`); }
    }
  }
  async function verifyPost() {
    if (!postPending || savingRef.current) return;
    savingRef.current = true; setSaving(true);
    try { showPostState(await getPurchase(postPending.id), postPending.version); }
    catch (e) { setError(e instanceof Error ? e.message : "过账状态核实失败"); }
    finally { savingRef.current = false; setSaving(false); }
  }
  async function applyResult(result: PurchaseSaveResult, postAfter = false) {
    if (result.state === "COMMITTED" && result.document && result.receipt) {
      acceptSaved(result.document, result.receipt.savedVersion);
      if (postAfter) await postSaved(result.document, result.receipt.savedVersion);
    }
    else if (result.state === "NOT_COMMITTED") { clearPending(); setError("已核实原保存未提交，可以继续修改并保存。"); }
    else setError("保存结果尚未确认，原请求可能仍在执行。请再次核实，或核实并关闭原标识后继续。");
  }
  async function verify(resolve = false) {
    if (!pending || savingRef.current) return;
    savingRef.current = true; setSaving(true);
    try { await applyResult(await (resolve ? resolvePurchaseSave(pending.operation, pending.key) : purchaseSaveResult(pending.operation, pending.key)), pending.postAfter === true); }
    catch (e) { setError(e instanceof Error ? e.message : "核实失败，请稍后重查。"); }
    finally { savingRef.current = false; setSaving(false); }
  }
  async function save(retry = false, postAfter = false) {
    if (savingRef.current || terminal || conflict || postPending || !storageKey || !recoveryLoaded || (!retry && (invalid || pending))) return;
    const p: Pending = retry && pending ? pending : { key: crypto.randomUUID(), operation: document ? "EDIT" : "CREATE", id: document?.id ?? 0, version: document?.version ?? 0, ...(postAfter ? { postAfter: true } : {}) };
    const body: PurchaseInput = retry && pendingBody.current ? pendingBody.current : { ...form, requestKey: p.key, remark: form.remark.trim() || undefined, items: form.items.map(l => ({ ...l, remark: l.remark.trim() || undefined })) };
    if (retry && !pendingBody.current) return;
    try { localStorage.setItem(storageKey, JSON.stringify(p)); }
    catch { setError("无法保存恢复标识，尚未提交。请检查浏览器存储后再试。"); return; }
    setPending(p); pendingBody.current = body;
    savingRef.current = true; setSaving(true); setError("");
    try {
      const d = p.operation === "EDIT" ? await updatePurchase(p.id, { ...body, version: p.version }) : await createPurchase(body);
      acceptSaved(d, d.saveReceipt?.savedVersion ?? d.version);
      if (p.postAfter) await postSaved(d, d.saveReceipt?.savedVersion ?? d.version);
    } catch (e) {
      const message = e instanceof Error ? e.message : "保存失败，输入已保留。";
      if (e instanceof ApiError && (e.status === 400 || e.status === 404 || e.status === 409)) {
        clearPending(); setError(message); if (e.status === 409) setConflict(true);
      } else {
        setError(`${message}，正在核实原保存结果。`);
        try { await applyResult(await purchaseSaveResult(p.operation, p.key), p.postAfter === true); }
        catch { setError(`${message}；暂时无法核实，请保留页面并重查。`); }
      }
    } finally { savingRef.current = false; setSaving(false); }
  }

  async function confirmSavePost() {
    const action = confirmation;
    setConfirmation(null);
    if (action === "SAVE_POST") { await save(false, true); return; }
    if (action !== "RETRY_POST" || !postPending || !document || document.id !== postPending.id || savingRef.current) return;
    savingRef.current = true; setSaving(true);
    try { await postSaved(document, postPending.version); }
    finally { savingRef.current = false; setSaving(false); }
  }
  const preview = confirmation === "RETRY_POST" && document ? fromDraft(document) : form;

  return <div className="mx-auto max-w-[1200px] space-y-space-4">
    <PageHeader title={document ? `采购录单 · ${document.documentNo}` : "新建采购单"} description={document ? `版本 ${document.version} · ${document.status === "DRAFT" ? "草稿，未改变库存与应付" : "单据已生效或取消，不能编辑"}` : "保存草稿后生成单号；草稿不改变库存或应付。"} actions={<Button onClick={() => navigate(returnTo)}>{returnTo.split("?")[0] === "/business/purchases" ? "返回采购列表" : "返回来源页面"}</Button>} />
    {notice && <p role="status" className="text-sm text-success">{notice}</p>}
    {error && <p role="alert" className="text-sm text-error">{error}</p>}
    {loading && <p role="status">加载采购草稿…</p>}
    {postPending && <div className="space-y-space-2 rounded-control border border-border bg-surface p-space-4">
      <p>单据 #{postPending.id} 的版本 {postPending.version} 过账结果待核实，保留原单据，不另建草稿。</p>
      <Button disabled={saving || loading} onClick={() => void verifyPost()}>核实过账状态</Button>{" "}
      {postRetryReady && <Button disabled={saving || loading} onClick={() => setConfirmation("RETRY_POST")}>重新核对并重试过账</Button>}
    </div>}
    {pending && <div className="space-y-space-2 rounded-control border border-border bg-surface p-space-4">
      <p>原保存尚待核实，暂时保留输入和保存标识，避免重复建单。</p>
      <div className="flex flex-wrap gap-space-2">
        <Button disabled={saving} onClick={() => void verify()}>核实保存结果</Button>
        <Button disabled={saving} onClick={() => void verify(true)}>核实并解除等待</Button>
        {pendingBody.current && <Button disabled={saving} onClick={() => void save(true)}>按原标识重试保存</Button>}
      </div>
    </div>}
    {(conflict || (error && id && !document)) && <Button disabled={saving || Boolean(pending)} onClick={() => { if (!dirty || window.confirm("重新加载将丢弃当前未保存输入，继续吗？")) void load(document?.id ?? documentID, true); }}>重新加载最新单据</Button>}
    <form onSubmit={e => { e.preventDefault(); void save(); }}>
      <fieldset disabled={locked} className="min-w-0 space-y-space-4">
        <FormSection title="供应商与业务资料">
          <Field label="供应商" required><PartnerSelect label="供应商" identity="SUPPLIER" value={form.partnerId} fallback={document?.partnerName} disabled={locked} onChange={p => { setPartner(p); setForm({ ...form, partnerId: p?.id ?? 0 }); }} /></Field>
          <Field label="业务日期" required htmlFor="purchase-date"><Input id="purchase-date" type="date" value={form.businessDate} onChange={e => setForm({ ...form, businessDate: e.target.value })} /></Field>
          <Field label="交付方式"><label className="flex items-center gap-space-2"><input type="checkbox" checked={form.directDelivery} onChange={e => setForm({ ...form, directDelivery: e.target.checked })} />供应商直接送达客户</label></Field>
          <Field label="整单备注" htmlFor="purchase-remark"><Textarea id="purchase-remark" maxLength={500} value={form.remark} onChange={e => setForm({ ...form, remark: e.target.value })} /></Field>
        </FormSection>
        <FormSection title="采购明细" description="可将同一商品按不同成交价分别录入；金额按行四舍五入至分。">
          <div className="space-y-space-3 md:col-span-2">
            {form.items.map((line, index) => {
              const selected = products.find(p => p.id === line.productId);
              return <div key={index} className="grid gap-space-3 rounded-control border border-border p-space-3 md:grid-cols-[2fr_1fr_1fr_auto]">
                <Field label={`第 ${index + 1} 行商品`} required><ProductSelect label={`第 ${index + 1} 行商品`} type="GOODS" disabled={locked} value={line.productId} fallback={document?.items.find(i => i.productId === line.productId)?.productName} onChange={p => {
                  if (p) setProducts(current => [...current.filter(x => x.id !== p.id), p]);
                  updateLine(index, { productId: p?.id ?? 0, unit: p?.unit ?? "", unitPrice: p?.purchasePrice ?? "0.00" });
                }} /></Field>
                <Field label="数量" required htmlFor={`quantity-${index}`}><Input id={`quantity-${index}`} inputMode="decimal" value={line.quantity} onChange={e => updateLine(index, { quantity: e.target.value })} /></Field>
                <Field label="成交单价（元）" required htmlFor={`price-${index}`}><Input id={`price-${index}`} inputMode="decimal" value={line.unitPrice} onChange={e => updateLine(index, { unitPrice: e.target.value })} /></Field>
                <div className="space-y-space-2"><p>金额：¥{lineAmounts[index] === null ? "待核对" : money(lineAmounts[index]!)}</p><Button disabled={form.items.length === 1} onClick={() => setForm({ ...form, items: form.items.filter((_, i) => i !== index) })}>删除</Button></div>
                <DraftProductConfirmation type={line.productType} unit={line.unit} current={selected} goodsOnly onConfirm={() => { if (selected?.type === "GOODS") updateLine(index, { unit: selected.unit }); }} />
                <Field label="行备注" htmlFor={`line-remark-${index}`}><Input id={`line-remark-${index}`} maxLength={500} value={line.remark} onChange={e => updateLine(index, { remark: e.target.value })} /></Field>
              </div>;
            })}
            <Button disabled={form.items.length >= 200} onClick={() => setForm({ ...form, items: [...form.items, blankLine()] })}>添加明细</Button>
          </div>
        </FormSection>
      </fieldset>
      <div className="mt-space-4 flex flex-wrap items-center justify-between gap-space-3 border-t border-border py-space-4">
        <p className="font-medium tabular-nums">合计：¥{total === null || total > 9223372036854775807n ? "待核对数量/单价" : money(total)} · {form.items.length} 行</p>
        <div className="flex gap-space-2">
          <Button type="submit" disabled={locked || invalid || conflict || !storageKey || !recoveryLoaded}>{saving ? "保存/核实中…" : "保存草稿"}</Button>
          <Button variant="primary" disabled={locked || invalid || conflict || !storageKey || !recoveryLoaded} onClick={() => setConfirmation("SAVE_POST")}>保存并过账</Button>
        </div>
      </div>
    </form>
    {blocker.state === "blocked" && <div role="alertdialog" aria-label="离开采购录单" className="space-y-space-3 rounded-control border border-border bg-surface p-space-4">
      <p>{saving || pending || postPending ? "保存或过账尚未核实，离开后仍需核实原结果；未保存的输入会丢失。" : "有未保存修改，离开将丢弃当前输入。"}</p>
      <Button onClick={() => blocker.reset()}>继续录单</Button> <Button disabled={saving} onClick={() => blocker.proceed()}>确认离开</Button>
    </div>}
    <FormDialog open={confirmation !== null} title={confirmation === "RETRY_POST" ? "重新核对并重试采购过账" : "确认保存并过账采购"} submitText="确认保存并过账" loading={saving} onCancel={() => setConfirmation(null)} onSubmit={confirmSavePost}>
      <p>供应商：{partner?.id === preview.partnerId ? `${partner.code} · ${partner.name}` : document?.partnerName ?? `#${preview.partnerId}`} · 日期：{preview.businessDate}</p>
      <ol className="my-space-3 space-y-space-2">
        {preview.items.map((line, index) => <li key={index}>{index + 1}. {[products.find(p => p.id === line.productId)?.code, products.find(p => p.id === line.productId)?.name, products.find(p => p.id === line.productId)?.model, products.find(p => p.id === line.productId)?.specification].filter(Boolean).join(" · ") || document?.items[index]?.productName || `商品 #${line.productId}`} · 数量 {line.quantity} {line.unit} · 单价 ¥{line.unitPrice} · 金额 ¥{money(amount(line.quantity, line.unitPrice) ?? 0n)}</li>)}
      </ol>
      <p className="font-medium">合计：¥{confirmation === "RETRY_POST" ? document?.totalAmount : total === null ? "待核对" : money(total)}</p>
      <p className="mt-space-3">{preview.directDelivery ? "直送采购：不进入店内库存，增加供应商应付；采购和销售仍需分别过账。" : "确认后增加商品库存和供应商应付。"} 保存失败不继续过账，过账失败保留同一草稿。</p>
    </FormDialog>
  </div>;
}
