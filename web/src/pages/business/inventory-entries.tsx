import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  getInventoryAdjustment,
  getProduct,
  inventoryEntryPage,
  productPage,
  type AdjustmentReason,
  type InventoryAdjustment,
  type InventoryEntry,
  type InventoryEntryType,
  type ProductRecord,
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
  DICT_CODES,
  INVENTORY_ADJUSTMENT_REASON_VALUES,
  type DictSelectOption,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { businessDictLabel, missingBusinessDictValues } from "@/lib/business-dict-label";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";
import { InventoryAdjustmentDetail } from "@/pages/business/inventory-adjustment-detail";

const PAGE_SIZE = 10;
const reasonFallback: DictSelectOption<AdjustmentReason>[] = [
  { value: "OPENING", label: "期初录入" }, { value: "SURPLUS", label: "盘盈" },
  { value: "SHORTAGE", label: "盘亏" }, { value: "DAMAGE", label: "报损" }, { value: "OTHER", label: "其他" },
];
const entryTypeFallback: DictSelectOption<InventoryEntryType>[] = [
  { value: "ORIGINAL", label: "原始变动" }, { value: "REVERSAL", label: "取消冲销" },
];
type Filters = { productId: string; entryType: string; occurredFrom: string; occurredTo: string };
const blankFilters: Filters = { productId: "", entryType: "", occurredFrom: "", occurredTo: "" };

