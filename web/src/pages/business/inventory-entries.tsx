import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  getInventoryAdjustment,
  inventoryEntryPage,
  type AdjustmentReason,
  type InventoryAdjustment,
  type InventoryEntry,
  type InventoryEntryReason,
  type InventoryEntryType,
  type InventorySourceType,
} from "@/api/business";
import { ProductSelect } from "@/components/business/master-data-select";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { Field } from "@/components/common/field";
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
import { businessMidnightToUtc, formatDateTime } from "@/lib/datetime";
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
const tradeReasonOptions: DictSelectOption<InventoryEntryReason>[] = [
  { value: "PURCHASE", label: "采购入库" }, { value: "PURCHASE_CANCEL", label: "采购入库取消" },
  { value: "SALE", label: "销售出库" }, { value: "SALE_CANCEL", label: "销售出库取消" },
  { value: "PURCHASE_RETURN", label: "采购退货" }, { value: "PURCHASE_RETURN_CANCEL", label: "采购退货取消" },
  { value: "SALE_RETURN", label: "销售退货" }, { value: "SALE_RETURN_CANCEL", label: "销售退货取消" },
];
const sourceOptions: { value: InventorySourceType; label: string }[] = [
  { value: "PURCHASE", label: "采购入库" }, { value: "SALE", label: "销售出库" },
  { value: "PURCHASE_RETURN", label: "采购退货" }, { value: "SALE_RETURN", label: "销售退货" },
  { value: "ADJUSTMENT", label: "库存调整" },
];
const sourceLabels = Object.fromEntries(sourceOptions.map((option) => [option.value, option.label])) as Record<InventorySourceType, string>;
type Filters = { productId: string; productKeyword: string; sourceType: string; entryType: string; occurredFrom: string; occurredTo: string };
const blankFilters: Filters = { productId: "", productKeyword: "", sourceType: "", entryType: "", occurredFrom: "", occurredTo: "" };

