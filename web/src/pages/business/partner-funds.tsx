import { randomUUID } from "@/lib/uuid";
import { PartnerAttachmentUpload } from "@/components/business/partner-attachments";
import { emptyAttachments } from "@/lib/partner-attachments";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { createRefund, createSettlement, partnerBalances, type PartnerBalance } from "@/api/business";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { Field } from "@/components/common/field";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cents, localToday, money } from "@/lib/partner-ledger";
import { PartnerRecords } from "./partner-records";
import { BusinessLeaveConfirm } from "@/components/business/business-leave-confirm";
import { useBusinessLeaveGuard } from "@/hooks/use-business-leave-guard";

import { PartnerSelect } from "@/components/business/master-data-select";
import { businessReturnTo } from "@/lib/business-navigation";

type Direction = "CUSTOMER" | "SUPPLIER";
type Kind = "SETTLEMENT" | "REFUND";

function formSignature(value: { partnerId: string; direction: Direction; amount: string; date: string; method: string; transactionNo: string; remark: string }) {
  return JSON.stringify([value.partnerId, value.direction, value.amount, value.date, value.method, value.transactionNo, value.remark]);
}

export function PartnerFundsPage({ kind }: { kind: Kind }) {
  const { feedback, notify } = useBusinessFeedback();
  const refund = kind === "REFUND";
  const [params] = useSearchParams();
  const [initial] = useState(() => ({
    partnerId: params.get("partnerId") ?? "",
    direction: (params.get("direction") === "SUPPLIER" ? "SUPPLIER" : "CUSTOMER") as Direction,
    date: localToday(),
  }));
  const navigate = useNavigate();
  const returnTo = businessReturnTo(params.get("returnTo"), "/business/partner-balances");
  const amountInput = useRef<HTMLInputElement>(null);
  const [receipt, setReceipt] = useState<{ operation: string } | null>(null);
  const [balanceError, setBalanceError] = useState("");
  const [partnerId, setPartnerId] = useState(initial.partnerId);
  const [direction, setDirection] = useState<Direction>(initial.direction);
  const [balance, setBalance] = useState<PartnerBalance | null>(null);
  const [balanceReady, setBalanceReady] = useState(false);
  const [amount, setAmount] = useState("");
  const [date, setDate] = useState(initial.date);
  const [method, setMethod] = useState("CASH");
  const [transactionNo, setTransactionNo] = useState("");
  const [remark, setRemark] = useState("");
  const [attachments, setAttachments] = useState(emptyAttachments);
  const [requestKey, setRequestKey] = useState(() => randomUUID());
  const [confirming, setConfirming] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [version, setVersion] = useState(0);
  const [baseline, setBaseline] = useState(() => formSignature({ ...initial, amount: "", method: "CASH", transactionNo: "", remark: "" }));
  const dirty = attachments.dirty || formSignature({ partnerId, direction, amount, date, method, transactionNo, remark }) !== baseline;
  const leaveGuard = useBusinessLeaveGuard({ dirty, busy: saving });

  useEffect(() => {
    if (!partnerId) { setBalance(null); setBalanceReady(false); return; }
    let active = true;
    setBalanceReady(false);
    setBalanceError("");
    void partnerBalances({ partnerId, direction, page: 1, pageSize: 1 }).then(result => {
      if (active) { setBalance(result.records[0] ?? null); setBalanceReady(true); }
    }).catch(e => { if (active) setBalanceError(e instanceof Error ? e.message : "读取当前余额失败"); });
    return () => { active = false; };
  }, [partnerId, direction, version]);

  const current = balanceReady ? cents(balance?.amount ?? "0.00") : null;
  const value = cents(amount);
  const valid = !!partnerId && balanceReady && current !== null && value !== null && value > 0n &&
    (refund ? current < 0n && value <= -current : current > 0n && value <= current);
  const after = valid && current !== null && value !== null ? money(current + (refund ? value : -value)) : "—";
  const operation = refund
    ? direction === "CUSTOMER" ? "向客户退款" : "收到供应商退款"
    : direction === "CUSTOMER" ? "客户收款" : "供应商付款";

  const available = current === null ? null : refund ? (current < 0n ? -current : 0n) : (current > 0n ? current : 0n);
  const amountError = amount && (value === null || value <= 0n)
    ? "请输入大于零的金额，最多两位小数。"
    : amount && available !== null && value !== null && value > available
      ? `金额超过当前可办理金额 ${money(available)} 元，请核对后修改。` : "";
  const currentLabel = refund ? direction === "CUSTOMER" ? "应退客户款" : "应收供应商退款" : direction === "CUSTOMER" ? "客户应收" : "供应商应付";

  async function save() {
    if (!valid || attachments.blocked) return;
    setSaving(true);
    setMessage("");
    try {
      const saved = await (refund ? createRefund : createSettlement)({ attachmentIds: attachments.ids, requestKey, partnerId: Number(partnerId), direction, amount, businessDate: date, paymentMethod: method, transactionNo, remark });
      setConfirming(false);
      setReceipt({ operation });
      setAmount("");
      setTransactionNo("");
      setRemark("");
      setBaseline(formSignature({ partnerId, direction, amount: "", date, method, transactionNo: "", remark: "" }));
      setAttachments(emptyAttachments);
      setRequestKey(randomUUID());
      notify({ type: "success", title: `${operation}成功`, description: `本次金额 ${amount} 元；${saved.partnerName ?? balance?.partnerName ?? "往来单位"} · ${saved.direction === "CUSTOMER" ? "客户方向" : "供应商方向"}余额 ${saved.balanceBefore} → ${saved.balanceAfter} 元。`, documentNo: saved.documentNo, status: "已生效" });
      setBalanceReady(false);
      setVersion(value => value + 1);
    } catch (e) {
      setConfirming(false);
      const message = e instanceof Error ? e.message : "保存失败，请核对当前余额"; setMessage(message); notify({ type: "error", title: "保存失败", description: message });
      setBalanceReady(false);
      setVersion(value => value + 1);
    } finally {
      setSaving(false);
    }
  }

  return <div className="space-y-4">
    <PageHeader title={refund ? "退款" : "收付款"} description={refund ? "待退款时记录实际退还客户或收到供应商的款项。退款不绑定单张退货单。" : "按往来单位的当前总欠款记录客户收款或供应商付款。"} actions={<Button onClick={() => navigate(returnTo)}>{returnTo.startsWith("/business/partner-balances") ? "返回往来余额" : "返回来源页面"}</Button>} />
    <BusinessFeedback feedback={feedback} />
    {receipt && <section aria-label="款项办理结果" className="flex flex-wrap items-center justify-between gap-3 rounded-admin border border-border bg-surface p-4">
      <p className="text-sm">{receipt.operation}已生效；最新余额：<strong className="tabular-nums">{balanceReady && balance ? balance.amount : balanceError ? "读取失败，请重试" : "读取中…"}{balanceReady && balance ? " 元" : ""}</strong></p>
      <div className="flex gap-2"><Button variant="primary" onClick={() => navigate(returnTo)}>{returnTo.startsWith("/business/partner-balances") ? "返回往来余额" : "返回来源页面"}</Button><Button onClick={() => { setReceipt(null); amountInput.current?.focus(); }}>继续登记</Button></div>
    </section>}
    <section className="rounded-admin border border-border bg-surface p-4" aria-label={refund ? "办理退款" : "办理收付款"}>
      <h2 className="mb-3 font-medium">{operation}</h2>
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        <Field label="方向" htmlFor="funds-direction"><Select id="funds-direction" disabled={saving} aria-label="资金方向" value={direction} onChange={e => { setDirection(e.target.value as Direction); setPartnerId(""); setBalance(null); setBalanceReady(false); setAmount(""); setReceipt(null); }}><option value="CUSTOMER">{refund ? "向客户退款" : "客户收款"}</option><option value="SUPPLIER">{refund ? "收到供应商退款" : "供应商付款"}</option></Select></Field>
        <Field label="资金往来单位"><PartnerSelect inlineBrowse label="资金往来单位" historical identity={direction} disabled={saving} value={Number(partnerId)} onChange={p => { setPartnerId(p ? String(p.id) : ""); setBalance(null); setBalanceReady(false); setAmount(""); setReceipt(null); }} /></Field>
        <Field label="金额（元）" htmlFor="funds-amount"><div className="flex gap-2"><Input id="funds-amount" ref={amountInput} disabled={saving} aria-label="资金金额" aria-invalid={!!amountError} aria-describedby={amountError ? "funds-amount-error" : undefined} inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} placeholder="0.00" /><Button disabled={saving || available === null || available <= 0n} onClick={() => { if (available !== null) setAmount(money(available)); }}>填入全额</Button></div>{amountError && <p id="funds-amount-error" role="alert" className="mt-1 text-error">{amountError}</p>}</Field>
        <Field label="业务日期" htmlFor="funds-date"><Input id="funds-date" disabled={saving} aria-label="资金业务日期" type="date" value={date} onChange={e => setDate(e.target.value)} /></Field>
        <Field label="方式" htmlFor="funds-method"><Select id="funds-method" disabled={saving} aria-label="资金方式" value={method} onChange={e => setMethod(e.target.value)}><option value="CASH">现金</option><option value="WECHAT">微信</option><option value="ALIPAY">支付宝</option><option value="BANK_TRANSFER">银行转账</option><option value="OTHER">其他</option></Select></Field>
        <Field label="交易流水号（选填）" htmlFor="funds-transaction"><Input id="funds-transaction" disabled={saving} value={transactionNo} onChange={e => setTransactionNo(e.target.value)} /></Field>
        <div className="md:col-span-2 xl:col-span-3"><Field label="备注（选填）" htmlFor="funds-remark"><Textarea id="funds-remark" disabled={saving} value={remark} onChange={e => setRemark(e.target.value)} /></Field></div>
      </div>
      <PartnerAttachmentUpload key={requestKey} kind={kind} disabled={saving} onChange={setAttachments} />
      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
        <p className="text-sm">{currentLabel}：<strong>{!partnerId ? "请先选择单位" : !balanceReady ? balanceError ? "读取失败" : "读取中…" : `${money(available ?? 0n)} 元`}</strong>；确认后余额：<strong>{after}{valid ? " 元" : ""}</strong></p>
        <Button disabled={!valid || saving || attachments.blocked} onClick={() => setConfirming(true)}>确认{operation}</Button>
      </div>
      {partnerId && balanceReady && available === 0n && <p className="mt-2 text-sm text-text-secondary">{refund ? "当前没有可退余额时不能办理退款；退货可能已抵减原有欠款。" : "当前没有正欠款时不能办理收付款。"}</p>}
      {partnerId && balanceError && <div role="alert" className="mt-2 flex items-center gap-3 text-sm text-error"><span>{balanceError}</span><Button disabled={saving} onClick={() => setVersion(v => v + 1)}>重试余额</Button></div>}
      {message && <p role="alert" className="mt-2 text-sm text-error">{message}</p>}
    </section>
    <PartnerRecords category={kind} version={version} onChanged={() => { setBalanceReady(false); setVersion(value => value + 1); }} />
    <ConfirmDialog suspended={leaveGuard.open} open={confirming} title={`确认${operation}`} description={balance ? `${balance.partnerName} · ${amount} 元 · 余额 ${balance.amount} → ${after} 元 · ${method}` : undefined} confirmText={`确认${operation}`} loading={saving} onCancel={() => setConfirming(false)} onConfirm={() => void save()} />
    <BusinessLeaveConfirm guard={leaveGuard} title={`离开${refund ? "退款" : "收付款"}`} stayText="继续填写" leaveText="放弃并离开" />
  </div>;
}
