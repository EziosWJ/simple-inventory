import { formatDateTime } from "@/lib/datetime";
import { PartnerAttachmentDetails } from "@/components/business/partner-attachments";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  partnerBalanceEntries,
  partnerBalanceEntry,
  reversePartnerEntry,
  partnerBalances,
  type PartnerBalanceEntry,
} from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { PartnerSelect } from "@/components/business/master-data-select";
import { Textarea } from "@/components/ui/textarea";
import { balanceDescription, cents, entryTypeLabel, money, paymentMethodLabel } from "@/lib/partner-ledger";
import type { DataTableColumn } from "@/types";

export type RecordCategory = "OPENING" | "SETTLEMENT" | "REFUND";
const pageSize = 20;
type Filters = { partnerId: string; partnerKeyword: string; direction: string; documentNo: string; reversalStatus: string; businessFrom: string; businessTo: string };
const blankFilters: Filters = { partnerId: "", partnerKeyword: "", direction: "", documentNo: "", reversalStatus: "", businessFrom: "", businessTo: "" };

function absoluteAmount(value: string) {
  const parsed = cents(value);
  return parsed === null ? value : money(parsed < 0n ? -parsed : parsed);
}

function belongsToCategory(entry: PartnerBalanceEntry, category: RecordCategory) {
  if (entry.entryType === "REVERSAL" && entry.reversesId) return true;
  if (category === "OPENING") return entry.entryType === "OPENING";
  if (category === "SETTLEMENT") return entry.entryType === "RECEIPT" || entry.entryType === "PAYMENT";
  return entry.entryType === "CUSTOMER_REFUND" || entry.entryType === "SUPPLIER_REFUND";
}

