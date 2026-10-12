import { formatDateTime } from "@/lib/datetime";
import { PartnerAttachmentDetails } from "@/components/business/partner-attachments";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  partnerBalanceEntries,
  partnerBalanceEntry,
  reversePartnerEntry,
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
import { balanceState, entryTypeLabel } from "@/lib/partner-ledger";
import type { DataTableColumn } from "@/types";

export type RecordCategory = "OPENING" | "SETTLEMENT" | "REFUND";
const pageSize = 20;
type Filters = { partnerId: string; partnerKeyword: string; direction: string; documentNo: string; reversalStatus: string; businessFrom: string; businessTo: string };
const blankFilters: Filters = { partnerId: "", partnerKeyword: "", direction: "", documentNo: "", reversalStatus: "", businessFrom: "", businessTo: "" };

function belongsToCategory(entry: PartnerBalanceEntry, category: RecordCategory) {
  if (entry.entryType === "REVERSAL" && entry.reversesId) return true;
  if (category === "OPENING") return entry.entryType === "OPENING";
  if (category === "SETTLEMENT") return entry.entryType === "RECEIPT" || entry.entryType === "PAYMENT";
  return entry.entryType === "CUSTOMER_REFUND" || entry.entryType === "SUPPLIER_REFUND";
}

export function PartnerRecords({ category, version = 0, onChanged }: { category: RecordCategory; version?: number; onChanged?: () => void }) {
  const { feedback, notify } = useBusinessFeedback();
  const [params] = useSearchParams();
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

  useEffect(() => {
    const id = Number(params.get("entryId"));
    if (id > 0) void partnerBalanceEntry(id).then(entry => {
      if (belongsToCategory(entry, category)) setDetail(entry);
      else setError("该记录不属于当前业务页");
    }).catch(e => setError(e instanceof Error ? e.message : "加载记录失败"));
  }, [params, category]);

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
    if (!reverseTarget || !reason.trim()) return;
    setSaving(true);
    setMessage("");
    try {
      const reversed = await reversePartnerEntry(reverseTarget.id, reason.trim());
      setReverseTarget(null);
      setReason("");
      notify({ type: "success", title: "往来记录冲销成功", description: `${reverseTarget.partnerName ?? "往来单位"} · ${reversed.direction === "CUSTOMER" ? "客户方向" : "供应商方向"}余额 ${reversed.balanceBefore} → ${reversed.balanceAfter} 元。原记录和冲销记录均保留在往来明细中。`, documentNo: reversed.documentNo, status: "已生效" });
      if (onChanged) onChanged();
      else setRevision(value => value + 1);
    } catch (e) {
      const message = e instanceof Error ? e.message : "冲销失败，请核对当前余额"; setMessage(message); notify({ type: "error", title: "往来冲销失败", description: message });
    } finally {
      setSaving(false);
    }
  }

  const columns: DataTableColumn<PartnerBalanceEntry>[] = [
    { title: "单号", dataIndex: "documentNo" },
    { title: "往来单位", dataIndex: "partnerName" },
    { title: "业务", key: "type", render: (_, row) => entryTypeLabel[row.entryType] ?? row.entryType },
    { title: "业务日期", dataIndex: "businessDate" },
    { title: "金额", dataIndex: "amount" },
    { title: "变动后余额", key: "balance", render: (_, row) => `${row.balanceAfter} 元（${balanceState(row.balanceAfter)}）` },
    { title: "附件", key: "attachments", render: (_, row) => `${row.attachmentCount ?? 0} 个` },
    { title: "状态", key: "status", render: (_, row) => row.reversedById ? "已冲销" : "已生效" },
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
      <DataTable columns={columns} dataSource={rows} rowKey="id" loading={loading} error={error} empty={<p className="text-sm text-text-secondary">当前条件下暂无记录，可在上方录入。</p>} />
    </DataTableCard>
    <DetailDialog footer={<BusinessReturnLink />} open={detail !== null} title={detail ? `${entryTypeLabel[detail.entryType]} · ${detail.documentNo}` : "记录详情"} onCancel={() => setDetail(null)}>
      {detail && <div className="space-y-3 text-sm">
        <p>{detail.partnerName} · {detail.direction === "CUSTOMER" ? "客户方向" : "供应商方向"}</p>
        <p>金额 {detail.amount} 元 · 余额 {detail.balanceBefore} → {detail.balanceAfter} 元</p>
        <p>业务日期 {detail.businessDate} · 实际生效 {formatDateTime(detail.effectiveAt)}</p>
        <p>操作人 {detail.operatorName ?? detail.operatorId}{detail.paymentMethod ? ` · 方式 ${detail.paymentMethod}` : ""}{detail.transactionNo ? ` · 流水号 ${detail.transactionNo}` : ""}</p>
        {detail.description && <p>说明：{detail.description}</p>}
        <PartnerAttachmentDetails key={detail.id} entryId={detail.id} kind={category} readOnly={!!detail.reversedById} onChanged={() => { setRevision(value => value + 1); void partnerBalanceEntry(detail.id).then(setDetail).catch(e => setError(String(e))); }} />
        {detail.reversedById ? <p>已由往来记录 {detail.reversedById} 冲销，原记录保留。</p> : <Button variant="secondary" onClick={() => { setReverseTarget(detail); setDetail(null); }}>冲销此记录</Button>}
      </div>}
    </DetailDialog>
    <FormDialog open={reverseTarget !== null} title="冲销往来记录" description={reverseTarget ? `原记录 ${reverseTarget.documentNo} · ${reverseTarget.amount} 元。冲销会追加反向记录，原记录保留。` : undefined} submitText="确认冲销" loading={saving} submitDisabled={!reason.trim()} onCancel={() => { setReverseTarget(null); setReason(""); }} onSubmit={reverse}>
      <label className="block text-sm">冲销原因（必填）<Textarea className="mt-2" value={reason} onChange={e => setReason(e.target.value)} maxLength={500} /></label>
      {message && <p role="alert" className="mt-2 text-sm text-error">{message}</p>}
    </FormDialog>
  </>;
}
