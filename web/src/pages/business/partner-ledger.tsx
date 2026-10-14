import { useExcelExport } from "@/hooks/use-excel-export";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { TableToolbar } from "@/components/common/table-toolbar";
import { exportPartnerEntries } from "@/api/business";
import { businessMidnightToUtc } from "@/lib/datetime";
import { formatDateTime } from "@/lib/datetime";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { LedgerSourceDetails, LedgerSourceDisclosure } from "@/components/business/ledger-source-details";
import { PartnerSelect } from "@/components/business/master-data-select";
import { withBusinessReturn } from "@/lib/business-navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { getPartnerStatement, partnerBalanceEntries, partnerBalanceEntry, getPartner, type PartnerBalanceEntry, type PartnerRecord, type PartnerStatement } from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { Field } from "@/components/common/field";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { balanceDescription, cents, entryTypeLabel, money, localPeriod, periodDateInput } from "@/lib/partner-ledger";
import type { DataTableColumn } from "@/types";

const pageSize = 20;
export function PartnerLedgerPage() {
  const { feedback, notify } = useBusinessFeedback();
  const excelExport = useExcelExport(notify);
  const navigate = useNavigate();
  const [params,setParams] = useSearchParams();
  const [selectedPartner,setSelectedPartner] = useState<PartnerRecord|null>(null);
  const [partnerKeyword,setPartnerKeyword]=useState(params.get("partnerKeyword")??"");
  const [partnerId, setPartnerId] = useState(params.get("partnerId") ?? "");
  const [direction, setDirection] = useState(params.get("direction") === "SUPPLIER" ? "SUPPLIER" : "CUSTOMER");
  const [from, setFrom] = useState(periodDateInput(params.get("from")??""));
  const [to, setTo] = useState(periodDateInput(params.get("to")??"",true));
  const query=useMemo(()=>({partnerId:params.get("partnerId")??"",partnerKeyword:params.get("partnerKeyword")??"",direction:params.get("direction")==="SUPPLIER"?"SUPPLIER":"CUSTOMER",from:params.get("from")??"",to:params.get("to")??""}),[params]);
  useEffect(()=>{setPartnerId(query.partnerId);setPartnerKeyword(query.partnerKeyword);setDirection(query.direction);setFrom(periodDateInput(query.from));setTo(periodDateInput(query.to,true));},[query]);
  const rawPage=Number(params.get("page"));const page=Number.isSafeInteger(rawPage)&&rawPage>0?rawPage:1;
  function setPage(next:number){const p=new URLSearchParams(params);p.set("page",String(next));setParams(p)}
  const [rows, setRows] = useState<PartnerBalanceEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [summary, setSummary] = useState<PartnerStatement | null>(null);
  const [detail, setDetail] = useState<PartnerBalanceEntry | null>(null);
  const detailAmount = detail ? cents(detail.amount) : null;
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [version, setVersion] = useState(0);
  const [expanded, setExpanded] = useState<{ context: string; ids: string[] }>({ context: "", ids: [] });
  const expansionContext = JSON.stringify([query, page, version]);
  const expandedKeys = expanded.context === expansionContext ? expanded.ids : [];
  function toggleDetails(id: number) {
    const key = String(id);
    setExpanded({ context: expansionContext, ids: expandedKeys.includes(key) ? expandedKeys.filter(value => value !== key) : [...expandedKeys, key] });
  }

  useEffect(()=>{let active=true;if(query.partnerId)void getPartner(Number(query.partnerId)).then(p=>{if(active)setSelectedPartner(p)}).catch(()=>{if(active)setSelectedPartner(null)});else setSelectedPartner(null);return()=>{active=false}},[query.partnerId]);
  useEffect(() => {
    let active=true;const id=Number(params.get("entryId"));
    if(id>0)void partnerBalanceEntry(id).then(d=>{if(active)setDetail(d)}).catch(e=>{if(active)setError(e instanceof Error?e.message:"加载记录失败")});else setDetail(null);
    return()=>{active=false};
  }, [params]);
  function closeDetail(){setDetail(null);const p=new URLSearchParams(params);p.delete("entryId");setParams(p,{replace:true})}

  const load = useCallback(() => {
    if (!query.partnerId&&!query.partnerKeyword) { setRows([]); setTotal(0); setSummary(null); setLoading(false); return; }
    let active = true;
    setLoading(true);
    setSummary(null);
    const period = query.partnerId && query.from && query.to ? getPartnerStatement({ partnerId: query.partnerId, direction: query.direction ?? "CUSTOMER", from: query.from, to: query.to }) : Promise.resolve(null);
    void Promise.all([partnerBalanceEntries({ ...query, page, pageSize }), period]).then(([entries, statement]) => {
      if (active) { setRows(entries.records); setTotal(entries.total); setSummary(statement); setError(""); }
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : "读取往来明细失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [query, page]);
  useEffect(load, [load, version]);

  function search() {
    try {
      if (!partnerId&&!partnerKeyword.trim()) { setError("请输入关键词或选择往来单位"); return; }
      const range: {from?:string;to?:string} = from || to ? localPeriod(from, to) : {};
      const p=new URLSearchParams(params);if(partnerKeyword.trim()){p.delete("partnerId");p.set("partnerKeyword",partnerKeyword.trim())}else{p.set("partnerId",partnerId);p.delete("partnerKeyword")};p.set("direction",direction);if(range.from)p.set("from",range.from);else p.delete("from");if(range.to)p.set("to",range.to);else p.delete("to");p.delete("page");p.delete("entryId");setParams(p);setVersion(v=>v+1);
      setError("");
    } catch (e) { setError(e instanceof Error ? e.message : "期间无效"); }
  }
  function print() {
    try {
      if (!partnerId) throw new Error("请先选择往来单位");
      const range = localPeriod(from, to);
      const p=new URLSearchParams(params);p.set("partnerId",partnerId);p.set("direction",direction);p.set("from",range.from);p.set("to",range.to);p.delete("page");
      navigate(withBusinessReturn(`/business/partner-statements?${new URLSearchParams({ partnerId, direction, ...range })}`,`/business/partner-ledger?${p}`));
    } catch (e) { setError(e instanceof Error ? e.message : "打印条件无效"); }
  }

  function exportEntries() {
    if (!partnerId && !partnerKeyword.trim()) { notify({ type: "error", title: "导出失败", description: "请输入关键词或选择往来单位" }); return; }
    const args = { partnerId: partnerKeyword.trim() ? "" : partnerId, partnerKeyword: partnerKeyword.trim(), direction,
      from: from ? businessMidnightToUtc(from) : "", to: to ? businessMidnightToUtc(to, true) : "" };
    void excelExport.start({ count: () => partnerBalanceEntries({ ...args, page: 1, pageSize: 1 }), download: () => exportPartnerEntries(args) });
  }

  const columns: DataTableColumn<PartnerBalanceEntry>[] = [
    {title:"往来单位／方向",key:"partner",render:(_,r)=><>{r.partnerName}<span className="block text-xs text-text-secondary">{r.direction==="CUSTOMER"?"客户":"供应商"}</span></>},
    { title: "类型 / 来源号", key: "source", render: (_, row) => <><span className="block text-xs text-text-secondary">{entryTypeLabel[row.entryType] ?? row.entryType}</span><Button size="sm" variant="secondary" onClick={() => void partnerBalanceEntry(row.id).then(setDetail).catch(e => setError(String(e)))}>{row.documentNo}</Button></> },
    { title: "业务日期", dataIndex: "businessDate" },
    { title: "实际生效", key: "effective", render: (_, row) => formatDateTime(row.effectiveAt) },
    { title: "实际操作人", key: "actor", render: (_, row) => `${row.operatorName ?? ""}（${row.operatorId}）` },
    { title: "变动（元）", dataIndex: "amount" },
    { title: "前余额", dataIndex: "balanceBefore" },
    { title: "后余额", dataIndex: "balanceAfter" },
    { title: "细节", key: "action", render: (_, row) => <Button size="sm" variant="secondary" aria-expanded={expandedKeys.includes(String(row.id))} onClick={() => toggleDetails(row.id)}>{expandedKeys.includes(String(row.id)) ? "收起细节" : "展示细节"}</Button> },
  ];

  return <div className="space-y-4">
    <PageHeader title="往来明细" description="选择一个往来单位和客户或供应商方向，按实际生效时间核对全部金额变化。" actions={<Button disabled={!partnerId || Boolean(partnerKeyword) || !from || !to} onClick={print}>打印期间对账单</Button>} />
    <SearchFilterBar actions={<><Button onClick={search}>查询</Button><Button variant="secondary" onClick={() => setVersion(value => value + 1)}>刷新</Button></>}>
      <Field label="往来单位"><PartnerSelect inlineBrowse label="往来单位" historical value={Number(partnerId)} keyword={partnerKeyword} onDraftKeywordChange={setPartnerKeyword} onKeywordChange={k=>{setPartnerKeyword(k);setPartnerId("");const p=new URLSearchParams(params);p.delete("partnerId");if(k)p.set("partnerKeyword",k);else p.delete("partnerKeyword");p.delete("page");setParams(p)}} onChange={r=>{setPartnerKeyword("");setPartnerId(r?String(r.id):"")}} /></Field>
      <Field label="往来方向"><Select aria-label="往来方向" value={direction} onChange={e => setDirection(e.target.value)}><option value="CUSTOMER">客户方向</option><option value="SUPPLIER">供应商方向</option></Select></Field>
      <Field label="生效开始日"><Input type="date" value={from} onChange={e => setFrom(e.target.value)} /></Field>
      <Field label="生效结束日（含当日）"><Input type="date" value={to} onChange={e => setTo(e.target.value)} /></Field>
    </SearchFilterBar>
    <BusinessFeedback feedback={feedback} />
    {excelExport.confirmation}
    {error && <p role="alert" className="text-sm text-error">{error}</p>}
    {query.partnerKeyword&&<p className="text-sm text-text-secondary">关键词只查当前单位档案。期间余额与对账打印请先指定一个单位和方向。</p>}
    {summary && <section className="grid gap-3 rounded-admin border border-border bg-surface p-4 text-sm sm:grid-cols-3" aria-label="期间余额">
      <p>期间期初<br /><strong className="text-lg tabular-nums">{summary.openingAmount} 元</strong></p>
      <p>期间变动<br /><strong className="text-lg tabular-nums">{summary.netChange} 元</strong></p>
      <p>期间期末<br /><strong className="text-lg tabular-nums">{summary.closingAmount} 元</strong></p>
    </section>}
    <DataTableCard toolbar={<TableToolbar title="往来明细" description={(query.partnerId||query.partnerKeyword) ? `${query.partnerKeyword?`关键词：${query.partnerKeyword}，各单位余额分别保留`:selectedPartner?.id === Number(query.partnerId) ? selectedPartner.name : "往来单位"} · ${query.direction === "CUSTOMER" ? "客户" : "供应商"}方向 · 共 ${total} 笔` : "请选择往来单位和方向"} actions={<Button size="sm" variant="secondary" disabled={excelExport.exporting} onClick={exportEntries}>{excelExport.exporting ? "导出中" : "导出明细 Excel"}</Button>} />} pagination={query.partnerId||query.partnerKeyword ? <Pagination page={page} pageSize={pageSize} total={total} onPageChange={setPage} /> : undefined}>
      <DataTable columns={columns} dataSource={rows} rowKey="id" loading={loading} error={error} expandedRowKeys={expandedKeys} renderExpandedRow={row => <LedgerSourceDetails key={`${row.id}-${version}`} entry={row} />} empty={<p className="text-sm text-text-secondary">{query.partnerId||query.partnerKeyword ? "当前条件下没有生效记录。" : "选择往来单位后查看明细。"}</p>} />
    </DataTableCard>
    <DetailDialog documentLayout contentClassName="max-w-[960px]" headerClassName="[&_h2]:break-all" footer={<div className="flex items-center justify-between gap-3"><BusinessReturnLink /><Button onClick={closeDetail}>关闭</Button></div>} open={detail !== null} title={detail ? `${entryTypeLabel[detail.entryType] ?? "往来记录"} · ${detail.documentNo}` : "往来记录"} onCancel={closeDetail}>
      {detail && <div className="space-y-5 text-sm">
        <div className="flex flex-wrap justify-between gap-2"><p className="font-medium">{detail.partnerName} · {detail.direction === "CUSTOMER" ? "客户往来" : "供应商往来"}</p><span className="text-text-secondary">{detail.entryType === "REVERSAL" ? "冲销记录" : detail.reversedById ? "已冲销" : "已生效"}</span></div>
        <section aria-label="本笔金额影响" className="grid gap-4 rounded-control border border-info-border bg-info-background p-4 sm:grid-cols-3">
          <div><p className="text-text-secondary">{detailAmount === null ? "往来变动" : detailAmount < 0n ? "往来余额减少" : "往来余额增加"}</p><p className="mt-1 text-3xl font-semibold text-primary tabular-nums">¥{detailAmount === null ? "—" : money(detailAmount < 0n ? -detailAmount : detailAmount)}</p></div>
          <div><p className="text-text-secondary">本笔生效前</p><p className="mt-2 font-medium tabular-nums">{balanceDescription(detail.direction, detail.balanceBefore)}</p></div>
          <div><p className="text-text-secondary">本笔生效后</p><p className="mt-2 font-medium tabular-nums">{balanceDescription(detail.direction, detail.balanceAfter)}</p></div>
        </section>
        <div className="grid gap-x-6 gap-y-2 sm:grid-cols-2"><p>业务日期：{detail.businessDate}</p><p>实际生效：{formatDateTime(detail.effectiveAt)}</p><p>操作人：{detail.operatorName ?? detail.operatorId}</p></div>
        {detail.description && <p className="whitespace-pre-wrap break-words text-text-secondary">说明：{detail.description}</p>}
        {detail.reversedDocumentNo && <p>冲销的原记录：{detail.reversedDocumentNo}</p>}
        {detail.reversedById && <p className="text-text-secondary">已生成反向冲销记录，原记录保留。</p>}
        <div className="border-t border-border pt-4"><LedgerSourceDisclosure key={detail.id} entry={detail} /></div>
      </div>}
    </DetailDialog>
  </div>;
}
