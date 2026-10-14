import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { createOpeningBalance, createRefund, createSettlement, partnerBalances, partnerBalanceEntries, type PartnerBalance, type PartnerBalanceEntry } from "@/api/business";
import { PartnerAttachmentUpload } from "@/components/business/partner-attachments";
import { PartnerEntryLedger } from "@/components/business/partner-entry-ledger";
import { PartnerSelect } from "@/components/business/master-data-select";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { BusinessLeaveConfirm } from "@/components/business/business-leave-confirm";
import { FormDialog } from "@/components/common/form-dialog";
import { Field } from "@/components/common/field";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { useBusinessLeaveGuard } from "@/hooks/use-business-leave-guard";
import { emptyAttachments } from "@/lib/partner-attachments";
import { balanceDescription, cents, localToday, money, paymentMethodLabel } from "@/lib/partner-ledger";
import { businessReturnTo } from "@/lib/business-navigation";
import { randomUUID } from "@/lib/uuid";
import { PartnerRecords, type RecordCategory } from "./partner-records";

type Direction = "CUSTOMER" | "SUPPLIER";
type Draft = { partnerId: string; direction: Direction; amount: string; date: string; method: string; transactionNo: string; remark: string };
function blankDraft(direction: Direction, partnerId = ""): Draft {
  return { partnerId, direction, amount: "", date: localToday(), method: "CASH", transactionNo: "", remark: "" };
}
function operationName(category: RecordCategory, direction: Direction) {
  if (category === "OPENING") return direction === "CUSTOMER" ? "录入客户原有欠款" : "录入欠供应商款";
  if (category === "REFUND") return direction === "CUSTOMER" ? "向客户退款" : "登记供应商退款";
  return direction === "CUSTOMER" ? "登记收款" : "登记付款";
}

