import { PartnerSelect } from "@/components/business/master-data-select";
import { withBusinessReturn } from "@/lib/business-navigation";
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams, useLocation } from "react-router-dom";
import { partnerBalances, partnerBalanceOverview, type PartnerBalance, type PartnerBalanceCategory, type PartnerBalanceOverview } from "@/api/business";
import { ContentCard } from "@/components/common/content-card";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { Field } from "@/components/common/field";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { cents, money } from "@/lib/partner-ledger";
import { cn } from "@/lib/utils";
import type { DataTableColumn } from "@/types";

const pageSize = 20;
const categories: { key: PartnerBalanceCategory; title: string; description: string }[] = [
  { key: "RECEIVABLE", title: "客户应收", description: "客户欠店铺" },
  { key: "PAYABLE", title: "供应商应付", description: "店铺欠供应商" },
  { key: "CUSTOMER_REFUND", title: "应退客户款", description: "店铺应退给客户" },
  { key: "SUPPLIER_REFUND", title: "应收供应商退款", description: "供应商应退给店铺" },
];

function amountLabel(row: PartnerBalance) {
  const value = cents(row.amount);
  if (value === null) return "余额异常";
  if (!row.hasRecords) return "尚无记录";
  if (value === 0n) return "净结清";
  return categories.find(c => c.key === (value > 0n
    ? row.direction === "CUSTOMER" ? "RECEIVABLE" : "PAYABLE"
    : row.direction === "CUSTOMER" ? "CUSTOMER_REFUND" : "SUPPLIER_REFUND"))!.title;
}
function displayAmount(amount: string) {
  const value = cents(amount);
  return value === null ? "—" : money(value < 0n ? -value : value).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

export function PartnerBalancesPage() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const partnerId = params.get("partnerId") ?? "";
  const partnerKeyword=params.get("partnerKeyword")??"";
  const direction = params.get("direction") ?? "";
  // Old direction/partner links keep their all-direction lookup semantics.
  const category = params.get("category") ?? (partnerId || direction ? "ALL" : "RECEIVABLE");
  const [draftPartnerId, setDraftPartnerId] = useState(partnerId);
  const [draftPartnerKeyword, setDraftPartnerKeyword] = useState(partnerKeyword);
  const [draftCategory, setDraftCategory] = useState(category);
  const [draftDirection, setDraftDirection] = useState(direction);
  useEffect(() => {
    setDraftPartnerId(partnerId);
    setDraftPartnerKeyword(partnerKeyword);
    setDraftCategory(category);
    setDraftDirection(direction);
  }, [partnerId, partnerKeyword, category, direction]);
  const rawPage = Number(params.get("page"));
  const page = Number.isSafeInteger(rawPage) && rawPage > 0 ? rawPage : 1;
  const [rows, setRows] = useState<PartnerBalance[]>([]);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [overview, setOverview] = useState<PartnerBalanceOverview | null>(null);
  const [overviewError, setOverviewError] = useState("");
  const [overviewLoading, setOverviewLoading] = useState(true);
  const [ranking, setRanking] = useState("CUSTOMER");
  const [version, setVersion] = useState(0);

  function change(key: string, value: string) {
    const p = new URLSearchParams(params);
    p.set("category", category);
    if (value) p.set(key, value); else p.delete(key);
    if (key === "category") p.delete("direction");
    p.delete("page");
    setParams(p);
  }
  function applySearch(next = { partnerId: draftPartnerId, partnerKeyword: draftPartnerKeyword, category: draftCategory, direction: draftDirection }) {
    const p = new URLSearchParams(params);
    p.set("category", next.category);
    p.delete("partnerId"); p.delete("partnerKeyword"); p.delete("direction"); p.delete("page");
    if (next.partnerKeyword.trim()) p.set("partnerKeyword", next.partnerKeyword.trim());
    else if (next.partnerId) p.set("partnerId", next.partnerId);
    if (next.direction) p.set("direction", next.direction);
    setParams(p);
  }
  function setPage(next: number) {
    const p = new URLSearchParams(params);
    p.set("category", category);
    p.set("page", String(next));
    setParams(p);
  }
  const load = useCallback(() => {
    let active = true;
    setLoading(true);
    void partnerBalances({ ...(partnerId ? { partnerId } : {}), partnerKeyword, direction, category, page, pageSize }).then(result => {
      if (active) { setRows(result.records); setTotal(result.total); setError(""); }
    }).catch(e => { if (active) { setRows([]); setTotal(0); setError(e instanceof Error ? e.message : "读取往来余额失败"); } })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [partnerId, partnerKeyword, direction, category, page]);
  useEffect(load, [load, version]);
  useEffect(() => {
    let active = true;
    setOverviewLoading(true);
    void partnerBalanceOverview().then(result => { if (active) { setOverview(result); setOverviewError(""); } })
      .catch(e => { if (active) { setOverview(null); setOverviewError(e instanceof Error ? e.message : "读取全店概览失败"); } })
      .finally(() => { if (active) setOverviewLoading(false); });
    return () => { active = false; };
  }, [version]);

  function route(path: string, row: PartnerBalance) {
    const from = new URLSearchParams(location.search);
    from.set("category", category);
    navigate(withBusinessReturn(`${path}?${new URLSearchParams({ partnerId: String(row.partnerId), direction: row.direction })}`, `${location.pathname}?${from}`));
  }
  const columns: DataTableColumn<PartnerBalance>[] = [
    { title: "往来单位", dataIndex: "partnerName" },
    { title: "身份", key: "direction", nowrap: true, render: (_, row) => row.direction === "CUSTOMER" ? "客户" : "供应商" },
    { title: "当前款项", key: "state", nowrap: true, render: (_, row) => amountLabel(row) },
    { title: "金额（元）", key: "amount", align: "right", nowrap: true, render: (_, row) => <strong className="tabular-nums">{displayAmount(row.amount)}</strong> },
    { title: "往来记录数", dataIndex: "entryCount", align: "right", nowrap: true },
    { title: "操作", key: "action", nowrap: true, render: (_, row) => {
      const amount = cents(row.amount);
      return <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="secondary" onClick={() => route("/business/partner-ledger", row)}>查看明细</Button>
        {amount !== null && amount > 0n && <Button size="sm" variant="primary" onClick={() => route("/business/settlements", row)}>{row.direction === "CUSTOMER" ? "收款" : "付款"}</Button>}
        {amount !== null && amount < 0n && <Button size="sm" variant="primary" onClick={() => route("/business/refunds", row)}>{row.direction === "CUSTOMER" ? "向客户退款" : "收供应商退款"}</Button>}
      </div>;
    } },
  ];
  const topRows = ranking === "CUSTOMER" ? overview?.topReceivables ?? [] : overview?.topPayables ?? [];
  const maxAmount = cents(topRows[0]?.amount ?? "0.00") ?? 0n;
  const selectedTitle = categories.find(c => c.key === category)?.title ?? "全部往来";

  return <div className="space-y-space-6">
    <PageHeader title="往来余额" description="查看当前未结余额，按金额安排收付款。客户与供应商分别计算，不自动抵扣。" actions={<div className="flex gap-2"><Button onClick={() => setVersion(value => value + 1)}>刷新</Button><Button onClick={() => navigate("/business/opening-balances")}>录入期初</Button></div>} />
    <section aria-label="全店当前未结余额" aria-busy={overviewLoading} className="rounded-admin border border-border bg-surface">
      <div className="border-b border-border px-card py-3"><h2 className="font-semibold">全店当前未结余额</h2><p className="mt-1 text-sm text-text-secondary">覆盖全店，不受下方搜索和分页影响。</p></div>
      {overviewError && !overviewLoading ? <div role="alert" className="flex flex-wrap items-center gap-3 p-card"><span className="text-error">{overviewError}</span><Button onClick={() => setVersion(v => v + 1)}>重试概览</Button></div> :
        <div className="grid grid-cols-1 divide-y divide-border sm:grid-cols-2 lg:grid-cols-4 lg:divide-y-0">
          {categories.map(c => {
            const summary = overview?.summaries.find(s => s.category === c.key);
            return <button key={c.key} type="button" aria-pressed={category === c.key} aria-label={`查看${c.title}`} onClick={() => change("category", c.key)} className={cn("p-card text-left hover:bg-neutral-background focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary", category === c.key && "bg-primary/5")}>
              <span className={cn("text-sm font-medium", category === c.key ? "text-primary" : "text-text-secondary")}>{c.title}</span>
              <span className="mt-2 block text-2xl font-semibold tabular-nums">{overviewLoading ? "读取中…" : summary ? displayAmount(summary.amount) : "—"}<span className="ml-1 text-sm font-normal text-text-secondary">元</span></span>
              <span className="mt-2 block text-sm text-text-secondary">{overviewLoading ? "正在核对余额" : `${summary?.partnerCount ?? "—"} 个单位 · ${c.description}`}</span>
            </button>;
          })}
        </div>}
    </section>
    <ContentCard title="欠款金额前十名" description="全店金额排名，点击单位查看对应方向明细。" extra={<Select className="w-40 shrink-0" aria-label="排名方向" value={ranking} onChange={e => setRanking(e.target.value)}><option value="CUSTOMER">客户应收</option><option value="SUPPLIER">供应商应付</option></Select>}>
      <div role="region" aria-label="欠款金额排名" aria-busy={overviewLoading}>
        {overviewLoading ? <p className="text-sm text-text-secondary">正在读取全店排名…</p> : overviewError ? <p className="text-sm text-text-secondary">概览加载失败，请重试概览。</p> : topRows.length === 0 ? <EmptyState title={ranking === "CUSTOMER" ? "当前没有客户应收" : "当前没有供应商应付"} description="该方向暂无待处理欠款。" /> :
          <ol className="grid gap-2 xl:grid-cols-2">
            {topRows.map((row, index) => {
              const value = cents(row.amount) ?? 0n;
              const percent = maxAmount > 0n ? Number(value * 10000n / maxAmount) / 100 : 0;
              return <li key={row.partnerId}><button type="button" aria-label={`${row.partnerName}，${displayAmount(row.amount)}元，查看明细`} onClick={() => route("/business/partner-ledger", row)} className="grid w-full grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-2 rounded-control p-2 text-left hover:bg-neutral-background focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">
                <span className="truncate text-sm" title={row.partnerName}><span className="mr-2 text-text-tertiary">{index + 1}.</span>{row.partnerName}</span><strong className="text-sm tabular-nums">{displayAmount(row.amount)} 元</strong>
                <span className="col-span-2 h-2 overflow-hidden rounded bg-neutral-background" aria-hidden="true"><span className="block h-full rounded bg-primary" style={{ width: `${percent}%` }} /></span>
              </button></li>;
            })}
          </ol>}
      </div>
    </ContentCard>
    <SearchFilterBar actions={<><Button onClick={() => applySearch()}>查询</Button><Button variant="secondary" onClick={() => applySearch({ partnerId: "", partnerKeyword: "", category, direction: "" })}>清除筛选</Button></>}>
      <Field label="往来单位"><PartnerSelect inlineBrowse label="筛选往来单位" historical value={Number(draftPartnerId)} keyword={draftPartnerKeyword} onDraftKeywordChange={setDraftPartnerKeyword} onKeywordChange={keyword => applySearch({ partnerId: "", partnerKeyword: keyword, category: draftCategory, direction: draftDirection })} onChange={record => { setDraftPartnerId(record ? String(record.id) : ""); setDraftPartnerKeyword(""); }} /></Field>
      <Field label="款项分类"><Select aria-label="筛选款项分类" value={draftCategory} onChange={e => setDraftCategory(e.target.value)}>{categories.map(c => <option key={c.key} value={c.key}>{c.title}</option>)}<option value="ALL">全部（含结清及无记录）</option></Select></Field>
      {draftCategory === "ALL" && <Field label="往来方向"><Select aria-label="筛选往来方向" value={draftDirection} onChange={e => setDraftDirection(e.target.value)}><option value="">客户及供应商</option><option value="CUSTOMER">客户</option><option value="SUPPLIER">供应商</option></Select></Field>}
    </SearchFilterBar>
    <DataTableCard toolbar={<div className="p-3 text-sm"><strong>{selectedTitle}</strong> · {loading ? "查询中…" : `${total} ${category === "ALL" ? "条往来余额" : "个单位"}`}{category !== "ALL" && " · 按金额从大到小"}{partnerId && " · 已筛选单位"}</div>} pagination={<Pagination page={page} pageSize={pageSize} total={total} onPageChange={setPage} />}>
      <DataTable columns={columns} dataSource={rows} rowKey={row => `${row.partnerId}-${row.direction}`} loading={loading} error={error ? <div role="alert" className="space-y-3"><p className="text-error">{error}</p><Button onClick={() => setVersion(v => v + 1)}>重试列表</Button></div> : undefined} empty={<EmptyState title="当前筛选下没有往来款项" description="可切换款项分类、清除单位筛选，或选择全部查看结清及无记录方向。" />} minWidth={850} />
    </DataTableCard>
  </div>;
}
