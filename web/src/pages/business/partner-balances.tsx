import { PartnerSelect } from "@/components/business/master-data-select";
import { withBusinessReturn } from "@/lib/business-navigation";
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams, useLocation } from "react-router-dom";
import { partnerBalances, type PartnerBalance } from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { cents } from "@/lib/partner-ledger";
import type { DataTableColumn } from "@/types";

const pageSize = 20;
export function PartnerBalancesPage() {
  const navigate = useNavigate();
  const [params,setParams] = useSearchParams();
  const location=useLocation();
  const partnerId=params.get("partnerId")??"",direction=params.get("direction")??"";
  const rawPage=Number(params.get("page"));const page=Number.isSafeInteger(rawPage)&&rawPage>0?rawPage:1;
  function change(key:string,value:string){const p=new URLSearchParams(params);if(value)p.set(key,value);else p.delete(key);p.delete("page");setParams(p)}
  function setPage(next:number){const p=new URLSearchParams(params);p.set("page",String(next));setParams(p)}
  const [rows, setRows] = useState<PartnerBalance[]>([]);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [version, setVersion] = useState(0);

  const load = useCallback(() => {
    let active = true;
    setLoading(true);
    void partnerBalances({ partnerId: partnerId || 0, direction, page, pageSize }).then(result => {
      if (active) { setRows(result.records); setTotal(result.total); setError(""); }
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : "读取往来余额失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [partnerId, direction, page]);
  useEffect(load, [load, version]);

  function route(path: string, row: PartnerBalance) {
    navigate(withBusinessReturn(`${path}?${new URLSearchParams({ partnerId: String(row.partnerId), direction: row.direction })}`, location.pathname+location.search));
  }
  const columns: DataTableColumn<PartnerBalance>[] = [
    { title: "往来单位", dataIndex: "partnerName" },
    { title: "方向", key: "direction", render: (_, row) => row.direction === "CUSTOMER" ? "客户应收" : "供应商应付" },
    { title: "当前余额", key: "amount", render: (_, row) => <strong className="tabular-nums">{row.amount} 元</strong> },
    { title: "状态", key: "state", render: (_, row) => row.hasRecords ? (cents(row.amount) === null ? "余额异常" : cents(row.amount)! > 0n ? row.direction === "CUSTOMER" ? "客户欠店铺" : "店铺欠供应商" : cents(row.amount)! < 0n ? row.direction === "CUSTOMER" ? "店铺待退客户" : "供应商待退店铺" : "净结清") : "尚无记录" },
    { title: "来源记录", dataIndex: "entryCount" },
    { title: "操作", key: "action", render: (_, row) => {
      const amount = cents(row.amount);
      return <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="secondary" onClick={() => route("/business/partner-ledger", row)}>查看明细</Button>
        {amount !== null && amount > 0n && <Button size="sm" onClick={() => route("/business/settlements", row)}>{row.direction === "CUSTOMER" ? "收款" : "付款"}</Button>}
        {amount !== null && amount < 0n && <Button size="sm" onClick={() => route("/business/refunds", row)}>{row.direction === "CUSTOMER" ? "向客户退款" : "收供应商退款"}</Button>}
      </div>;
    } },
  ];

  return <div className="space-y-4">
    <PageHeader title="往来余额" description="客户应收与供应商应付分别显示。正数是欠款，负数是待退款；同一单位的两个方向不自动抵扣。" actions={<Button variant="secondary" onClick={() => navigate("/business/opening-balances")}>录入期初</Button>} />
    <SearchFilterBar actions={<Button variant="secondary" onClick={() => setVersion(value => value + 1)}>刷新</Button>}>
      <PartnerSelect label="筛选往来单位" historical value={Number(partnerId)} onChange={p=>change("partnerId",p?String(p.id):"")} />
      <Select aria-label="筛选往来方向" value={direction} onChange={e => change("direction",e.target.value)}><option value="">客户及供应商</option><option value="CUSTOMER">客户</option><option value="SUPPLIER">供应商</option></Select>
    </SearchFilterBar>
    <DataTableCard toolbar={<div className="p-3 text-sm">当前余额 · 共 {total} 个单位方向</div>} pagination={<Pagination page={page} pageSize={pageSize} total={total} onPageChange={setPage} />}>
      <DataTable columns={columns} dataSource={rows} rowKey={row => `${row.partnerId}-${row.direction}`} loading={loading} error={error} minWidth={850} />
    </DataTableCard>
  </div>;
}
