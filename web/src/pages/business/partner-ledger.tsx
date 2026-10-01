import { useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { getPartnerStatement, partnerBalanceEntries, partnerBalanceEntry, partnerPage, type PartnerBalanceEntry, type PartnerRecord, type PartnerStatement } from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { entryTypeLabel, ledgerSource, localPeriod } from "@/lib/partner-ledger";
import type { DataTableColumn } from "@/types";

const pageSize = 20;
export function PartnerLedgerPage() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [partners, setPartners] = useState<PartnerRecord[]>([]);
  const [partnerId, setPartnerId] = useState(params.get("partnerId") ?? "");
  const [direction, setDirection] = useState(params.get("direction") === "SUPPLIER" ? "SUPPLIER" : "CUSTOMER");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [query, setQuery] = useState<Record<string, string | number>>({ partnerId, direction });
  const [page, setPage] = useState(1);
  const [rows, setRows] = useState<PartnerBalanceEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [summary, setSummary] = useState<PartnerStatement | null>(null);
  const [detail, setDetail] = useState<PartnerBalanceEntry | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    void partnerPage({ page: 1, pageSize: 500 }).then(result => setPartners(result.records)).catch(e => setError(String(e)));
  }, []);
  useEffect(() => {
    const id = Number(params.get("entryId"));
    if (id > 0) void partnerBalanceEntry(id).then(setDetail).catch(e => setError(e instanceof Error ? e.message : "加载记录失败"));
  }, [params]);

  const load = useCallback(() => {
    if (!query.partnerId) { setRows([]); setTotal(0); setSummary(null); setLoading(false); return; }
    let active = true;
    setLoading(true);
    const period = query.from && query.to ? getPartnerStatement({ partnerId: query.partnerId, direction: query.direction ?? "CUSTOMER", from: query.from, to: query.to }) : Promise.resolve(null);
    void Promise.all([partnerBalanceEntries({ ...query, page, pageSize }), period]).then(([entries, statement]) => {
      if (active) { setRows(entries.records); setTotal(entries.total); setSummary(statement); setError(""); }
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : "读取往来明细失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [query, page]);
  useEffect(load, [load, version]);

  function search() {
    try {
      if (!partnerId) { setError("请先选择往来单位"); return; }
      const range = from || to ? localPeriod(from, to) : {};
      setQuery({ partnerId, direction, ...range });
      setPage(1);
      setError("");
    } catch (e) { setError(e instanceof Error ? e.message : "期间无效"); }
  }
  function print() {
    try {
      if (!partnerId) throw new Error("请先选择往来单位");
      const range = localPeriod(from, to);
      navigate(`/business/partner-statements?${new URLSearchParams({ partnerId, direction, ...range })}`);
    } catch (e) { setError(e instanceof Error ? e.message : "打印条件无效"); }
  }
  async function openSource(entry: PartnerBalanceEntry) {
    try {
      const source = entry.entryType === "REVERSAL" && entry.reversesId ? await partnerBalanceEntry(entry.reversesId) : entry;
      navigate(ledgerSource(source));
    } catch (e) { setError(e instanceof Error ? e.message : "打开来源失败"); }
  }

  const columns: DataTableColumn<PartnerBalanceEntry>[] = [
    { title: "类型 / 来源号", key: "source", render: (_, row) => <><span className="block text-xs text-text-secondary">{entryTypeLabel[row.entryType] ?? row.entryType}</span><Button size="sm" variant="secondary" onClick={() => void partnerBalanceEntry(row.id).then(setDetail).catch(e => setError(String(e)))}>{row.documentNo}</Button></> },
    { title: "业务日期", dataIndex: "businessDate" },
    { title: "实际生效", key: "effective", render: (_, row) => new Date(row.effectiveAt).toLocaleString() },
    { title: "实际操作人", key: "actor", render: (_, row) => `${row.operatorName ?? ""}（${row.operatorId}）` },
    { title: "变动（元）", dataIndex: "amount" },
    { title: "前余额", dataIndex: "balanceBefore" },
    { title: "后余额", dataIndex: "balanceAfter" },
    { title: "来源", key: "action", render: (_, row) => <Button size="sm" variant="secondary" onClick={() => void openSource(row)}>打开来源</Button> },
  ];

  return <div className="space-y-4">
    <PageHeader title="往来明细" description="选择一个往来单位和客户或供应商方向，按实际生效时间核对全部金额变化。" actions={<Button disabled={!partnerId || !from || !to} onClick={print}>打印期间对账单</Button>} />
    <SearchFilterBar actions={<><Button onClick={search}>查询</Button><Button variant="secondary" onClick={() => setVersion(value => value + 1)}>刷新</Button></>}>
      <Select aria-label="往来单位" value={partnerId} onChange={e => setPartnerId(e.target.value)}><option value="">选择往来单位</option>{partners.map(partner => <option value={partner.id} key={partner.id}>{partner.name}（{partner.code}）</option>)}</Select>
      <Select aria-label="往来方向" value={direction} onChange={e => setDirection(e.target.value)}><option value="CUSTOMER">客户方向</option><option value="SUPPLIER">供应商方向</option></Select>
      <label className="text-sm">生效开始日<Input type="date" value={from} onChange={e => setFrom(e.target.value)} /></label>
      <label className="text-sm">生效结束日（含当日）<Input type="date" value={to} onChange={e => setTo(e.target.value)} /></label>
    </SearchFilterBar>
    {error && <p role="alert" className="text-sm text-error">{error}</p>}
    {summary && <section className="grid gap-3 rounded-admin border border-border bg-surface p-4 text-sm sm:grid-cols-3" aria-label="期间余额">
      <p>期间期初<br /><strong className="text-lg tabular-nums">{summary.openingAmount} 元</strong></p>
      <p>期间变动<br /><strong className="text-lg tabular-nums">{summary.netChange} 元</strong></p>
      <p>期间期末<br /><strong className="text-lg tabular-nums">{summary.closingAmount} 元</strong></p>
    </section>}
    <DataTableCard toolbar={<div className="p-3 text-sm">{query.partnerId ? `${partners.find(partner => String(partner.id) === String(query.partnerId))?.name ?? "往来单位"} · ${query.direction === "CUSTOMER" ? "客户" : "供应商"}方向 · 共 ${total} 笔` : "请选择往来单位和方向"}</div>} pagination={query.partnerId ? <Pagination page={page} pageSize={pageSize} total={total} onPageChange={setPage} /> : undefined}>
      <DataTable columns={columns} dataSource={rows} rowKey="id" loading={loading} error={error} empty={<p className="text-sm text-text-secondary">{query.partnerId ? "当前条件下没有生效记录。" : "选择往来单位后查看明细。"}</p>} />
    </DataTableCard>
    <DetailDialog open={detail !== null} title={detail ? `往来记录 · ${detail.documentNo}` : "往来记录"} onCancel={() => setDetail(null)}>
      {detail && <div className="space-y-3 text-sm">
        <p>{detail.partnerName} · {entryTypeLabel[detail.entryType] ?? detail.entryType}</p>
        <p>业务日期 {detail.businessDate} · 实际生效 {new Date(detail.effectiveAt).toLocaleString()}</p>
        <p>变动 {detail.amount} 元 · 余额 {detail.balanceBefore} → {detail.balanceAfter} 元</p>
        <p>操作人 {detail.operatorName ?? detail.operatorId}{detail.description ? ` · ${detail.description}` : ""}</p>
        {detail.reversedDocumentNo && <p>冲销来源：{detail.reversedDocumentNo}</p>}
        {detail.reversedById && <p>已由往来记录 {detail.reversedById} 冲销</p>}
        <Button variant="secondary" size="sm" onClick={() => void openSource(detail)}>打开来源业务</Button>
      </div>}
    </DetailDialog>
  </div>;
}