export function InventoryEntriesPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate=useNavigate();
  const reasonDict = useDictOptions<AdjustmentReason>(DICT_CODES.INVENTORY_ADJUSTMENT_REASON, {
    allowedValues: INVENTORY_ADJUSTMENT_REASON_VALUES, fallback: reasonFallback,
  });
  const [records, setRecords] = useState<InventoryEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [draftFilters, setDraftFilters] = useState<Filters>(() => ({ ...blankFilters, productId: searchParams.get("productId") ?? "" }));
  const [filters, setFilters] = useState<Filters>(() => ({ ...blankFilters, productId: searchParams.get("productId") ?? "" }));
  const [reload, setReload] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [filterError, setFilterError] = useState("");
  const [actionError, setActionError] = useState("");
  const [detail, setDetail] = useState<InventoryAdjustment | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const missingReasons = missingBusinessDictValues(reasonDict.options, INVENTORY_ADJUSTMENT_REASON_VALUES);
  const reasonOptions = missingReasons.length ? reasonFallback : reasonDict.options;
  const entryReasonOptions = [...reasonOptions, ...tradeReasonOptions];

  useEffect(() => {
    let active = true;
    setLoading(true);
    void inventoryEntryPage({
      page, pageSize: PAGE_SIZE, productId: filters.productId, productKeyword: filters.productKeyword, sourceType: filters.sourceType, entryType: filters.entryType,
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
    setDraftFilters((current) => ({ ...current, productId: linkedProductId }));
    setFilters((current) => ({ ...current, productId: linkedProductId }));
    setPage(1);
  }, [linkedProductId, filters.productId]);

  const columns: DataTableColumn<InventoryEntry>[] = [
    { title: "生效时间", dataIndex: "occurredAt", nowrap: true, render: (value) => formatDateTime(String(value ?? "")) },
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
      nowrap: true,
      render: (value) => <span className={String(value ?? "").startsWith("-") ? "tabular-nums text-danger" : "tabular-nums text-success"}>{String(value ?? "")}</span>,
    },
    { title: "调整前 → 调整后", key: "balance", nowrap: true, render: (_, record) => <span className="tabular-nums">{record.balanceBefore} → {record.balanceAfter}</span> },
    { title: "业务来源", dataIndex: "sourceType", nowrap: true, render: (value, row) => `${sourceLabels[value as InventorySourceType] ?? String(value ?? "")}${row.directDelivery ? " · 直送" : ""}` },
    { title: "类型", dataIndex: "entryType", nowrap: true, render: (value) => businessDictLabel(entryTypeFallback, value as InventoryEntryType) },
    { title: "原因", dataIndex: "reason", nowrap: true, render: (value) => businessDictLabel(entryReasonOptions, value as InventoryEntryReason) },
    { title: "说明", dataIndex: "remark", render: (value) => <span className="whitespace-pre-wrap">{value || "-"}</span> },
    { title: "操作人", key: "operator", nowrap: true, render: (_, record) => record.operatorName || `用户 ${record.operatorId}` },
    { title: "来源单号", dataIndex: "documentNo", nowrap: true, render: (value, record) => <button type="button" title={String(value ?? "")} className="block max-w-[12rem] truncate text-left text-primary hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2" onClick={() => openSource(record)}>{String(value ?? "")}</button> },
  ];

  function openSource(record: InventoryEntry) {
    if (record.saleReturnId) navigate(`/business/sale-returns?saleReturnId=${record.saleReturnId}`);
    else if (record.purchaseReturnId) navigate(`/business/purchase-returns?purchaseReturnId=${record.purchaseReturnId}`);
    else if (record.saleId) navigate(`/business/sales?saleId=${record.saleId}`);
    else if (record.purchaseId) navigate(`/business/purchases?purchaseId=${record.purchaseId}`);
    else if (record.adjustmentId) void openSourceDocument(record.adjustmentId);
  }

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
  <SearchFilterBar onSubmit={() => applyFilters(draftFilters)} actions={<><Button variant="secondary" onClick={() => applyFilters(blankFilters)}>重置</Button><Button onClick={() => applyFilters(draftFilters)}>查询</Button></>}>
        <Field label="商品"><ProductSelect inlineBrowse label="筛选商品" placeholder="搜索商品编码、名称、型号或规格" historical value={Number(draftFilters.productId)} keyword={draftFilters.productKeyword} onDraftKeywordChange={(productKeyword) => setDraftFilters((current) => ({ ...current, productKeyword, productId: "" }))} onKeywordChange={(productKeyword) => applyFilters({ ...draftFilters, productKeyword, productId: "" })} onChange={(product) => setDraftFilters((current) => ({ ...current, productId: product ? String(product.id) : "", productKeyword: "" }))} /></Field>
        <Field label="业务来源"><Select aria-label="业务来源" value={draftFilters.sourceType} onChange={(event) => setDraftFilters({ ...draftFilters, sourceType: event.target.value })}>
          <option value="">全部来源</option>
          {sourceOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select></Field>
        <Field label="流水类型"><Select aria-label="流水类型" value={draftFilters.entryType} onChange={(event) => setDraftFilters({ ...draftFilters, entryType: event.target.value })}>
          <option value="">全部类型</option>
          {entryTypeFallback.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select></Field>
        <Field label="生效时间起（含）"><Input type="date" value={draftFilters.occurredFrom} onChange={(event) => setDraftFilters({ ...draftFilters, occurredFrom: event.target.value })} /></Field>
        <Field label="生效时间止（含当日）"><Input type="date" value={draftFilters.occurredTo} onChange={(event) => setDraftFilters({ ...draftFilters, occurredTo: event.target.value })} /></Field>
      </SearchFilterBar>
      {(reasonDict.error || missingReasons.length > 0) && <div role="alert" className="mb-3 flex items-center justify-between gap-3 rounded-admin border border-warning/30 bg-warning/5 p-3 text-sm text-text-secondary"><span>原因选项暂不可用，当前显示默认选项。</span><Button size="sm" variant="secondary" onClick={reasonDict.reload}>重试</Button></div>}
      {filterError && <p role="alert" className="mb-3 text-sm text-danger">{filterError}</p>}
      {actionError && <p role="alert" className="mb-3 text-sm text-danger">{actionError}</p>}
      <DataTableCard
        toolbar={<TableToolbar title="流水列表" description="历史描述为过账时的商品资料快照，不随当前档案编辑变化；当前库存请查看“当前库存”页面。" actions={<span className="text-sm text-text-tertiary">共 {total} 条</span>} />}
        pagination={<Pagination page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} />}
      >
        <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} error={error} minWidth={1040} />
      </DataTableCard>
      {detailLoading && <div role="status" className="fixed inset-0 z-50 flex items-center justify-center bg-black/20"><div className="rounded-admin bg-surface p-6">正在加载来源调整单…</div></div>}
      {detail && <InventoryAdjustmentDetail adjustment={detail} onClose={() => setDetail(null)} />}
    </>
  );
}

const localMidnightToUtc = businessMidnightToUtc;
const localNextMidnightToUtc = (date: string) => businessMidnightToUtc(date, true);
