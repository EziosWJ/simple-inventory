import { useBusinessLeaveGuard } from "@/hooks/use-business-leave-guard";
import { BusinessLeaveConfirm } from "@/components/business/business-leave-confirm";
import { businessReturnTo, withBusinessReturn } from "@/lib/business-navigation";
import { draftLineAmount as amount, draftMoney as money } from "@/components/business/draft-amount";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { createSale, getPartner, getProduct, getSale, getPurchase, postSale, saleSaveResult, resolveSaleSave, updateSale, type PartnerRecord, type ProductRecord, type SaleDraft, type SaleInput, type SaleSaveResult, type PurchaseDraft } from "@/api/business";
import { PartnerSelect, ProductSelect, PurchaseSourceSelect } from "@/components/business/master-data-select";
import { DraftProductConfirmation } from "@/components/business/draft-product-confirmation";
import { PageHeader } from "@/components/common/page-header";
import { FormDialog } from "@/components/common/form-dialog";
import { FormSection } from "@/components/common/form-section";
import { Field } from "@/components/common/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api-error";
import { useAuthStore } from "@/store/auth-store";

type Line = { id?: number; productId: number; productType: "GOODS" | "SERVICE"; unit: string; quantity: string; unitPrice: string; remark: string };
type Form = { partnerId: number; businessDate: string; directDelivery: boolean; directPurchaseId?: number; deliveryContact: string; deliveryPhone: string; deliveryAddress: string; remark: string; items: Line[] };
type Pending = { key: string; operation: "CREATE" | "EDIT"; id: number; version: number; postAfter?: boolean };
type PendingPost = { id: number; version: number };
const blankLine = (): Line => ({ productId: 0, productType: "GOODS", unit: "", quantity: "1", unitPrice: "0.00", remark: "" });
function initialForm(): Form {
  const now = new Date();
  const businessDate = new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
  return { partnerId: 0, businessDate, directDelivery: false, deliveryContact: "", deliveryPhone: "", deliveryAddress: "", remark: "", items: [blankLine()] };
}
function fromDraft(d: SaleDraft): Form {
  return { partnerId: d.partnerId, businessDate: d.businessDate, directDelivery: d.directDelivery, directPurchaseId: d.directPurchaseId ?? undefined, deliveryContact: d.deliveryContact ?? "", deliveryPhone: d.deliveryPhone ?? "", deliveryAddress: d.deliveryAddress ?? "", remark: d.remark ?? "", items: d.items.map(i => ({ id: i.id, productId: i.productId, productType: i.productType, unit: i.unit, quantity: i.quantity, unitPrice: i.unitPrice, remark: i.remark ?? "" })) };
}
export function SaleFormPage() {
  const { id } = useParams();
  const [params] = useSearchParams();
  const directSourceID = params.get("directPurchaseId");
  const [source, setSource] = useState<PurchaseDraft | null>(null);
  const navigate = useNavigate();
  const returnTo = businessReturnTo(params.get("returnTo"),"/business/sales");
  const user = useAuthStore(s => s.user);
  const documentID = id ? Number(id) : 0;
  const storageKey = user ? `simple-inventory:sale-save:${user.id}` : null;
  const postStorageKey = user ? `simple-inventory:sale-post:${user.id}` : null;
  const [form, setForm] = useState<Form>(initialForm);
  const [baseline, setBaseline] = useState(() => JSON.stringify(form));
  const [document, setDocument] = useState<SaleDraft | null>(null);
  const [products, setProducts] = useState<ProductRecord[]>([]);
  const [partner, setPartner] = useState<PartnerRecord | null>(null);
  const [confirmation, setConfirmation] = useState<"SAVE_POST" | "RETRY_POST" | null>(null);
  const [postPending, setPostPending] = useState<PendingPost | null>(null);
  const [postRetryReady, setPostRetryReady] = useState(false);
  const [loading, setLoading] = useState(Boolean(documentID || directSourceID));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [conflict, setConflict] = useState(false);
  const [pending, setPending] = useState<Pending | null>(null);
  const [recoveryLoaded, setRecoveryLoaded] = useState(false);
  const pendingBody = useRef<SaleInput | null>(null);
  const savingRef = useRef(false);
  const loadGeneration = useRef(0);
  const savedRoute = useRef<number | null>(null);
  const invalidateLoad = useCallback(() => { ++loadGeneration.current; }, []);
  const dirty = JSON.stringify(form) !== baseline;
  const terminal = document !== null && document.status !== "DRAFT";
  const locked = loading || saving || Boolean(confirmation) || Boolean(pending) || Boolean(postPending) || terminal || Boolean(id && !document);
  const leaveGuard = useBusinessLeaveGuard({ dirty, busy: saving, uncertain: Boolean(pending) || Boolean(postPending) });

  const load = useCallback(async (target = documentID, clearMessage = false) => {
    const generation = ++loadGeneration.current;
    if (!Number.isSafeInteger(target) || target < 1) { setError("销售单 ID 无效"); setLoading(false); return; }
    setLoading(true);
    try {
      const d = await getSale(target);
      const selected = await Promise.all([...new Set(d.items.map(i => i.productId))].map(getProduct));
      const supplier = await getPartner(d.partnerId);
      const linked = d.directPurchaseId ? await getPurchase(d.directPurchaseId) : null;
      if (generation !== loadGeneration.current) return;
      const next = fromDraft(d);
      setDocument(d); setProducts(selected); setPartner(supplier); setSource(linked); setForm(next); setBaseline(JSON.stringify(next));
      setError(""); setConflict(false);
      if (clearMessage) setNotice("");
    } catch (e) { if (generation === loadGeneration.current) setError(e instanceof Error ? e.message : "销售草稿加载失败"); }
    finally { if (generation === loadGeneration.current) setLoading(false); }
  }, [documentID]);

  const chooseSource = useCallback(async (p: PurchaseDraft | null) => {
    if (!p) { setSource(null); setForm(current => ({ ...current, directDelivery: false, directPurchaseId: undefined })); return; }
    const generation = ++loadGeneration.current;
    setLoading(true);
    try {
      const selected = await Promise.all([...new Set(p.items.map(i => i.productId))].map(getProduct));
      if (generation !== loadGeneration.current) return;
      setProducts(selected); setSource(p);
      setForm(current => ({ ...current, directDelivery: true, directPurchaseId: p.id, items: p.items.map(i => ({ productId: i.productId, productType: "GOODS", unit: i.unit, quantity: i.quantity, unitPrice: selected.find(x => x.id === i.productId)?.salePrice ?? "0.00", remark: "" })) }));
      setError("");
    } catch (e) { if (generation === loadGeneration.current) setError(e instanceof Error ? e.message : "直送采购资料加载失败"); }
    finally { if (generation === loadGeneration.current) setLoading(false); }
  }, []);
  useEffect(() => {
    if (id && savedRoute.current === documentID) { savedRoute.current = null; setLoading(false); }
    else if (id) void load();
    else { const next = initialForm(); setForm(next); setBaseline(JSON.stringify(next)); setDocument(null); setProducts([]); setPartner(null); setConflict(false); setError(""); setNotice("");
      if (directSourceID) {
        const target = Number(directSourceID);
        const generation = loadGeneration.current;
        setLoading(true);
        if (!Number.isSafeInteger(target) || target < 1) { setError("直送采购 ID 无效"); setLoading(false); }
        else void getPurchase(target).then(p => { if (generation !== loadGeneration.current) return; if (!p.directDelivery || p.status === "CANCELLED") { setError("来源必须是未取消的直送采购，请重新选择。"); setLoading(false); return; } void chooseSource(p); }).catch(e => { if (generation === loadGeneration.current) { setError(e instanceof Error ? e.message : "直送采购加载失败"); setLoading(false); } });
      } else setLoading(false);
    }
    return invalidateLoad;
  }, [id, documentID, load, directSourceID, chooseSource, invalidateLoad]);
  useEffect(() => {
    if (!documentID && document && !dirty && !pending && !postPending && !saving && !conflict) navigate(withBusinessReturn(`/business/sales/${document.id}/edit`, returnTo), { replace: true });
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

  function updateLine(index: number, patch: Partial<Line>) {
    setForm(current => ({ ...current, items: current.items.map((line, n) => n === index ? { ...line, ...patch } : line) }));
  }
  const unitPending = form.items.some(line => { const p = products.find(p => p.id === line.productId); return Boolean(p && (p.type !== line.productType || p.unit !== line.unit)); });
  const lineAmounts = form.items.map(line => amount(line.quantity, line.unitPrice));
  const total = lineAmounts.every(a => a !== null) ? lineAmounts.reduce<bigint>((sum, a) => sum + (a ?? 0n), 0n) : null;
  const invalid = (form.directDelivery && !form.directPurchaseId) || !form.partnerId || !form.businessDate || !form.items.length || form.items.length > 200 || form.items.some(line => !line.productId || !line.unit) || total === null || total > 9223372036854775807n || unitPending;

  function clearPending() {
    if (storageKey) localStorage.removeItem(storageKey);
    setPending(null); pendingBody.current = null;
  }
  function acceptSaved(d: SaleDraft, originalVersion: number) {
    if (!documentID) savedRoute.current = d.id;
    clearPending(); setDocument(d);
    // Preserve the user's input after their save, unless another operation has
    // subsequently changed the document. Reload is explicit in that case.
    const next = d.version === originalVersion ? { ...form, items: form.items.map((l, i) => ({ ...l, id: d.items[i]?.id })) } : form;
    setForm(next); setBaseline(JSON.stringify(next)); setConflict(false);
    setNotice(`已保存 ${d.documentNo}，原保存版本 ${originalVersion}；当前版本 ${d.version}，状态 ${d.status === "DRAFT" ? "草稿（未生效）" : d.status === "POSTED" ? "已过账" : "已取消"}。`);
    if (d.version !== originalVersion) { setConflict(true); setError("原保存已成功，单据随后被修改。请重新加载最新内容后继续编辑。"); }
    else setError("");
  }
  function clearPost() {
    if (postStorageKey) localStorage.removeItem(postStorageKey);
    setPostPending(null); setPostRetryReady(false);
  }
  function showPostState(d: SaleDraft, expected: number, definiteFailure = false, failure = "") {
    setDocument(d);
    if (document?.id !== d.id) { const next = fromDraft(d); setForm(next); setBaseline(JSON.stringify(next)); }
    if (d.status === "POSTED") { clearPost(); setConflict(false); setError(""); setNotice(`已过账 ${d.documentNo}，当前版本 ${d.version}；${d.directDelivery || d.items.every(i => i.productType === "SERVICE") ? "库存无变化，应收已处理" : "库存与应收已生效"}。`); }
    else if (d.status === "CANCELLED" || d.version !== expected) { clearPost(); setNotice(""); setConflict(true); setError(`原过账已停止：${d.documentNo} 当前${d.status === "CANCELLED" ? "已取消" : `版本 ${d.version}，已被修改`}，请重新加载核对。`); }
    else if (definiteFailure) { clearPost(); setNotice(`已保存 ${d.documentNo}，仍为草稿，尚未过账。`); setError(`${failure}；已保存 ${d.documentNo}，仍为草稿，输入已保留，可修改后重新确认。`); }
    else { setNotice(`已保存 ${d.documentNo}，过账结果尚待核实。`); setPostRetryReady(true); setError(`${d.documentNo} 当前仍为草稿，原过账结果尚未确认；请再次核实，或重新核对后按同一 ID 和版本安全重试。`); }
  }
  async function postSaved(d: SaleDraft, version: number) {
    if (d.status === "POSTED") { showPostState(d, version); return; }
    if (d.status !== "DRAFT" || d.version !== version) { showPostState(d, version); return; }
    if (!postStorageKey) return;
    const target = { id: d.id, version };
    try { localStorage.setItem(postStorageKey, JSON.stringify(target)); }
    catch { setError(`已保存 ${d.documentNo}，无法保留过账恢复标识，尚未过账。`); return; }
    setPostPending(target); setPostRetryReady(false); setNotice(`已保存 ${d.documentNo}，正在过账…`);
    try { showPostState(await postSale(d.id, version), version); }
    catch (e) {
      const message = e instanceof Error ? e.message : "过账结果未知";
      setError(`${message}，正在核实单据实际状态。`);
      try { showPostState(await getSale(d.id), version, e instanceof ApiError && [400, 404, 409].includes(e.status ?? 0), message); }
      catch { setNotice(`已保存 ${d.documentNo}，过账结果尚待核实。`); setError(`${message}；暂时无法核实过账，请保留原单号并重查。`); }
    }
  }
  async function verifyPost() {
    if (!postPending || savingRef.current) return;
    savingRef.current = true; setSaving(true);
    try { showPostState(await getSale(postPending.id), postPending.version); }
    catch (e) { setError(e instanceof Error ? e.message : "过账状态核实失败"); }
    finally { savingRef.current = false; setSaving(false); }
  }
  async function applyResult(result: SaleSaveResult, postAfter = false) {
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
    try { await applyResult(await (resolve ? resolveSaleSave(pending.operation, pending.key) : saleSaveResult(pending.operation, pending.key)), pending.postAfter === true); }
    catch (e) { setError(e instanceof Error ? e.message : "核实失败，请稍后重查。"); }
    finally { savingRef.current = false; setSaving(false); }
  }
  async function save(retry = false, postAfter = false) {
    if (savingRef.current || terminal || conflict || postPending || !storageKey || !recoveryLoaded || (!retry && (invalid || pending))) return;
    const p: Pending = retry && pending ? pending : { key: crypto.randomUUID(), operation: document ? "EDIT" : "CREATE", id: document?.id ?? 0, version: document?.version ?? 0, ...(postAfter ? { postAfter: true } : {}) };
    const body: SaleInput = retry && pendingBody.current ? pendingBody.current : { ...form, requestKey: p.key, remark: form.remark.trim() || undefined, items: form.items.map(l => ({ ...l, remark: l.remark.trim() || undefined })) };
    if (retry && !pendingBody.current) return;
    try { localStorage.setItem(storageKey, JSON.stringify(p)); }
    catch { setError("无法保存恢复标识，尚未提交。请检查浏览器存储后再试。"); return; }
    setPending(p); pendingBody.current = body;
    savingRef.current = true; setSaving(true); setError("");
    try {
      const d = p.operation === "EDIT" ? await updateSale(p.id, { ...body, version: p.version }) : await createSale(body);
      acceptSaved(d, d.saveReceipt?.savedVersion ?? d.version);
      if (p.postAfter) await postSaved(d, d.saveReceipt?.savedVersion ?? d.version);
    } catch (e) {
      const message = e instanceof Error ? e.message : "保存失败，输入已保留。";
      if (e instanceof ApiError && (e.status === 400 || e.status === 404 || e.status === 409)) {
        clearPending(); setError(message); if (e.status === 409) setConflict(true);
      } else {
        setError(`${message}，正在核实原保存结果。`);
        try { await applyResult(await saleSaveResult(p.operation, p.key), p.postAfter === true); }
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

  const goodsImpact = new Map<number, { name: string; unit: string; quantity: bigint }>();
  for (const [index, line] of preview.items.entries()) {
    if (line.productType !== "GOODS") continue;
    const [whole, decimal=""] = line.quantity.split(".");
    if (!/^\d+$/.test(whole) || !/^\d{0,3}$/.test(decimal)) continue;
    const q = BigInt(whole)*1000n+BigInt(decimal.padEnd(3,"0"));
    const old = goodsImpact.get(line.productId);
    goodsImpact.set(line.productId, { name: products.find(p=>p.id===line.productId)?.name ?? document?.items[index]?.productName ?? `商品 #${line.productId}`, unit: line.unit, quantity: (old?.quantity ?? 0n)+q });
  }
  return <div className="mx-auto max-w-[1200px] space-y-space-4">
    <PageHeader title={document ? `销售录单 · ${document.documentNo}` : "新建销售单"} description={document ? `版本 ${document.version} · ${document.status === "DRAFT" ? "草稿，未改变库存与应收" : "单据已生效或取消，不能编辑"}` : "保存草稿后生成单号；草稿不改变库存或应收。"} actions={<Button onClick={() => navigate(returnTo)}>{returnTo.split("?")[0] === "/business/sales" ? "返回销售列表" : "返回来源页面"}</Button>} />
    {notice && <p role="status" className="text-sm text-success">{notice}</p>}
    {error && <p role="alert" className="text-sm text-error">{error}</p>}
    {loading && <p role="status">加载销售草稿…</p>}
    {document && document.status !== "CANCELLED" && <Button onClick={() => navigate(`/business/sales/${document.id}/delivery-note`)}>打印已保存送货单</Button>}
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
        <FormSection title="客户与业务资料">
          <Field label="客户" required><PartnerSelect label="客户" identity="CUSTOMER" value={form.partnerId} fallback={document?.partnerName} disabled={locked} onChange={p => { setPartner(p); setForm({ ...form, partnerId: p?.id ?? 0, deliveryContact: p?.contact ?? "", deliveryPhone: p?.phone ?? "", deliveryAddress: p?.address ?? "" }); }} /></Field>
          <Field label="业务日期" required htmlFor="sale-date"><Input id="sale-date" type="date" value={form.businessDate} onChange={e => setForm({ ...form, businessDate: e.target.value })} /></Field>
          <Field label="关联直送采购" help="普通销售可留空；直送采购与销售分别过账，实物按商品汇总数量一致，可添加服务。">
            <PurchaseSourceSelect label="关联直送采购" value={form.directPurchaseId ?? 0} disabled={locked || Boolean(document?.directPurchaseId)} onChange={p => void chooseSource(p)} />
            {source && <p>{source.documentNo} · {source.partnerName} · {source.status === "POSTED" ? "采购已过账" : "采购尚未过账"}</p>}
          </Field>
          <Field label="送货联系人" htmlFor="sale-contact"><Input id="sale-contact" maxLength={100} value={form.deliveryContact} onChange={e => setForm({ ...form, deliveryContact: e.target.value })} /></Field>
          <Field label="收货人电话" htmlFor="sale-phone"><Input id="sale-phone" maxLength={50} value={form.deliveryPhone} onChange={e => setForm({ ...form, deliveryPhone: e.target.value })} /></Field>
          <Field label="送货地址" htmlFor="sale-address" help="请填写完整地址，包含楼栋、楼层及房间号"><Textarea id="sale-address" rows={3} maxLength={500} placeholder="例如：南屯为民服务中心 3楼301室" value={form.deliveryAddress} onChange={e => setForm({ ...form, deliveryAddress: e.target.value })} /></Field>
          <Field label="整单备注" htmlFor="sale-remark"><Textarea id="sale-remark" maxLength={500} value={form.remark} onChange={e => setForm({ ...form, remark: e.target.value })} /></Field>
        </FormSection>
        <FormSection title="销售明细" description="可将同一商品按不同成交价分别录入；金额按行四舍五入至分。">
          <div className="space-y-space-3 md:col-span-2">
            {form.items.map((line, index) => {
              const selected = products.find(p => p.id === line.productId);
              return <div key={index} className="grid gap-space-3 rounded-control border border-border p-space-3 md:grid-cols-[2fr_1fr_1fr_auto]">
                <Field label={`第 ${index + 1} 行商品`} required><ProductSelect label={`第 ${index + 1} 行商品`} disabled={locked} value={line.productId} fallback={document?.items.find(i => i.productId === line.productId)?.productName} onChange={p => {
                  if (p) setProducts(current => [...current.filter(x => x.id !== p.id), p]);
                  updateLine(index, { productId: p?.id ?? 0, unit: p?.unit ?? "", productType: p?.type ?? "GOODS", unitPrice: p?.salePrice ?? "0.00" });
                }} /></Field>
                <Field label="数量" required htmlFor={`quantity-${index}`}><Input id={`quantity-${index}`} inputMode="decimal" value={line.quantity} onChange={e => updateLine(index, { quantity: e.target.value })} /></Field>
                <Field label="成交单价（元）" required htmlFor={`price-${index}`}><Input id={`price-${index}`} inputMode="decimal" value={line.unitPrice} onChange={e => updateLine(index, { unitPrice: e.target.value })} /></Field>
                <div className="space-y-space-2"><p>金额：¥{lineAmounts[index] === null ? "待核对" : money(lineAmounts[index]!)}</p><Button disabled={form.items.length === 1} onClick={() => setForm({ ...form, items: form.items.filter((_, i) => i !== index) })}>删除</Button></div>
                <DraftProductConfirmation type={line.productType} unit={line.unit} current={selected} onConfirm={() => { if (selected) updateLine(index, { productType: selected.type, unit: selected.unit }); }} />
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
    <FormDialog trapFocus={!leaveGuard.open} closeOnEscape={!leaveGuard.open} closeOnOverlayClick={!leaveGuard.open} open={confirmation !== null} title={confirmation === "RETRY_POST" ? "重新核对并重试销售过账" : "确认保存并过账销售"} submitText="确认保存并过账" loading={saving} onCancel={() => setConfirmation(null)} onSubmit={confirmSavePost}>
      <p>客户：{partner?.id === preview.partnerId ? `${partner.code} · ${partner.name}` : document?.partnerName ?? `#${preview.partnerId}`} · 日期：{preview.businessDate}</p>
      <ol className="my-space-3 space-y-space-2">
        {preview.items.map((line, index) => <li key={index}>{index + 1}. {[products.find(p => p.id === line.productId)?.code, products.find(p => p.id === line.productId)?.name, products.find(p => p.id === line.productId)?.model, products.find(p => p.id === line.productId)?.specification].filter(Boolean).join(" · ") || document?.items[index]?.productName || `商品 #${line.productId}`} · 数量 {line.quantity} {line.unit} · 单价 ¥{line.unitPrice} · 金额 ¥{money(amount(line.quantity, line.unitPrice) ?? 0n)}</li>)}
      </ol>
      <p className="font-medium">合计：¥{confirmation === "RETRY_POST" ? document?.totalAmount : total === null ? "待核对" : money(total)}</p>
      <p>送货：{preview.deliveryContact} / {preview.deliveryPhone} / {preview.deliveryAddress}</p>
      <p className="mt-space-3">{preview.directDelivery ? `直送销售：关联采购 #${preview.directPurchaseId}；不扣店内库存，先采购后销售分别过账。` : goodsImpact.size ? "扣减店内实物库存（同商品合计）：" : "纯服务销售：不改变库存。"}</p>
      {!preview.directDelivery && [...goodsImpact].map(([id, item]) => <p key={id}>{item.name}：-{item.quantity / 1000n}.{String(item.quantity % 1000n).padStart(3,"0")} {item.unit}</p>)}
      <p>增加客户应收；零金额不产生往来金额流水。保存失败不继续过账，过账失败保留同一草稿。</p>
    </FormDialog>
    <BusinessLeaveConfirm guard={leaveGuard} title="离开销售录单" stayText="继续录单" leaveText="确认离开" description={saving || pending || postPending ? "保存或过账尚未核实，离开后仍需核实原结果；未保存的输入会丢失。" : undefined} />
  </div>;
}
