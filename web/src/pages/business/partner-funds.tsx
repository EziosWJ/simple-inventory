import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { createRefund, createSettlement, partnerBalances, partnerPage, type PartnerBalance, type PartnerRecord } from "@/api/business";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cents, localToday, money } from "@/lib/partner-ledger";
import { PartnerRecords } from "./partner-records";

type Direction = "CUSTOMER" | "SUPPLIER";
type Kind = "SETTLEMENT" | "REFUND";

export function PartnerFundsPage({ kind }: { kind: Kind }) {
  const { feedback, notify } = useBusinessFeedback();
  const refund = kind === "REFUND";
  const [params] = useSearchParams();
  const [partners, setPartners] = useState<PartnerRecord[]>([]);
  const [partnerId, setPartnerId] = useState(params.get("partnerId") ?? "");
  const [direction, setDirection] = useState<Direction>(params.get("direction") === "SUPPLIER" ? "SUPPLIER" : "CUSTOMER");
  const [balance, setBalance] = useState<PartnerBalance | null>(null);
  const [balanceReady, setBalanceReady] = useState(false);
  const [amount, setAmount] = useState("");
  const [date, setDate] = useState(localToday());
  const [method, setMethod] = useState("CASH");
  const [transactionNo, setTransactionNo] = useState("");
  const [remark, setRemark] = useState("");
  const [requestKey, setRequestKey] = useState(() => crypto.randomUUID());
  const [confirming, setConfirming] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [version, setVersion] = useState(0);

  useEffect(() => {
    void partnerPage({ page: 1, pageSize: 500 }).then(result => setPartners(result.records)).catch(e => setMessage(String(e)));
  }, []);
  useEffect(() => {
    if (!partnerId) { setBalance(null); setBalanceReady(false); return; }
    let active = true;
    setBalanceReady(false);
    void partnerBalances({ partnerId, direction, page: 1, pageSize: 1 }).then(result => {
      if (active) { setBalance(result.records[0] ?? null); setBalanceReady(true); }
    }).catch(e => { if (active) setMessage(e instanceof Error ? e.message : "读取当前余额失败"); });
    return () => { active = false; };
  }, [partnerId, direction, version]);

  const current = cents(balance?.amount ?? "0.00");
  const value = cents(amount);
  const valid = !!partnerId && balanceReady && current !== null && value !== null && value > 0n &&
    (refund ? current < 0n && value <= -current : current > 0n && value <= current);
  const after = valid && current !== null && value !== null ? money(current + (refund ? value : -value)) : "—";
  const operation = refund
    ? direction === "CUSTOMER" ? "向客户退款" : "收到供应商退款"
    : direction === "CUSTOMER" ? "客户收款" : "供应商付款";

  async function save() {
    if (!valid) return;
    setSaving(true);
    setMessage("");
    try {
      const saved = await (refund ? createRefund : createSettlement)({ requestKey, partnerId: Number(partnerId), direction, amount, businessDate: date, paymentMethod: method, transactionNo, remark });
      setConfirming(false);
      setAmount("");
      setTransactionNo("");
      setRemark("");
      setRequestKey(crypto.randomUUID());
      notify({ type: "success", title: `${operation}成功`, description: `${saved.partnerName ?? balance?.partnerName ?? partners.find(p => p.id === saved.partnerId)?.name ?? "往来单位"} · ${saved.direction === "CUSTOMER" ? "客户方向" : "供应商方向"}余额 ${saved.balanceBefore} → ${saved.balanceAfter} 元。`, documentNo: saved.documentNo, status: "已生效" });
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
    <PageHeader title={refund ? "退款" : "收付款"} description={refund ? "待退款时记录实际退还客户或收到供应商的款项。退款不绑定单张退货单。" : "按往来单位的当前总欠款记录客户收款或供应商付款。"} />
    <BusinessFeedback feedback={feedback} />
    <section className="rounded-admin border border-border bg-surface p-4" aria-label={refund ? "办理退款" : "办理收付款"}>
      <h2 className="mb-3 font-medium">{operation}</h2>
      <div className="grid gap-3 md:grid-cols-3">
        <label className="text-sm">方向<Select className="mt-1" aria-label="资金方向" value={direction} onChange={e => { setDirection(e.target.value as Direction); setPartnerId(""); setBalance(null); setBalanceReady(false); setAmount(""); }}><option value="CUSTOMER">{refund ? "向客户退款" : "客户收款"}</option><option value="SUPPLIER">{refund ? "收到供应商退款" : "供应商付款"}</option></Select></label>
        <label className="text-sm">往来单位<Select className="mt-1" aria-label="资金往来单位" value={partnerId} onChange={e => { setPartnerId(e.target.value); setBalance(null); setBalanceReady(false); setAmount(""); }}><option value="">选择往来单位</option>{partners.map(partner => <option key={partner.id} value={partner.id}>{partner.name}（{partner.code}）</option>)}</Select></label>
        <label className="text-sm">金额（元）<Input className="mt-1" aria-label="资金金额" inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} placeholder="0.00" /></label>
        <label className="text-sm">业务日期<Input className="mt-1" aria-label="资金业务日期" type="date" value={date} onChange={e => setDate(e.target.value)} /></label>
        <label className="text-sm">方式<Select className="mt-1" aria-label="资金方式" value={method} onChange={e => setMethod(e.target.value)}><option value="CASH">现金</option><option value="WECHAT">微信</option><option value="ALIPAY">支付宝</option><option value="BANK_TRANSFER">银行转账</option><option value="OTHER">其他</option></Select></label>
        <label className="text-sm">交易流水号（选填）<Input className="mt-1" value={transactionNo} onChange={e => setTransactionNo(e.target.value)} /></label>
        <label className="text-sm md:col-span-3">备注（选填）<Textarea className="mt-1" value={remark} onChange={e => setRemark(e.target.value)} /></label>
      </div>
      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
        <p className="text-sm">当前余额：<strong>{balance?.amount ?? "0.00"} 元</strong>；确认后：<strong>{after} 元</strong></p>
        <Button disabled={!valid || saving} onClick={() => setConfirming(true)}>确认{operation}</Button>
      </div>
      {partnerId && !valid && !amount && <p className="mt-2 text-sm text-text-secondary">{refund ? "当前没有可退余额时不能办理退款；退货可能已抵减原有欠款。" : "当前没有正欠款时不能办理收付款。"}</p>}
      {message && <p role="alert" className="mt-2 text-sm text-error">{message}</p>}
    </section>
    <PartnerRecords category={kind} version={version} onChanged={() => { setBalanceReady(false); setVersion(value => value + 1); }} />
    <ConfirmDialog open={confirming} title={`确认${operation}`} description={balance ? `${balance.partnerName} · ${amount} 元 · 余额 ${balance.amount} → ${after} 元 · ${method}` : undefined} confirmText={`确认${operation}`} loading={saving} onCancel={() => setConfirming(false)} onConfirm={() => void save()} />
  </div>;
}