export function InventoryEntriesPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const reasonDict = useDictOptions<AdjustmentReason>(DICT_CODES.INVENTORY_ADJUSTMENT_REASON, {
    allowedValues: INVENTORY_ADJUSTMENT_REASON_VALUES, fallback: reasonFallback,
  });
  const [records, setRecords] = useState<InventoryEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [draftFilters, setDraftFilters] = useState<Filters>(() => ({ ...blankFilters, productId: searchParams.get("productId") ?? "" }));
  const [filters, setFilters] = useState<Filters>(() => ({ ...blankFilters, productId: searchParams.get("productId") ?? "" }));
  const [productFilter, setProductFilter] = useState<ProductRecord | null>(null);
  const [reload, setReload] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [filterError, setFilterError] = useState("");
  const [actionError, setActionError] = useState("");
  const [detail, setDetail] = useState<InventoryAdjustment | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const missingReasons = missingBusinessDictValues(reasonDict.options, INVENTORY_ADJUSTMENT_REASON_VALUES);
  const reasonOptions = missingReasons.length ? reasonFallback : reasonDict.options;

  useEffect(() => {
    let active = true;
    setLoading(true);
    void inventoryEntryPage({
      page, pageSize: PAGE_SIZE, productId: filters.productId, entryType: filters.entryType,
      // The list takes whole local days; the API compares instants, so the
      // bounds become the local day start and the next day's start in UTC.
      occurredFrom: filters.occurredFrom ? localMidnightToUtc(filters.occurredFrom) : "",
      occurredTo: filters.occurredTo ? localNextMidnightToUtc(filters.occurredTo) : "",
    }).then((result) => {
      if (!active) return;
      setRecords(result.records);
      setTotal(result.total);
      setError("");
    }).catch((reason: unknown) => {
      if (!active) return;
      setRecords([]); setTotal(0); setError(reason instanceof Error ? reason.message : "加载库存流水失败");
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [filters, page, reload]);

  // The link from the current-stock page carries the product as a query
  // parameter. A navigation while this page is already mounted does not re-run
  // the initial state, so the parameter is followed here too.
  const linkedProductId = searchParams.get("productId") ?? "";
  useEffect(() => {
    if (linkedProductId === filters.productId) return;
    setProductFilter(null);
    setDraftFilters((current) => ({ ...current, productId: linkedProductId }));
    setFilters((current) => ({ ...current, productId: linkedProductId }));
    setPage(1);
  }, [linkedProductId, filters.productId]);

  // Entering from the current-stock page pre-selects the product by its internal
  // ID, so the linked history keeps working after a rename or a status change.
  useEffect(() => {
    const id = filters.productId;
    if (!id || productFilter) return;
    let active = true;
    void getProduct(Number(id)).then((product) => {
      if (active) setProductFilter(product);
    }).catch(() => { if (active) setProductFilter(null); });
    return () => { active = false; };
  }, [filters.productId, productFilter]);

  const columns: DataTableColumn<InventoryEntry>[] = [
    { title: "生效时间", dataIndex: "occurredAt", render: (value) => formatDateTime(String(value ?? "")) },
    {
      title: "商品快照",
      key: "product",
      render: (_, record) => (
        <div>
          <div>{record.productName}</div>
          <div className="text-xs text-text-tertiary">{record.productCode} · {record.productModel || "-"} / {record.productSpecification || "-"} · {record.unit}</div>
        </div>
      ),
    },
    {
      title: "变动",
      dataIndex: "quantity",
      render: (value) => <span className={String(value ?? "").startsWith("-") ? "tabular-nums text-danger" : "tabular-nums text-success"}>{String(value ?? "")}</span>,
    },
    { title: "调整前 → 调整后", key: "balance", render: (_, record) => <span className="tabular-nums">{record.balanceBefore} → {record.balanceAfter}</span> },
    { title: "类型", dataIndex: "entryType", render: (value) => businessDictLabel(entryTypeFallback, value as InventoryEntryType) },
    { title: "原因", dataIndex: "reason", render: (value) => businessDictLabel(reasonOptions, value as AdjustmentReason) },
    { title: "说明", dataIndex: "remark", render: (value) => <span className="whitespace-pre-wrap">{value || "-"}</span> },
    { title: "操作人", key: "operator", render: (_, record) => record.operatorName || `用户 ${record.operatorId}` },
    { title: "来源单号", dataIndex: "documentNo", render: (value, record) => <button className="text-primary hover:underline" onClick={() => void openSourceDocument(record.adjustmentId)}>{String(value ?? "")}</button> },
  ];

  async function openSourceDocument(id: number) {
    setDetailLoading(true); setDetail(null); setActionError("");
    try { setDetail(await getInventoryAdjustment(id)); }
    catch (reason) { setActionError(reason instanceof Error ? reason.message : "读取来源调整单失败"); }
    finally { setDetailLoading(false); }
  }
  function applyFilters(next: Filters) {
    if (next.occurredFrom && next.occurredTo && next.occurredFrom > next.occurredTo) {
      setFilterError("生效时间起始日期不能晚于结束日期。请调整日期后再查询。");
      return;
    }
    setFilterError("");
    setFilters(next);
    setPage(1);
    setSearchParams(next.productId ? { productId: next.productId } : {}, { replace: true });
    setReload((value) => value + 1);
  }

  return (
    <>
      <PageHeader title="库存流水" description="追溯实物商品每一次已生效的库存变动。流水不可修改或删除，取消已过账单据会追加反向流水。" />
      <SearchFilterBar actions={<><Button variant="secondary" onClick={() => { setProductFilter(null); applyFilters(blankFilters); }}>重置</Button><Button onClick={() => applyFilters(draftFilters)}>查询</Button></>}>
        <ProductFilter selected={productFilter} onChoose={(product) => { setProductFilter(product); setDraftFilters({ ...draftFilters, productId: String(product.id) }); }} onClear={() => { setProductFilter(null); setDraftFilters({ ...draftFilters, productId: "" }); }} />
        <Select aria-label="流水类型" value={draftFilters.entryType} onChange={(event) => setDraftFilters({ ...draftFilters, entryType: event.target.value })}>
          <option value="">全部类型</option>
          {entryTypeFallback.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select>
        <label className="grid gap-1 text-xs text-text-secondary">生效时间起（含）<Input type="date" value={draftFilters.occurredFrom} onChange={(event) => setDraftFilters({ ...draftFilters, occurredFrom: event.target.value })} /></label>
        <label className="grid gap-1 text-xs text-text-secondary">生效时间止（含当日）<Input type="date" value={draftFilters.occurredTo} onChange={(event) => setDraftFilters({ ...draftFilters, occurredTo: event.target.value })} /></label>
      </SearchFilterBar>
      {(reasonDict.error || missingReasons.length > 0) && <div role="alert" className="mb-3 flex items-center justify-between gap-3 rounded-admin border border-warning/30 bg-warning/5 p-3 text-sm text-text-secondary"><span>原因选项暂不可用，当前显示默认选项。</span><Button size="sm" variant="secondary" onClick={reasonDict.reload}>重试</Button></div>}
      {filterError && <p role="alert" className="mb-3 text-sm text-danger">{filterError}</p>}
      {actionError && <p role="alert" className="mb-3 text-sm text-danger">{actionError}</p>}
      <DataTableCard
        toolbar={<TableToolbar title="流水列表" description="历史描述为过账时的商品资料快照，不随当前档案编辑变化；当前库存请查看“当前库存”页面。" actions={<span className="text-sm text-text-tertiary">共 {total} 条</span>} />}
        pagination={<Pagination page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} />}
      >
        <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} error={error} />
      </DataTableCard>
      {detailLoading && <div role="status" className="fixed inset-0 z-50 flex items-center justify-center bg-black/20"><div className="rounded-admin bg-surface p-6">正在加载来源调整单…</div></div>}
      {detail && <InventoryAdjustmentDetail adjustment={detail} onClose={() => setDetail(null)} />}
    </>
  );
}

function ProductFilter({ selected, onChoose, onClear }: { selected: ProductRecord | null; onChoose: (product: ProductRecord) => void; onClear: () => void }) {
  const [keyword, setKeyword] = useState("");
  const [activeKeyword, setActiveKeyword] = useState("");
  const [page, setPage] = useState(1);
  const [run, setRun] = useState(0);
  const [products, setProducts] = useState<ProductRecord[]>([]);
  useEffect(() => {
    let active = true;
    void productPage({ page, pageSize: 10, keyword: activeKeyword }).then((result) => {
      if (!active) return;
      setProducts(result.records);
    }).catch(() => { if (active) setProducts([]); });
    return () => { active = false; };
  }, [activeKeyword, page, run]);
  let options = products;
  if (selected && !products.some((product) => product.id === selected.id)) options = [selected, ...products];
  return <div className="grid min-w-[240px] gap-1">
    <div className="flex gap-1">
      <Input placeholder={selected ? `${selected.code} · ${selected.name}` : "搜索商品编码或名称"} value={keyword} onChange={(event) => setKeyword(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); setActiveKeyword(keyword.trim()); setPage(1); setRun((value) => value + 1); } }} />
      <Button type="button" size="sm" variant="secondary" onClick={() => { setActiveKeyword(keyword.trim()); setPage(1); setRun((value) => value + 1); }}>搜索</Button>
    </div>
    <Select aria-label="按商品筛选" value={selected ? String(selected.id) : ""} onChange={(event) => { if (!event.target.value) { onClear(); return; } const product = options.find((item) => item.id === Number(event.target.value)); if (product) onChoose(product); }}>
      <option value="">全部商品</option>
      {options.map((product) => <option key={product.id} value={product.id}>{product.code} · {product.name}</option>)}
    </Select>
    {selected && <button type="button" className="justify-self-start text-xs text-primary hover:underline" onClick={onClear}>清除商品筛选</button>}
  </div>;
}

function localMidnightToUtc(date: string) { return new Date(`${date}T00:00:00`).toISOString(); }
function localNextMidnightToUtc(date: string) {
  const next = new Date(`${date}T00:00:00`); next.setDate(next.getDate() + 1); return next.toISOString();
}