export function PartnerEntryPage({ category }: { category: RecordCategory }) {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const opening = category === "OPENING", refund = category === "REFUND";
  const [draft, setDraft] = useState(() => blankDraft(params.get("direction") === "SUPPLIER" ? "SUPPLIER" : "CUSTOMER", params.get("partnerId") ?? ""));
  const [baseline, setBaseline] = useState(() => JSON.stringify(draft));
  const [editing, setEditing] = useState(() => !!params.get("partnerId") && !params.get("entryId"));
  const [reviewing, setReviewing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [browsing, setBrowsing] = useState(false);
  const [showLedger, setShowLedger] = useState(false);
  const [attachments, setAttachments] = useState(emptyAttachments);
  const [requestKey, setRequestKey] = useState(randomUUID);
  const [balance, setBalance] = useState<PartnerBalance | null>(null);
  const [ready, setReady] = useState(false);
  const [balanceError, setBalanceError] = useState("");
  const [openingCount, setOpeningCount] = useState<number | null>(null);
  const [openingError, setOpeningError] = useState("");
  const [error, setError] = useState("");
  const [version, setVersion] = useState(0);
  const [receipt, setReceipt] = useState<PartnerBalanceEntry | null>(null);
  const [detailId, setDetailId] = useState<number | null>(null);
  const { feedback, notify } = useBusinessFeedback();
  const guard = useBusinessLeaveGuard({ dirty: editing && (attachments.dirty || JSON.stringify(draft) !== baseline), busy: saving });
  const operation = operationName(category, draft.direction);
  const title = opening ? "期初往来" : refund ? "退款" : "收付款";
  const returnTo = businessReturnTo(params.get("returnTo"), "");

  useEffect(() => {
    if (!editing || !draft.partnerId) return;
    let active = true;
    void partnerBalances({ partnerId: draft.partnerId, direction: draft.direction, page: 1, pageSize: 1 })
      .then(result => { if (active) { setBalance(result.records[0] ?? null); setReady(true); setBalanceError(""); } })
      .catch(e => { if (active) setBalanceError(e instanceof Error ? e.message : "读取余额失败"); });
    if (opening) void partnerBalanceEntries({ category: "OPENING", partnerId: draft.partnerId, direction: draft.direction, reversalStatus: "EFFECTIVE", page: 1, pageSize: 1 })
      .then(result => { if (active) { setOpeningCount(result.total); setOpeningError(""); } })
      .catch(() => { if (active) setOpeningError("未能核对已有期初记录，请重试或先查看历史记录。"); });
    return () => { active = false; };
  }, [editing, draft.partnerId, draft.direction, version, opening]);

  const current = ready ? cents(balance?.amount ?? "0.00") : null;
  const value = cents(draft.amount);
  const available = current === null ? null : refund ? (current < 0n ? -current : 0n) : (current > 0n ? current : 0n);
  const amountError = draft.amount && (value === null || value <= 0n) ? "请输入大于零的金额，最多两位小数。"
    : !opening && value !== null && available !== null && value > available ? "本次金额不能超过 ¥" + money(available) + "。" : "";
  const amountValid = value !== null && value > 0n && current !== null && (opening || available !== null && available > 0n && value <= available);
  const valid = !!draft.partnerId && ready && amountValid && !!draft.date && (!opening || !!draft.remark.trim()) && !attachments.blocked;
  const after = amountValid && current !== null && value !== null ? money(current + (opening || refund ? value : -value)) : null;
  const amountLabel = opening ? "本次补记欠款（元）" : refund ? draft.direction === "CUSTOMER" ? "本次实际退给客户（元）" : "本次实际收到退款（元）" : draft.direction === "CUSTOMER" ? "本次实际收到（元）" : "本次实际支付（元）";
  const fullLabel = refund ? draft.direction === "CUSTOMER" ? "退清款项" : "收齐退款" : draft.direction === "CUSTOMER" ? "收齐欠款" : "付清欠款";
  const balanceLabel = current !== null ? balanceDescription(draft.direction, money(current)).split(" ¥")[0] : "当前往来";
  const balanceAmount = current !== null ? money(current < 0n ? -current : current).replace(/\B(?=(\d{3})+(?!\d))/g, ",") : "—";

  function refresh() { setReady(false); setBalanceError(""); setOpeningCount(null); setOpeningError(""); setVersion(v => v + 1); }
  function start(direction: Direction, partnerId = "") {
    const next = blankDraft(direction, partnerId);
    setDraft(next); setBaseline(JSON.stringify(next)); setAttachments(emptyAttachments); setRequestKey(randomUUID());
    setBalance(null); setReady(false); setBalanceError(""); setOpeningCount(null); setOpeningError(""); setError(""); setShowLedger(false); setReviewing(false); setEditing(true);
  }
  function close() {
    guard.requestClose(() => { setEditing(false); setReviewing(false); setAttachments(emptyAttachments); });
  }
  async function save() {
    if (!valid || saving || value === null) return;
    setSaving(true); setError("");
    try {
      const common = { attachmentIds: attachments.ids, requestKey, partnerId: Number(draft.partnerId), direction: draft.direction, amount: draft.amount, businessDate: draft.date };
      const saved = opening ? await createOpeningBalance({ ...common, description: draft.remark.trim() })
        : await (refund ? createRefund : createSettlement)({ ...common, paymentMethod: draft.method, transactionNo: draft.transactionNo, remark: draft.remark });
      const result = { ...saved, partnerName: saved.partnerName || balance?.partnerName || "往来单位" };
      setReceipt(result); setEditing(false); setReviewing(false); setAttachments(emptyAttachments); setVersion(v => v + 1);
      notify({ type: "success", title: operation + "成功", description: result.partnerName + "，本次 ¥" + money(value) + "；" + balanceDescription(saved.direction, saved.balanceAfter) + "。", documentNo: saved.documentNo, status: "已生效" });
    } catch (e) {
      setError(e instanceof Error ? e.message : "登记失败，请核对后重试"); setReviewing(false); refresh();
    } finally { setSaving(false); }
  }
  function ledgerPath(partnerId: string | number, direction: Direction) {
    return "/business/partner-ledger?partnerId=" + partnerId + "&direction=" + direction;
  }

  return <div className="space-y-space-6">
    <PageHeader title={title} description={opening ? "记录启用系统前尚未结清的欠款。" : refund ? "查询退款记录，登记实际退给客户或收到供应商的退款。" : "查询收付款记录，登记实际收到或付出的款项。"} actions={<div className="flex flex-wrap gap-2">
      {returnTo && <Button onClick={() => navigate(returnTo)}>返回来源页面</Button>}
      <Button variant="primary" onClick={() => start("CUSTOMER")}>{operationName(category, "CUSTOMER")}</Button>
      <Button onClick={() => start("SUPPLIER")}>{operationName(category, "SUPPLIER")}</Button>
    </div>} />
    <BusinessFeedback feedback={feedback} />
    {receipt && <section aria-label="登记结果" className="flex flex-wrap items-center justify-between gap-3 border-l-2 border-primary bg-surface px-4 py-3 text-sm">
      <p>{receipt.partnerName}：{balanceDescription(receipt.direction, receipt.balanceAfter)}<span className="ml-2 text-text-tertiary">（本笔登记后）</span></p>
      <div className="flex flex-wrap gap-2"><Button size="sm" onClick={() => setDetailId(receipt.id)}>查看本笔记录</Button><Button size="sm" onClick={() => navigate(ledgerPath(receipt.partnerId, receipt.direction))}>查看往来</Button><Button size="sm" onClick={() => start(receipt.direction, String(receipt.partnerId))}>再登记一笔</Button></div>
    </section>}
    <PartnerRecords category={category} version={version} detailId={detailId} onDetailClosed={() => setDetailId(null)} onChanged={() => setVersion(v => v + 1)} />
    <FormDialog open={editing} title={reviewing ? "核对" + operation : operation} description={reviewing ? "确认后立即生效。" : opening ? "本次补记金额会累加到当前余额。" : "按该单位总余额登记。"}
      documentLayout contentClassName="max-w-[680px]" loading={saving} submitText={reviewing ? "确认登记并生效" : "核对并登记"} submitDisabled={!valid || guard.open}
      cancelText={reviewing ? "返回填写" : "取消"} closeOnEscape={!guard.open && !browsing} closeOnOverlayClick={!guard.open && !browsing} trapFocus={!guard.open && !browsing}
      onCancel={() => { if (reviewing && !saving) setReviewing(false); else close(); }} onSubmit={() => { if (valid) { if (reviewing) void save(); else setReviewing(true); } }}>
      {reviewing && <div className="space-y-4" aria-label="登记核对">
        <p className="font-medium">{balance?.partnerName}</p>
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 text-sm"><dt>{amountLabel.replace("（元）", "")}</dt><dd className="text-right text-xl font-semibold tabular-nums">¥{value !== null ? money(value) : "—"}</dd><dt>业务日期</dt><dd className="text-right">{draft.date}</dd>{!opening && <><dt>方式</dt><dd className="text-right">{paymentMethodLabel[draft.method]}</dd></>}<dt>当前往来</dt><dd className="text-right">{balanceDescription(draft.direction, balance?.amount ?? "0.00")}</dd><dt>登记后</dt><dd className="text-right font-medium">{after !== null ? balanceDescription(draft.direction, after) : "—"}</dd></dl>
        {draft.remark && <p className="whitespace-pre-wrap text-sm">{opening ? "说明" : "备注"}：{draft.remark}</p>}
        {draft.transactionNo && !opening && <p className="text-sm">流水号：{draft.transactionNo}</p>}
        <p className="text-sm text-text-secondary">附件 {attachments.ids.length} 个{opening ? "；补记金额将累加，请核对是否重复录入。" : ""}</p>
      </div>}
      <div hidden={reviewing} className="space-y-3">
        <Field label={draft.direction === "CUSTOMER" ? "客户" : "供应商"} required><PartnerSelect onBrowseOpenChange={setBrowsing} inlineBrowse label={opening ? "期初往来单位" : "资金往来单位"} historical={!opening} identity={draft.direction} disabled={saving} value={Number(draft.partnerId)} onChange={p => { setDraft(d => ({ ...d, partnerId: p ? String(p.id) : "", amount: "" })); setBalance(null); setReady(false); setBalanceError(""); setOpeningCount(null); setOpeningError(""); setError(""); setShowLedger(false); }} /></Field>
        {draft.partnerId && <section aria-label="当前往来余额" className="rounded-control border border-info-border bg-info-background p-4 text-sm">
          {!ready ? balanceError ? <div role="alert"><span>{balanceError}</span> <Button size="sm" onClick={refresh}>重试余额</Button></div> : <p>正在读取余额…</p> : <>
            <div className="flex flex-wrap items-center justify-between gap-3"><div aria-live="polite"><p className="font-medium text-text-secondary">{balanceLabel}</p><p className="mt-1 text-3xl font-semibold leading-tight text-primary tabular-nums">¥{balanceAmount}</p></div>
              <Button size="sm" aria-expanded={showLedger} aria-controls="entry-ledger" onClick={() => setShowLedger(v => !v)}>{showLedger ? "收起往来明细" : "查看往来明细"}</Button></div>
            {!balance?.hasRecords && <p className="mt-1 text-text-tertiary">尚无往来记录</p>}
            {!opening && available === 0n && <p className="mt-1 text-text-secondary">{refund ? "当前没有待退款项，退货金额可能已抵减欠款。" : "当前没有可办理的欠款。"}</p>}
          </>}
          {showLedger && <PartnerEntryLedger key={`${draft.partnerId}-${draft.direction}-${version}`} partnerId={draft.partnerId} direction={draft.direction} />}
        </section>}
        <Field label={amountLabel} required htmlFor="entry-amount" error={amountError}><div className="flex gap-2"><Input id="entry-amount" aria-label={opening ? "期初金额" : "资金金额"} aria-invalid={!!amountError} inputMode="decimal" value={draft.amount} onChange={e => setDraft(d => ({ ...d, amount: e.target.value }))} placeholder="0.00" />{!opening && <Button disabled={available === null || available <= 0n} onClick={() => { if (available !== null) setDraft(d => ({ ...d, amount: money(available) })); }}>{fullLabel}</Button>}</div></Field>
        <p aria-live="polite" className="text-sm font-medium">登记后：{after !== null ? balanceDescription(draft.direction, after) : "填写有效金额后显示"}</p>
        <div className="grid gap-3 sm:grid-cols-2"><Field label="业务日期" required htmlFor="entry-date"><Input id="entry-date" aria-label={opening ? "期初业务日期" : "资金业务日期"} type="date" value={draft.date} onChange={e => setDraft(d => ({ ...d, date: e.target.value }))} /></Field>{!opening && <Field label={refund ? "退款方式" : draft.direction === "CUSTOMER" ? "收款方式" : "付款方式"} htmlFor="entry-method"><Select id="entry-method" aria-label="资金方式" value={draft.method} onChange={e => setDraft(d => ({ ...d, method: e.target.value }))}>{Object.entries(paymentMethodLabel).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</Select></Field>}</div>
        {opening && <><Field label="欠款说明" required htmlFor="entry-description" help="例如：启用系统时核对的未收款项。"><Textarea id="entry-description" aria-label="期初说明" value={draft.remark} maxLength={500} onChange={e => setDraft(d => ({ ...d, remark: e.target.value }))} /></Field>{openingCount !== null && openingCount > 0 && <p role="status" className="text-sm text-warning">已有 {openingCount} 笔未冲销期初记录，本次将继续累加，请确认没有重复录入。</p>}{openingError && <p role="alert" className="text-sm text-error">{openingError} <Button size="sm" onClick={refresh}>重试核对</Button></p>}</>}
        <details className="border-t border-border pt-3"><summary className="cursor-pointer text-sm font-medium">{opening ? "附件（选填）" : "备注、流水号与附件（选填）"}{attachments.ids.length > 0 ? " · 已上传 " + attachments.ids.length + " 个" : ""}</summary><div className="mt-3 space-y-3">{!opening && <><Field label="备注（选填）" htmlFor="entry-remark"><Textarea id="entry-remark" value={draft.remark} maxLength={500} onChange={e => setDraft(d => ({ ...d, remark: e.target.value }))} /></Field><Field label="交易流水号（选填）" htmlFor="entry-transaction"><Input id="entry-transaction" value={draft.transactionNo} maxLength={100} onChange={e => setDraft(d => ({ ...d, transactionNo: e.target.value }))} /></Field></>}<PartnerAttachmentUpload key={requestKey} kind={category} disabled={saving} onChange={setAttachments} /></div></details>
        {attachments.blocked && <p role="alert" className="text-sm text-error">附件尚未全部上传成功，请展开附件区域重试或移除失败项。</p>}
      </div>
      {error && <p role="alert" className="mt-3 text-sm text-error">{error}</p>}
    </FormDialog>
    <BusinessLeaveConfirm guard={guard} title="放弃本次登记？" stayText="继续填写" />
  </div>;
}
