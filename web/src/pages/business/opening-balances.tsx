import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { createOpeningBalance, partnerBalances, partnerPage, type PartnerBalance, type PartnerRecord } from "@/api/business";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cents, localToday, money } from "@/lib/partner-ledger";
import { PartnerRecords } from "./partner-records";

type Direction = "CUSTOMER" | "SUPPLIER";

export function OpeningBalancesPage() {
  const [params] = useSearchParams();
  const [partners, setPartners] = useState<PartnerRecord[]>([]);
  const [partnerId, setPartnerId] = useState(params.get("partnerId") ?? "");
  const [direction, setDirection] = useState<Direction>(params.get("direction") === "SUPPLIER" ? "SUPPLIER" : "CUSTOMER");
  const [balance, setBalance] = useState<PartnerBalance | null>(null);
  const [balanceReady, setBalanceReady] = useState(false);
  const [amount, setAmount] = useState("");
  const [date, setDate] = useState(localToday());
  const [description, setDescription] = useState("");
  const [requestKey, setRequestKey] = useState(() => crypto.randomUUID());
  const [confirming, setConfirming] = useState(false);
  const [saving, setSaving] = useState(false);
  const [version, setVersion] = useState(0);
  const [message, setMessage] = useState("");

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

  const before = cents(balance?.amount ?? "0.00");
  const value = cents(amount);
  const valid = !!partnerId && balanceReady && before !== null && value !== null && value > 0n && !!description.trim();
  const after = valid && before !== null && value !== null ? money(before + value) : "—";

  async function save() {
    if (!valid) return;
    setSaving(true);
    setMessage("");
    try {
      await createOpeningBalance({ requestKey, partnerId: Number(partnerId), direction, amount, businessDate: date, description: description.trim() });
      setAmount("");
      setDescription("");
      setRequestKey(crypto.randomUUID());
      setConfirming(false);
      setBalanceReady(false);
      setVersion(value => value + 1);
      setMessage("期初金额已生效");
    } catch (e) {
      setConfirming(false);
      setBalanceReady(false);
      setVersion(value => value + 1);
      setMessage(e instanceof Error ? e.message : "保存失败，请检查期初金额");
    } finally {
      setSaving(false);
    }
  }

  return <div className="space-y-4">
    <PageHeader title="期初录入" description="补录启用系统前的应收或应付。不补造历史销售、采购单；录错在原记录详情中冲销。" />
    <section className="rounded-admin border border-border bg-surface p-4" aria-label="录入期初应收应付">
      <h2 className="mb-3 font-medium">录入期初应收 / 应付</h2>
      <div className="grid gap-3 md:grid-cols-3">
        <label className="text-sm">方向<Select className="mt-1" aria-label="期初方向" value={direction} onChange={e => { setDirection(e.target.value as Direction); setPartnerId(""); setBalance(null); setBalanceReady(false); setAmount(""); }}><option value="CUSTOMER">期初应收</option><option value="SUPPLIER">期初应付</option></Select></label>
        <label className="text-sm">往来单位<Select className="mt-1" aria-label="期初往来单位" value={partnerId} onChange={e => { setPartnerId(e.target.value); setBalance(null); setBalanceReady(false); setAmount(""); }}><option value="">选择往来单位</option>{partners.filter(partner => direction === "CUSTOMER" ? partner.isCustomer : partner.isSupplier).map(partner => <option key={partner.id} value={partner.id}>{partner.name}（{partner.code}）</option>)}</Select></label>
        <label className="text-sm">金额（元）<Input className="mt-1" aria-label="期初金额" inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} placeholder="0.00" /></label>
        <label className="text-sm">业务日期<Input className="mt-1" aria-label="期初业务日期" type="date" value={date} onChange={e => setDate(e.target.value)} /></label>
        <label className="text-sm md:col-span-2">说明（必填）<Textarea className="mt-1" aria-label="期初说明" value={description} onChange={e => setDescription(e.target.value)} /></label>
      </div>
      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
        <p className="text-sm">当前余额：<strong>{balance?.amount ?? "0.00"} 元</strong>；确认后：<strong>{after} 元</strong></p>
        <Button disabled={!valid || saving} onClick={() => setConfirming(true)}>确认录入期初</Button>
      </div>
      {message && <p role="status" className="mt-2 text-sm text-text-secondary">{message}</p>}
    </section>
    <PartnerRecords category="OPENING" version={version} onChanged={() => { setBalanceReady(false); setVersion(value => value + 1); }} />
    <ConfirmDialog open={confirming} title="确认录入期初" description={balance ? `${balance.partnerName} · ${direction === "CUSTOMER" ? "应收" : "应付"} ${amount} 元 · 余额 ${balance.amount} → ${after} 元` : `期初金额 ${amount} 元 · 余额 0.00 → ${after} 元`} confirmText="保存并生效" loading={saving} onCancel={() => setConfirming(false)} onConfirm={() => void save()} />
  </div>;
}