export function PartnerRecords({ category, version = 0, onChanged, detailId, onDetailClosed }: { category: RecordCategory; version?: number; onChanged?: () => void; detailId?: number | null; onDetailClosed?: () => void }) {
  const { feedback, notify } = useBusinessFeedback();
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const [draftFilters, setDraftFilters] = useState<Filters>(() => ({ ...blankFilters, partnerId: params.get("partnerId") ?? "", direction: params.get("direction") ?? "" }));
  const [filters, setFilters] = useState<Filters>(() => ({ ...blankFilters, partnerId: params.get("partnerId") ?? "", direction: params.get("direction") ?? "" }));
  const [page, setPage] = useState(1);
  const [rows, setRows] = useState<PartnerBalanceEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [detail, setDetail] = useState<PartnerBalanceEntry | null>(null);
  const [reverseTarget, setReverseTarget] = useState<PartnerBalanceEntry | null>(null);
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [revision, setRevision] = useState(0);
  const [reverseBalance, setReverseBalance] = useState<string | null>(null);
  const [reverseError, setReverseError] = useState("");
  const [reverseRevision, setReverseRevision] = useState(0);

  useEffect(() => {
    const id = detailId ?? Number(params.get("entryId"));
    let active = true;
    if (id > 0) void partnerBalanceEntry(id).then(entry => {
      if (!active) return;
      if (belongsToCategory(entry, category)) setDetail(entry);
      else setError("该记录不属于当前业务页");
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : "加载记录失败"); });
    return () => { active = false; };
  }, [params, category, detailId]);

  useEffect(() => {
    if (!reverseTarget) return;
    let active = true;
    void partnerBalances({ partnerId: reverseTarget.partnerId, direction: reverseTarget.direction, page: 1, pageSize: 1 })
      .then(result => { if (active) setReverseBalance(result.records[0]?.amount ?? "0.00"); })
      .catch(e => { if (active) setReverseError(e instanceof Error ? e.message : "读取当前余额失败"); });
    return () => { active = false; };
  }, [reverseTarget, reverseRevision]);

  function closeDetail() {
    setDetail(null); onDetailClosed?.();
    if (params.has("entryId")) { const next = new URLSearchParams(params); next.delete("entryId"); setParams(next, { replace: true }); }
  }
  function openRelated(id: number) {
    void partnerBalanceEntry(id).then(setDetail).catch(e => setError(e instanceof Error ? e.message : "加载关联记录失败"));
  }
  const reverseCurrent = reverseBalance === null ? null : cents(reverseBalance);
  const reverseAmount = reverseTarget ? cents(reverseTarget.amount) : null;
  const reverseAfter = reverseCurrent !== null && reverseAmount !== null ? money(reverseCurrent - reverseAmount) : null;

  const load = useCallback(() => {
    let active = true;
    setLoading(true);
    void partnerBalanceEntries({ category, partnerId: filters.partnerId || 0, partnerKeyword: filters.partnerKeyword, direction: filters.direction, documentNo: filters.documentNo, reversalStatus: filters.reversalStatus, businessFrom: filters.businessFrom, businessTo: filters.businessTo, page, pageSize }).then(result => {
      if (active) { setRows(result.records); setTotal(result.total); setError(""); }
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : "读取记录失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [category, filters, page]);
  useEffect(load, [load, version, revision]);

  async function reverse() {
    if (!reverseTarget || !reason.trim() || reverseAfter === null || saving) return;
    setSaving(true);
    setMessage("");
    try {
      const reversed = await reversePartnerEntry(reverseTarget.id, reason.trim());
      setReverseTarget(null);
      setReason("");
      notify({ type: "success", title: "往来记录冲销成功", description: `${reverseTarget.partnerName ?? "往来单位"}，${balanceDescription(reversed.direction, reversed.balanceAfter)}。原记录和冲销记录均保留在往来明细中。`, documentNo: reversed.documentNo, status: "已生效" });
      if (onChanged) onChanged();
      else setRevision(value => value + 1);
    } catch (e) {
      const message = e instanceof Error ? e.message : "冲销失败，请核对当前余额"; setMessage(message); notify({ type: "error", title: "往来冲销失败", description: message });
      setReverseBalance(null); setReverseError(""); setReverseRevision(v => v + 1);
    } finally {
      setSaving(false);
    }
  }

  const columns: DataTableColumn<PartnerBalanceEntry>[] = [
    { title: "单号", dataIndex: "documentNo" },
    { title: "往来单位", dataIndex: "partnerName" },
    { title: "业务", key: "type", render: (_, row) => entryTypeLabel[row.entryType] ?? row.entryType },
    { title: "业务日期", dataIndex: "businessDate" },
    { title: "金额（元）", key: "amount", align: "right", render: (_, row) => absoluteAmount(row.amount) },
    { title: "本笔变动后", key: "balance", render: (_, row) => balanceDescription(row.direction, row.balanceAfter) },
    { title: "附件", key: "attachments", render: (_, row) => `${row.attachmentCount ?? 0} 个` },
    { title: "状态", key: "status", render: (_, row) => row.entryType === "REVERSAL" ? "冲销记录" : row.reversedById ? "已冲销" : "已生效" },
    { title: "操作", key: "action", render: (_, row) => <Button size="sm" variant="secondary" onClick={() => void partnerBalanceEntry(row.id).then(setDetail).catch(e => setError(String(e)))}>查看详情</Button> },
  ];
  function applyFilters(next: Filters) {
    if (next.businessFrom && next.businessTo && next.businessFrom > next.businessTo) { setError("业务日期起始不能晚于结束日期。"); return; }
    setError("");
    setFilters({ ...next, partnerKeyword: next.partnerKeyword.trim(), documentNo: next.documentNo.trim(), partnerId: next.partnerKeyword.trim() ? "" : next.partnerId });
    setPage(1);
  }

  return <>
    <BusinessFeedback feedback={feedback} />
    <SearchFilterBar onSubmit={() => applyFilters(draftFilters)} actions={<><Button onClick={() => applyFilters(draftFilters)}>查询</Button><Button variant="secondary" onClick={() => setRevision(value => value + 1)}>刷新</Button><Button variant="secondary" onClick={() => { setDraftFilters(blankFilters); applyFilters(blankFilters); }}>重置</Button></>}>
      <Field label="往来单位"><PartnerSelect inlineBrowse label="筛选往来单位" historical value={Number(draftFilters.partnerId)} keyword={draftFilters.partnerKeyword} onDraftKeywordChange={keyword => setDraftFilters(current => ({ ...current, partnerKeyword: keyword, partnerId: "" }))} onKeywordChange={keyword => { const next = { ...draftFilters, partnerKeyword: keyword, partnerId: "" }; setDraftFilters(next); applyFilters(next); }} onChange={partner => setDraftFilters(current => ({ ...current, partnerId: partner ? String(partner.id) : "", partnerKeyword: "" }))} /></Field>
      <Field label="往来方向"><Select aria-label="筛选往来方向" value={draftFilters.direction} onChange={e => setDraftFilters({ ...draftFilters, direction: e.target.value })}>
        <option value="">客户及供应商</option><option value="CUSTOMER">客户</option><option value="SUPPLIER">供应商</option>
      </Select></Field>
      <Field label="往来单号"><Input placeholder="往来单号" value={draftFilters.documentNo} onChange={e => setDraftFilters({ ...draftFilters, documentNo: e.target.value })} /></Field>
      <Field label="冲销状态"><Select aria-label="冲销状态" value={draftFilters.reversalStatus} onChange={e => setDraftFilters({ ...draftFilters, reversalStatus: e.target.value })}><option value="">全部冲销状态</option><option value="EFFECTIVE">未冲销</option><option value="REVERSED">原记录已冲销</option><option value="REVERSAL">冲销记录</option></Select></Field>
      <Field label="业务日期起"><Input type="date" value={draftFilters.businessFrom} onChange={e => setDraftFilters({ ...draftFilters, businessFrom: e.target.value })} /></Field>
      <Field label="业务日期止"><Input type="date" value={draftFilters.businessTo} onChange={e => setDraftFilters({ ...draftFilters, businessTo: e.target.value })} /></Field>
    </SearchFilterBar>
    {message && <p role="alert" className="mb-3 text-sm text-error">{message}</p>}
    <DataTableCard toolbar={<div className="p-3 text-sm">已生效记录 · 共 {total} 笔</div>} pagination={<Pagination page={page} pageSize={pageSize} total={total} onPageChange={setPage} />}>
      <DataTable columns={columns} dataSource={rows} rowKey="id" loading={loading} error={error} empty={<p className="text-sm text-text-secondary">当前条件下暂无记录，可清除筛选或点击页面右上方按钮登记。</p>} />
    </DataTableCard>
    <DetailDialog fixedFooter trapFocus contentClassName="max-w-[760px]" footer={<div className="flex flex-wrap items-center justify-between gap-2"><BusinessReturnLink /><div className="flex flex-wrap gap-2"><Button onClick={() => { if (detail) navigate(`/business/partner-ledger?partnerId=${detail.partnerId}&direction=${detail.direction}`); }}>查看往来明细</Button>{detail && !detail.reversedById && detail.entryType !== "REVERSAL" && <Button onClick={() => { setReverseTarget(detail); setReverseBalance(null); setReverseError(""); setReason(""); setMessage(""); closeDetail(); }}>冲销此记录</Button>}<Button onClick={closeDetail}>关闭</Button></div></div>} open={detail !== null} title={detail ? `${entryTypeLabel[detail.entryType]} · ${detail.documentNo}` : "记录详情"} onCancel={closeDetail}>
      {detail && <div className="space-y-4 text-sm">
        <section aria-label="往来对象与状态" className="rounded-control border border-border bg-neutral-background p-3">
          <p className="font-medium">{detail.partnerName}</p>
          <p className="mt-1 text-text-secondary">{detail.direction === "CUSTOMER" ? "客户方向" : "供应商方向"} · {detail.entryType === "REVERSAL" ? "冲销记录" : detail.reversedById ? "原记录已冲销" : "已生效"}</p>
        </section>
        <section aria-label="金额影响" className="grid gap-3 rounded-control border border-border p-3 sm:grid-cols-3">
          <div><p className="text-text-tertiary">{detail.entryType === "RECEIPT" ? "实际收到" : detail.entryType === "PAYMENT" ? "实际支付" : detail.entryType === "CUSTOMER_REFUND" ? "实际退给客户" : detail.entryType === "SUPPLIER_REFUND" ? "实际收到退款" : detail.entryType === "OPENING" ? "本次补记欠款" : "本次冲销金额"}</p><p className="mt-1 font-medium tabular-nums">¥{absoluteAmount(detail.amount)}</p>{detail.entryType === "REVERSAL" && <p className="text-text-secondary">{cents(detail.amount)! < 0n ? "减少" : "增加"}该方向往来余额</p>}</div>
          <div><p className="text-text-tertiary">本笔生效前</p><p className="mt-1 font-medium tabular-nums">{balanceDescription(detail.direction, detail.balanceBefore)}</p></div>
          <div><p className="text-text-tertiary">本笔生效后</p><p className="mt-1 font-medium tabular-nums">{balanceDescription(detail.direction, detail.balanceAfter)}</p></div>
        </section>
        <section aria-label="业务信息" className="space-y-2 rounded-control border border-border p-3">
          <p>业务日期：{detail.businessDate}</p>
          <p>实际生效：{formatDateTime(detail.effectiveAt)}</p>
          <p>操作人：{detail.operatorName ?? detail.operatorId}</p>
          {detail.paymentMethod && <p>{detail.entryType.endsWith("_REFUND") ? "退款方式" : detail.direction === "CUSTOMER" ? "收款方式" : "付款方式"}：{paymentMethodLabel[detail.paymentMethod] ?? detail.paymentMethod}{detail.transactionNo ? ` · 流水号 ${detail.transactionNo}` : ""}</p>}
          {detail.reversesId && <p>冲销的原记录：<Button size="sm" variant="ghost" onClick={() => openRelated(detail.reversesId!)}>{detail.reversedDocumentNo || "查看原记录"}</Button></p>}
          {detail.reversedById && <p>关联冲销：<Button size="sm" variant="ghost" onClick={() => openRelated(detail.reversedById!)}>查看冲销记录</Button></p>}
        </section>
        {detail.description && <section aria-label="说明" className="rounded-control border border-border p-3"><p>说明：{detail.description}</p></section>}
        {detail.entryType !== "REVERSAL" && <PartnerAttachmentDetails key={detail.id} entryId={detail.id} kind={category} readOnly={!!detail.reversedById} onChanged={() => { setRevision(value => value + 1); void partnerBalanceEntry(detail.id).then(setDetail).catch(e => setError(String(e))); }} />}
        <div className="flex flex-wrap items-center gap-3 border-t border-border pt-3">
          {detail.reversedById ? <p className="text-text-secondary">本记录已冲销，原记录和凭证保留。</p> : detail.entryType === "REVERSAL" ? <p className="text-text-secondary">这是冲销记录，不能再次冲销。凭证保留在原记录中。</p> : <p className="text-text-secondary">录错时可冲销后重新登记。冲销只纠正账面记录，不代表实际退钱。</p>}
        </div>
      </div>}
    </DetailDialog>
    <FormDialog open={reverseTarget !== null} title="冲销往来记录" description={reverseTarget ? `${reverseTarget.partnerName} · ${reverseTarget.documentNo}` : undefined} submitText="确认冲销" loading={saving} submitDisabled={!reason.trim() || reverseAfter === null} onCancel={() => { if (!saving) { setDetail(reverseTarget); setReverseTarget(null); setReason(""); } }} onSubmit={reverse}>
      <div className="mb-4 space-y-2 text-sm"><p>冲销将抵消本笔 {reverseTarget ? entryTypeLabel[reverseTarget.entryType] : ""} ¥{reverseTarget ? absoluteAmount(reverseTarget.amount) : "—"}，原记录保留。</p>{reverseTarget && reverseBalance !== null && <p>当前：{balanceDescription(reverseTarget.direction, reverseBalance)}</p>}{reverseTarget && reverseAfter !== null && <p className="font-medium">预计冲销后：{balanceDescription(reverseTarget.direction, reverseAfter)}</p>}{reverseBalance === null && !reverseError && <p>正在核对当前余额…</p>}{reverseError && <p role="alert" className="text-error">{reverseError} <Button size="sm" onClick={() => { setReverseError(""); setReverseRevision(v => v + 1); }}>重试核对</Button></p>}<p className="text-text-secondary">实际结果以提交时的最新余额为准。冲销用于纠正录错，不会实际收钱或退钱。</p>{reverseTarget?.entryType === "OPENING" && reverseAfter !== null && cents(reverseAfter)! < 0n && <p className="text-warning">冲销后会形成待退款，当前余额不足以冲销这笔期初欠款；提交将由系统拒绝。</p>}</div>
      <label className="block text-sm">冲销原因（必填）<Textarea className="mt-2" value={reason} onChange={e => setReason(e.target.value)} maxLength={500} /></label>
      {message && <p role="alert" className="mt-2 text-sm text-error">{message}</p>}
    </FormDialog>
  </>;
}
