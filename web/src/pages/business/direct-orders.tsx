import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { directOrderPage, directStatus, type DirectOrder } from "@/api/direct-orders";
import { PageHeader } from "@/components/common/page-header";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { DataTableCard } from "@/components/common/data-table-card";
import { DataTable } from "@/components/common/data-table";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Pagination } from "@/components/common/pagination";
import { Field } from "@/components/common/field";
import { PartnerSelect } from "@/components/business/master-data-select";
import { DocumentRowActions } from "@/components/business/document-row-actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { DataTableColumn } from "@/types";
const empty = { documentNo: "", supplierId: 0, customerId: 0, status: "", businessFrom: "", businessTo: "" };
export function DirectOrdersPage() {
  const navigate = useNavigate();
  const [filters, setFilters] = useState(empty);
  const [query, setQuery] = useState(empty);
  const [page, setPage] = useState(1);
  const [rows, setRows] = useState<DirectOrder[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true); setError("");
    void directOrderPage({ ...query, page, pageSize: 10 }).then(result => {
      if (active) { setRows(result.records); setTotal(result.total); }
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : "直送单加载失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [query, page, reload]);
  const columns: DataTableColumn<DirectOrder>[] = [
    { title: "供应商", dataIndex: "supplierName" }, { title: "客户", dataIndex: "customerName" },
    { title: "业务日期", dataIndex: "businessDate", nowrap: true },
    { title: "采购金额", dataIndex: "purchaseTotal", align: "right", render: v => `¥${v}` },
    { title: "销售金额", dataIndex: "saleTotal", align: "right", render: v => `¥${v}` },
    { title: "状态", dataIndex: "status", nowrap: true, render: (_, row) => directStatus[row.status] },
    { title: "操作", key: "actions", nowrap: true, render: (_, row) => <DocumentRowActions documentNo={row.documentNo}><Button size="sm" onClick={() => navigate(`/business/direct-orders/${row.id}`)}>详情</Button>{row.status === "DRAFT" && <Button size="sm" onClick={() => navigate(`/business/direct-orders/${row.id}/edit`)}>编辑</Button>}</DocumentRowActions> },
  ];
  function search() { setPage(1); setQuery({ ...filters }); }
  return <div className="space-y-space-4">
    <PageHeader title="直送单" description="供应商直接送货给客户；草稿保存不改变库存或往来。" actions={<Button variant="primary" onClick={() => navigate("/business/direct-orders/new")}>新建直送单</Button>} />
    <SearchFilterBar onSubmit={search} actions={<><Button variant="primary" onClick={search}>查询</Button><Button onClick={() => { setFilters(empty); setQuery(empty); setPage(1); }}>重置</Button></>}>
      <Field label="直送单号" htmlFor="direct-query-no"><Input id="direct-query-no" value={filters.documentNo} onChange={e => setFilters({ ...filters, documentNo: e.target.value })} /></Field>
      <Field label="供应商"><PartnerSelect inlineBrowse historical identity="SUPPLIER" label="供应商筛选" value={filters.supplierId} onChange={p => setFilters({ ...filters, supplierId: p?.id ?? 0 })} /></Field>
      <Field label="客户"><PartnerSelect inlineBrowse historical identity="CUSTOMER" label="客户筛选" value={filters.customerId} onChange={p => setFilters({ ...filters, customerId: p?.id ?? 0 })} /></Field>
      <Field label="状态" htmlFor="direct-query-status"><Select id="direct-query-status" value={filters.status} onChange={e => setFilters({ ...filters, status: e.target.value })}><option value="">全部</option><option value="DRAFT">草稿</option><option value="POSTED">已确认</option><option value="CANCELLED">已取消</option></Select></Field>
      <Field label="业务日期起" htmlFor="direct-query-from"><Input id="direct-query-from" type="date" value={filters.businessFrom} onChange={e => setFilters({ ...filters, businessFrom: e.target.value })} /></Field>
      <Field label="业务日期止" htmlFor="direct-query-to"><Input id="direct-query-to" type="date" value={filters.businessTo} onChange={e => setFilters({ ...filters, businessTo: e.target.value })} /></Field>
    </SearchFilterBar>
    <DataTableCard toolbar={<TableToolbar title="直送单列表" actions={<Button disabled={loading} onClick={() => setReload(n => n + 1)}>刷新</Button>} />} pagination={<Pagination page={page} pageSize={10} total={total} onPageChange={setPage} disabled={loading} />}>
      <DataTable columns={columns} dataSource={rows} rowKey="id" loading={loading} error={error ? <span role="alert">{error}</span> : undefined} />
    </DataTableCard>
  </div>;
}
