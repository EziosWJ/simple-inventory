import { useEffect, useState } from "react";
import {
  inventoryBalancePage,
  type InventoryBalance,
} from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  BUSINESS_STATUS_VALUES,
  DICT_CODES,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { businessDictLabel, missingBusinessDictValues } from "@/lib/business-dict-label";
import type { ApiStatus, DataTableColumn } from "@/types";

const PAGE_SIZE = 10;
type StockScope = "nonzero" | "all" | "zero";
const stockScopes: { value: StockScope; label: string }[] = [
  { value: "nonzero", label: "非零库存" },
  { value: "all", label: "全部商品" },
  { value: "zero", label: "零库存" },
];
type Filters = { keyword: string; category: string; status: string; stock: StockScope };
const blankFilters: Filters = { keyword: "", category: "", status: "", stock: "nonzero" };

export function InventoryBalancesPage() {
  const businessStatusDict = useDictOptions<ApiStatus>(DICT_CODES.BUSINESS_STATUS, {
    allowedValues: BUSINESS_STATUS_VALUES,
    valueType: "number",
  });
  const [records, setRecords] = useState<InventoryBalance[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [draftFilters, setDraftFilters] = useState<Filters>(blankFilters);
  const [filters, setFilters] = useState<Filters>(blankFilters);
  const [reload, setReload] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const missingStatuses = missingBusinessDictValues(businessStatusDict.options, BUSINESS_STATUS_VALUES);
  const statusIssue = businessStatusDict.error || (missingStatuses.length ? `字典缺少可用值：${missingStatuses.join("、")}` : "");

  useEffect(() => {
    let active = true;
    setLoading(true);
    void inventoryBalancePage({
      page, pageSize: PAGE_SIZE, keyword: filters.keyword, category: filters.category,
      status: filters.status, stock: filters.stock,
    }).then((result) => {
      if (!active) return;
      setRecords(result.records);
      setTotal(result.total);
      setError("");
    }).catch((reason: unknown) => {
      if (!active) return;
      setRecords([]); setTotal(0); setError(reason instanceof Error ? reason.message : "加载当前库存失败");
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [filters, page, reload]);

  const columns: DataTableColumn<InventoryBalance>[] = [
    {
      title: "商品",
      key: "product",
      render: (_, record) => (
        <div>
          <div className="font-medium">{record.name}</div>
          <div className="text-xs text-text-tertiary">{record.code}</div>
        </div>
      ),
    },
    { title: "型号 / 规格", key: "specification", render: (_, record) => `${record.model || "-"} / ${record.specification || "-"}` },
    { title: "分类", dataIndex: "category", render: (value) => value || "-" },
    { title: "基本单位", dataIndex: "unit" },
    { title: "当前数量", dataIndex: "quantity", render: (value) => <span className="tabular-nums font-medium">{String(value ?? "")}</span> },
    { title: "状态", dataIndex: "status", render: (value) => businessDictLabel(businessStatusDict.options, value) },
  ];

  function applyFilters(next: Filters) {
    setDraftFilters(next);
    setFilters(next);
    setPage(1);
    setReload((value) => value + 1);
  }

  return (
    <>
      <PageHeader title="当前库存" description="查看实物商品的实际数量。数量由库存调整单过账产生，停用商品仍会显示。" />
      <SearchFilterBar actions={<><Button variant="secondary" onClick={() => applyFilters(blankFilters)}>重置</Button><Button onClick={() => applyFilters(draftFilters)}>查询</Button></>}>
        <Input placeholder="编码、名称、型号或规格" value={draftFilters.keyword} onChange={(event) => setDraftFilters({ ...draftFilters, keyword: event.target.value })} />
        <Input placeholder="分类" value={draftFilters.category} onChange={(event) => setDraftFilters({ ...draftFilters, category: event.target.value })} />
        <Select aria-label="启用状态" value={draftFilters.status} onChange={(event) => setDraftFilters({ ...draftFilters, status: event.target.value })}>
          <option value="">全部状态</option>
          {businessStatusDict.options.map((option) => <option key={option.value} value={String(option.value)}>{option.label}</option>)}
        </Select>
        <Select aria-label="库存范围" value={draftFilters.stock} onChange={(event) => setDraftFilters({ ...draftFilters, stock: event.target.value as StockScope })}>
          {stockScopes.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select>
      </SearchFilterBar>
      {statusIssue && <div role="alert" className="mb-3 flex items-center justify-between gap-3 rounded-admin border border-warning/30 bg-warning/5 p-3 text-sm text-text-secondary"><span>{DICT_CODES.BUSINESS_STATUS}：{statusIssue}</span><Button size="sm" variant="secondary" onClick={businessStatusDict.reload}>重试</Button></div>}
      <DataTableCard
        toolbar={<TableToolbar title="库存列表" description={stockDescription(filters.stock)} actions={<span className="text-sm text-text-tertiary">共 {total} 项</span>} />}
        pagination={<Pagination page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} />}
      >
        <DataTable columns={columns} dataSource={records} rowKey="productId" loading={loading} error={error} />
      </DataTableCard>
    </>
  );
}

function stockDescription(scope: StockScope) {
  switch (scope) {
    case "all":
      return "包含未建立库存记录的商品，未建立记录的实物按 0 显示；服务项目不参与库存，不在此列表。";
    case "zero":
      return "显示当前数量为 0 的商品，包含从未过账过的实物；服务项目不参与库存，不在此列表。";
    default:
      return "默认只显示当前数量大于 0 的商品，包含已停用商品；服务项目不参与库存，不在此列表。";
  }
}
