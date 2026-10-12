import { DirectOrderActions } from "@/components/business/direct-order-actions";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { getProduct, type ProductRecord } from "@/api/business";
import { createDirectOrder, updateDirectOrder, getDirectOrder, directSaveResult, directStatus, type DirectOrder, type DirectOrderInput, type DirectOrderLineInput } from "@/api/direct-orders";
import { PartnerSelect, ProductSelect } from "@/components/business/master-data-select";
import { DraftProductConfirmation } from "@/components/business/draft-product-confirmation";
import { draftLineAmount, draftMoney } from "@/components/business/draft-amount";
import { BusinessLeaveConfirm } from "@/components/business/business-leave-confirm";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessLeaveGuard } from "@/hooks/use-business-leave-guard";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { useSelectionChoice } from "@/hooks/use-selection-choice";
import { PageHeader } from "@/components/common/page-header";
import { FormSection } from "@/components/common/form-section";
import { Field } from "@/components/common/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api-error";
import { businessToday, formatDateTime } from "@/lib/datetime";
import { randomUUID } from "@/lib/uuid";
import { useAuthStore } from "@/store/auth-store";
type Line = DirectOrderLineInput & { priceEdited?: boolean };
type Form = Omit<DirectOrderInput, "items" | "requestKey"> & { items: Line[] };
type Pending = { key: string; operation: "CREATE" | "EDIT"; id: number; version: number };
const blank = (type: "GOODS" | "SERVICE" = "GOODS"): Line => ({ productId: 0, productType: type, unit: "", quantity: "1", purchasePrice: "0.00", salePrice: "0.00", remark: "" });
const initial = (): Form => ({ supplierId: 0, customerId: 0, businessDate: businessToday(), deliveryContact: "", deliveryPhone: "", deliveryAddress: "", remark: "", items: [blank()] });
function fromDocument(d: DirectOrder): Form {
  return { supplierId: d.supplierId, customerId: d.customerId, businessDate: d.businessDate, deliveryContact: d.deliveryContact ?? "", deliveryPhone: d.deliveryPhone ?? "", deliveryAddress: d.deliveryAddress ?? "", remark: d.remark ?? "", items: d.items.map(l => ({ productId: l.productId, productType: l.productType, unit: l.unit, quantity: l.quantity, purchasePrice: l.purchasePrice, salePrice: l.salePrice, remark: l.remark ?? "", priceEdited: true })) };
}
export function DirectOrderFormPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const user = useAuthStore(s => s.user);
  const storageKey = `simple-inventory:direct-save:${user?.id ?? "unknown"}`;
  const [form, setForm] = useState<Form>(initial);
  const [baseline, setBaseline] = useState(() => JSON.stringify(form));
  const [document, setDocument] = useState<DirectOrder | null>(null);
  const [products, setProducts] = useState<ProductRecord[]>([]);
  const [loading, setLoading] = useState(Boolean(id));
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const generation = useRef(0);
  const [error, setError] = useState("");
  const [savedID, setSavedID] = useState<number | null>(null);
  const [pending, setPending] = useState<Pending | null>(() => {
    try { const raw = localStorage.getItem(storageKey); if (!raw) return null; const p = JSON.parse(raw); return typeof p.key === "string" && ["CREATE", "EDIT"].includes(p.operation) && Number.isSafeInteger(p.id) && Number.isSafeInteger(p.version) ? p : null; } catch { return null; }
  });
  const pendingBody = useRef<DirectOrderInput | null>(null);
  const deliveryEdited = useRef(false);
  const selection = useSelectionChoice();
  const { feedback, notify } = useBusinessFeedback();
  const dirty = JSON.stringify(form) !== baseline;
  const locked = loading || saving || Boolean(pending) || selection.busy || Boolean(id && !document) || Boolean(document && document.status !== "DRAFT");
  const guard = useBusinessLeaveGuard({ dirty, busy: saving, uncertain: Boolean(pending) });
  useEffect(() => { if (savedID && !saving && !dirty && !pending) { setSavedID(null); navigate(`/business/direct-orders/${savedID}/edit`, { replace: true }); } }, [savedID, saving, dirty, pending, navigate]);
  const adopt = useCallback((d: DirectOrder) => {
    const next = fromDocument(d); setDocument(d); setForm(next); setBaseline(JSON.stringify(next)); deliveryEdited.current = true;
  }, []);
  const load = useCallback(async () => {
    const target = Number(id); if (!Number.isSafeInteger(target) || target < 1) { setError("直送单 ID 无效"); setLoading(false); return; }
    const current = ++generation.current; setLoading(true);
    try { const d = await getDirectOrder(target); const selected = await Promise.all([...new Set(d.items.map(l => l.productId))].map(getProduct)); if (current === generation.current) { adopt(d); setProducts(selected); setError(""); } }
    catch (e) { if (current === generation.current) setError(e instanceof Error ? e.message : "直送单加载失败"); }
    finally { if (current === generation.current) setLoading(false); }
  }, [id, adopt]);
  const invalidateLoad = useCallback(() => { ++generation.current; }, []);
  useEffect(() => { if (id) void load(); return invalidateLoad; }, [id, load, invalidateLoad]);
  const amounts = form.items.map(l => ({ purchase: l.productType === "SERVICE" ? 0n : draftLineAmount(l.quantity, l.purchasePrice), sale: draftLineAmount(l.quantity, l.salePrice) }));
  const purchaseTotal = amounts.every(a => a.purchase !== null) ? amounts.reduce((n, a) => n + a.purchase!, 0n) : null;
  const saleTotal = amounts.every(a => a.sale !== null) ? amounts.reduce((n, a) => n + a.sale!, 0n) : null;
  const unitsPending = form.items.some(l => { const p = products.find(p => p.id === l.productId); return p && (p.type !== l.productType || p.unit !== l.unit); });
  const invalid = !form.supplierId || !form.customerId || !form.businessDate || !form.items.some(l => l.productType === "GOODS") || form.items.some(l => !l.productId || !l.unit || !/^\d+(\.\d{1,3})?$/.test(l.quantity) || !/[1-9]/.test(l.quantity)) || unitsPending || purchaseTotal === null || saleTotal === null || purchaseTotal > 9223372036854775807n || saleTotal > 9223372036854775807n;
  function updateLine(index: number, patch: Partial<Line>) { setForm(current => ({ ...current, items: current.items.map((l, i) => i === index ? { ...l, ...patch } : l) })); }
  async function selectProduct(index: number, p: ProductRecord | null) {
    const line = form.items[index]; if (!line) return false;
    if (line.productId === (p?.id ?? 0)) { if (p) setProducts(current => [...current.filter(x => x.id !== p.id), p]); return true; }
    if (line.priceEdited || (line.unit && p && line.unit !== p.unit)) {
      const choice = await selection.choose({ title: "核对商品与成交价", description: `更换商品后请核对数量和单位。当前采购价 ${line.purchasePrice}、销售价 ${line.salePrice}。`, keepText: "保留成交价", replaceText: "使用新参考价" });
      if (choice === null) return false;
      if (p) setProducts(current => [...current.filter(x => x.id !== p.id), p]);
      updateLine(index, { productId: p?.id ?? 0, unit: p?.unit ?? "", productType: p?.type ?? line.productType, purchasePrice: line.productType === "SERVICE" ? "0.00" : choice ? p?.purchasePrice ?? "0.00" : line.purchasePrice, salePrice: choice ? p?.salePrice ?? "0.00" : line.salePrice, priceEdited: !choice });
    } else {
      if (p) setProducts(current => [...current.filter(x => x.id !== p.id), p]);
      updateLine(index, { productId: p?.id ?? 0, unit: p?.unit ?? "", productType: p?.type ?? line.productType, purchasePrice: p?.type === "SERVICE" ? "0.00" : p?.purchasePrice ?? "0.00", salePrice: p?.salePrice ?? "0.00" });
    }
    return true;
  }
  function clearPending() { localStorage.removeItem(storageKey); setPending(null); pendingBody.current = null; }
  async function save(retry = false) {
    if (savingRef.current || (!retry && (locked || invalid))) return;
    const attempt = retry ? pending : { key: randomUUID(), operation: document ? "EDIT" as const : "CREATE" as const, id: document?.id ?? 0, version: document?.version ?? 0 };
    if (!attempt) return;
    const body = retry ? pendingBody.current : { ...form, requestKey: attempt.key, items: form.items.map(l => { const value = { ...l }; delete value.priceEdited; return value; }) };
    if (!body) return;
    try { localStorage.setItem(storageKey, JSON.stringify(attempt)); } catch { setError("无法保存恢复标识，请检查浏览器存储后重试。"); return; }
    ++generation.current; savingRef.current = true; setSaving(true); setPending(attempt); pendingBody.current = body; setError("");
    try {
      const d = attempt.operation === "CREATE" ? await createDirectOrder(body) : await updateDirectOrder(attempt.id, { ...body, version: attempt.version });
      clearPending(); adopt(d); notify({ type: "success", title: "直送草稿已保存", description: "库存和往来未变化。", documentNo: d.documentNo, status: directStatus[d.status] });
      setSavedID(d.id);
    } catch (e) {
      if (e instanceof ApiError && e.status && e.status < 500) clearPending();
      setError(e instanceof Error ? e.message : "保存结果待核实");
    } finally { savingRef.current = false; setSaving(false); }
  }
  async function verify(resolve = false) {
    if (!pending || savingRef.current) return; savingRef.current = true; setSaving(true);
    try {
      const result = await directSaveResult(pending.operation, pending.key, resolve);
      if (result.state === "COMMITTED" && result.document) { const d = result.document; clearPending(); adopt(d); setError(""); setSavedID(d.id); notify({ type: "success", title: "已核实原保存结果", description: "沿用原草稿，没有重复建单。", documentNo: d.documentNo }); }
      else if (result.state === "NOT_COMMITTED") { clearPending(); setError("已核实原保存未提交，可以按当前实际情况重新保存。"); }
      else setError("原保存仍待核实，可再次核实或关闭未提交标识。");
    } catch (e) { setError(e instanceof Error ? e.message : "核实失败，请保留原标识"); }
    finally { savingRef.current = false; setSaving(false); }
  }
  return <div className="mx-auto max-w-form-wide space-y-space-4">
    <PageHeader title={document ? `编辑直送草稿 · ${document.documentNo}` : "新建直送单"} description="实物共用商品和数量，分别填写采购与销售价格；草稿不改变库存或往来。" actions={<Button onClick={() => guard.requestClose(() => navigate("/business/direct-orders"))}>返回列表</Button>} />
    <BusinessFeedback feedback={feedback} />
    {error && <p role="alert" className="text-error">{error}</p>}
    {loading && <p role="status">加载直送单…</p>}
    {document && <p>{directStatus[document.status]} · 版本 {document.version} · 创建人 {document.createdByName} · {formatDateTime(document.createTime)}</p>}
    {pending && <div className="space-y-space-2 rounded-control border border-border p-space-4"><p>原保存结果待核实，保留原标识，避免重复建单。</p><Button disabled={saving} onClick={() => void verify()}>核实保存结果</Button> <Button disabled={saving} onClick={() => void verify(true)}>核实并关闭未提交标识</Button> {pendingBody.current && <Button disabled={saving} onClick={() => void save(true)}>按原标识重试</Button>}</div>}
    {error && id && !pending && <Button disabled={saving || loading} onClick={() => { if (!dirty || window.confirm("重新加载会丢弃未保存输入，继续吗？")) void load(); }}>重新加载最新单据</Button>}
    {document && <DirectOrderActions document={document} disabled={dirty || loading || saving || Boolean(pending)} onChange={adopt} />}
    <form onSubmit={e => { e.preventDefault(); void save(); }}>
      <fieldset disabled={locked} className="min-w-0 space-y-space-4">
        <FormSection title="供应商与客户">
          <Field label="供应商" required><PartnerSelect inlineBrowse identity="SUPPLIER" label="供应商" value={form.supplierId} fallback={document?.supplierName} disabled={locked} onChange={p => setForm({ ...form, supplierId: p?.id ?? 0 })} /></Field>
          <Field label="客户" required><PartnerSelect inlineBrowse identity="CUSTOMER" label="客户" value={form.customerId} fallback={document?.customerName} disabled={locked} onChange={p => { setForm(current => ({ ...current, customerId: p?.id ?? 0, ...(!deliveryEdited.current ? { deliveryContact: p?.contact ?? "", deliveryPhone: p?.phone ?? "", deliveryAddress: p?.address ?? "" } : {}) })); }} /></Field>
          <Field label="业务日期" required htmlFor="direct-date"><Input id="direct-date" type="date" value={form.businessDate} onChange={e => setForm({ ...form, businessDate: e.target.value })} /></Field>
          <Field label="整单备注" htmlFor="direct-remark"><Textarea id="direct-remark" maxLength={500} value={form.remark} onChange={e => setForm({ ...form, remark: e.target.value })} /></Field>
        </FormSection>
        <FormSection title="送货资料">
          <Field label="送货联系人" htmlFor="direct-contact"><Input id="direct-contact" maxLength={100} value={form.deliveryContact} onChange={e => { deliveryEdited.current = true; setForm({ ...form, deliveryContact: e.target.value }); }} /></Field>
          <Field label="送货电话" htmlFor="direct-phone"><Input id="direct-phone" maxLength={50} value={form.deliveryPhone} onChange={e => { deliveryEdited.current = true; setForm({ ...form, deliveryPhone: e.target.value }); }} /></Field>
          <Field label="送货地址" htmlFor="direct-address"><Textarea id="direct-address" maxLength={500} value={form.deliveryAddress} onChange={e => { deliveryEdited.current = true; setForm({ ...form, deliveryAddress: e.target.value }); }} /></Field>
        </FormSection>
        <FormSection title="直送明细" description="至少一条实物；服务只记录销售价。">
          <div className="space-y-space-3 md:col-span-2">{form.items.map((line, index) => <div key={index} className="grid gap-space-3 rounded-control border border-border p-space-3 md:grid-cols-4">
            <Field label={`第 ${index + 1} 行${line.productType === "SERVICE" ? "服务" : "商品"}`} required><ProductSelect inlineBrowse type={line.productType} label={`第 ${index + 1} 行商品`} value={line.productId} fallback={document?.items[index]?.productName} disabled={locked} onChange={p => selectProduct(index, p)} /></Field>
            <Field label="数量" htmlFor={`direct-qty-${index}`} required><Input id={`direct-qty-${index}`} inputMode="decimal" value={line.quantity} onChange={e => updateLine(index, { quantity: e.target.value })} /></Field>
            {line.productType === "GOODS" && <Field label="采购单价（元）" htmlFor={`direct-buy-${index}`} required><Input id={`direct-buy-${index}`} inputMode="decimal" value={line.purchasePrice} onChange={e => updateLine(index, { purchasePrice: e.target.value, priceEdited: true })} /></Field>}
            <Field label="销售单价（元）" htmlFor={`direct-sell-${index}`} required><Input id={`direct-sell-${index}`} inputMode="decimal" value={line.salePrice} onChange={e => updateLine(index, { salePrice: e.target.value, priceEdited: true })} /></Field>
            <DraftProductConfirmation type={line.productType} unit={line.unit} current={products.find(p => p.id === line.productId)} onConfirm={() => { const p = products.find(p => p.id === line.productId); if (p) updateLine(index, { productType: p.type, unit: p.unit, ...(p.type === "SERVICE" ? { purchasePrice: "0.00" } : {}) }); }} />
            <Field label="行备注" htmlFor={`direct-line-remark-${index}`}><Input id={`direct-line-remark-${index}`} maxLength={500} value={line.remark ?? ""} onChange={e => updateLine(index, { remark: e.target.value })} /></Field>
            <p>采购金额 ¥{amounts[index].purchase === null ? "待核对" : draftMoney(amounts[index].purchase!)} / 销售金额 ¥{amounts[index].sale === null ? "待核对" : draftMoney(amounts[index].sale!)}</p>
            <Button disabled={form.items.length === 1} onClick={() => setForm({ ...form, items: form.items.filter((_, i) => i !== index) })}>删除明细</Button>
          </div>)}
          <div className="flex gap-space-2"><Button disabled={form.items.length >= 200} onClick={() => setForm({ ...form, items: [...form.items, blank()] })}>添加实物</Button><Button disabled={form.items.length >= 200} onClick={() => setForm({ ...form, items: [...form.items, blank("SERVICE")] })}>添加服务</Button></div></div>
        </FormSection>
      </fieldset>
      <div className="mt-space-4 flex flex-wrap items-center justify-between gap-space-3 border-t border-border py-space-4"><p>采购合计 ¥{purchaseTotal === null ? "待核对" : draftMoney(purchaseTotal)} / 销售合计 ¥{saleTotal === null ? "待核对" : draftMoney(saleTotal)}</p>{<Button type="submit" variant="primary" disabled={locked || invalid}>{saving ? "保存中…" : "保存草稿"}</Button>}</div>
    </form>
    <BusinessLeaveConfirm guard={guard} />
    {selection.dialog}
  </div>;
}
