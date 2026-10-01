import { useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { partnerBalances, partnerPage, type PartnerBalance, type PartnerRecord } from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { balanceState, cents } from "@/lib/partner-ledger";
import type { DataTableColumn } from "@/types";

const pageSize = 20;
export function PartnerBalancesPage() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [partners, setPartners] = useState<PartnerRecord[]>([]);
  const [partnerId, setPartnerId] = useState(params.get("partnerId") ?? "");
  const [direction, setDirection] = useState(params.get("direction") ?? "");
  const [page, setPage] = useState(1);
  const [rows, setRows] = useState<PartnerBalance[]>([]);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    void partnerPage({ page: 1, pageSize: 500 }).then(result => setPartners(result.records)).catch(e => setError(String(e)));
  }, []);
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
    navigate(`${path}?${new URLSearchParams({ partnerId: String(row.partnerId), direction: row.direction })}`);
  }
  const columns: DataTableColumn<PartnerBalance>[] = [
    { title: "往来单位", dataIndex: "partnerName" },
    { title: "方向", key: "direction", render: (_, row) => row.direction === "CUSTOMER" ? "客户应收" : "供应商应付" },
    { title: "当前余额", key: "amount", render: (_, row) => <strong className="tabular-nums">{row.amount} 元</strong> },
    { title: "状态", key: "state", render: (_, row) => row.hasRecords ? balanceState(row.amount) : "尚无记录" },
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
      <Select aria-label="筛选往来单位" value={partnerId} onChange={e => { setPartnerId(e.target.value); setPage(1); }}><option value="">全部往来单位</option>{partners.map(partner => <option key={partner.id} value={partner.id}>{partner.name}（{partner.code}）</option>)}</Select>
      <Select aria-label="筛选往来方向" value={direction} onChange={e => { setDirection(e.target.value); setPage(1); }}><option value="">客户及供应商</option><option value="CUSTOMER">客户</option><option value="SUPPLIER">供应商</option></Select>
    </SearchFilterBar>
    <DataTableCard toolbar={<div className="p-3 text-sm">当前余额 · 共 {total} 个单位方向</div>} pagination={<Pagination page={page} pageSize={pageSize} total={total} onPageChange={setPage} />}>
      <DataTable columns={columns} dataSource={rows} rowKey={row => `${row.partnerId}-${row.direction}`} loading={loading} error={error} minWidth={850} />
    </DataTableCard>
  </div>;
}
