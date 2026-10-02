import { useEffect, useState } from "react";
import {
  createInventoryAdjustment,
  cancelInventoryAdjustment,
  getInventoryAdjustment,
  getProduct,
  inventoryAdjustmentPage,
  postInventoryAdjustment,
  productPage,
  updateInventoryAdjustment,
  type AdjustmentReason,
  type AdjustmentStatus,
  type InventoryAdjustment,
  type InventoryAdjustmentDraftItem,
  type InventoryAdjustmentItem,
  type ProductRecord,
} from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  DICT_CODES,
  INVENTORY_ADJUSTMENT_REASON_VALUES,
  INVENTORY_ADJUSTMENT_STATUS_VALUES,
  type DictSelectOption,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { useBusinessLeaveGuard } from "@/hooks/use-business-leave-guard";
import { BusinessLeaveConfirm } from "@/components/business/business-leave-confirm";
import { businessDictLabel, missingBusinessDictValues } from "@/lib/business-dict-label";
import { isApiError } from "@/lib/api-error";
import { InventoryAdjustmentDetail } from "@/pages/business/inventory-adjustment-detail";
import type { DataTableColumn } from "@/types";

const PAGE_SIZE = 10;
const reasonFallback: DictSelectOption<AdjustmentReason>[] = [
  { value: "OPENING", label: "期初录入" }, { value: "SURPLUS", label: "盘盈" },
  { value: "SHORTAGE", label: "盘亏" }, { value: "DAMAGE", label: "报损" }, { value: "OTHER", label: "其他" },
];
const statusFallback: DictSelectOption<AdjustmentStatus>[] = [
  { value: "DRAFT", label: "草稿" }, { value: "POSTED", label: "已过账" }, { value: "CANCELLED", label: "已取消" },
];
type Filters = { documentNo: string; status: string; productId: string; createdFrom: string; createdTo: string };
const blankFilters: Filters = { documentNo: "", status: "", productId: "", createdFrom: "", createdTo: "" };
type DraftLine = { key: number; product: ProductRecord | null; savedSnapshot: boolean; quantity: string; reason: AdjustmentReason | ""; remark: string };
let nextLineKey = 1;

export function InventoryAdjustmentsPage() {
  const statusDict = useDictOptions<AdjustmentStatus>(DICT_CODES.INVENTORY_ADJUSTMENT_STATUS, {
    allowedValues: INVENTORY_ADJUSTMENT_STATUS_VALUES, fallback: statusFallback,
  });
  const reasonDict = useDictOptions<AdjustmentReason>(DICT_CODES.INVENTORY_ADJUSTMENT_REASON, {
    allowedValues: INVENTORY_ADJUSTMENT_REASON_VALUES, fallback: reasonFallback,
  });
  const [records, setRecords] = useState<InventoryAdjustment[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [draftFilters, setDraftFilters] = useState<Filters>(blankFilters);
  const [productFilter, setProductFilter] = useState<ProductRecord | null>(null);
  const [filterError, setFilterError] = useState("");
  const [filters, setFilters] = useState<Filters>(blankFilters);
  const [reload, setReload] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [editAdjustment, setEditAdjustment] = useState<InventoryAdjustment | null>(null);
  const [cancelAdjustment, setCancelAdjustment] = useState<InventoryAdjustment | null>(null);
  const [postAdjustment, setPostAdjustment] = useState<InventoryAdjustment | null>(null);
  const [detail, setDetail] = useState<InventoryAdjustment | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [postLoading, setPostLoading] = useState(false);
  const [actionError, setActionError] = useState("");

  useEffect(() => {
    let active = true;
    setLoading(true);
    void inventoryAdjustmentPage({
      page, pageSize: PAGE_SIZE, documentNo: filters.documentNo, status: filters.status,
      productId: filters.productId, createdFrom: filters.createdFrom, createdTo: filters.createdTo,
    }).then((result) => {
      if (!active) return;
      setRecords(result.records);
      setTotal(result.total);
      setError("");
    }).catch((reason: unknown) => {
      if (!active) return;
      setRecords([]); setTotal(0); setError(reason instanceof Error ? reason.message : "加载库存调整单失败");
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [filters, page, reload]);

  const columns: DataTableColumn<InventoryAdjustment>[] = [
    { title: "调整单号", dataIndex: "documentNo", render: (value, record) => <button className="text-primary hover:underline" onClick={() => void openDetail(record.id)}>{String(value ?? "")}</button> },
    { title: "状态", dataIndex: "status", render: (value) => businessDictLabel(statusDict.options, value as AdjustmentStatus) },
    { title: "创建人", key: "creator", render: (_, record) => record.createdByName || `用户 ${record.createdBy}` },
    { title: "创建时间", dataIndex: "createTime", render: (value) => formatDateTime(String(value ?? "")) },
    { title: "操作", key: "actions", render: (_, record) => <div className="flex gap-2"><Button size="sm" variant="secondary" onClick={() => void openDetail(record.id)}>查看</Button>{record.status === "DRAFT" && <Button size="sm" onClick={() => void openDetailForPost(record.id)}>过账</Button>}</div> },
  ];

  async function openDetailForPost(id: number) {
    setPostLoading(true); setActionError("");
    try { setPostAdjustment(await getInventoryAdjustment(id)); }
    catch (reason) { setActionError(reason instanceof Error ? reason.message : "读取库存调整单详情失败"); }
    finally { setPostLoading(false); }
  }
  async function openDetail(id: number) {
    setDetailLoading(true); setDetail(null); setActionError("");
    try { setDetail(await getInventoryAdjustment(id)); }
    catch (reason) { setActionError(reason instanceof Error ? reason.message : "读取库存调整单详情失败"); }
    finally { setDetailLoading(false); }
  }
  function applyFilters(next: Filters) {
    if (next.createdFrom && next.createdTo && next.createdFrom > next.createdTo) {
      setFilterError("创建时间起始日期不能晚于结束日期。请调整日期后再查询。");
      return;
    }
    setFilterError("");
    setDraftFilters(next); setFilters(toApiFilters(next)); setPage(1); setReload((value) => value + 1);
  }
  async function saveDraft(items: InventoryAdjustmentDraftItem[]) {
    await createInventoryAdjustment({ items });
    setCreateOpen(false); setPage(1); setReload((value) => value + 1);
  }
  function openEdit(adjustment: InventoryAdjustment) {
    setDetail(null);
    setEditAdjustment(adjustment);
  }
  function openCancel(adjustment: InventoryAdjustment) {
    setDetail(null);
    setCancelAdjustment(adjustment);
  }
  function openPost(adjustment: InventoryAdjustment) {
    setDetail(null);
    setPostAdjustment(adjustment);
  }
  function openCancelPosted(adjustment: InventoryAdjustment) {
    setDetail(null);
    setCancelAdjustment(adjustment);
  }
  function finishEdit(adjustment: InventoryAdjustment) {
    setEditAdjustment(null);
    setDetail(adjustment);
    setPage(1);
    setReload((value) => value + 1);
  }
  function finishCancel(adjustment: InventoryAdjustment) {
    setCancelAdjustment(null);
    setDetail(adjustment);
    setPage(1);
    setReload((value) => value + 1);
  }
  function finishPost(adjustment: InventoryAdjustment) {
    setPostAdjustment(null);
    setDetail(adjustment);
    setPage(1);
    setReload((value) => value + 1);
  }
  async function reloadLatest(id: number) {
    const latest = await getInventoryAdjustment(id);
    setEditAdjustment(null);
    setCancelAdjustment(null);
    setPostAdjustment(null);
    setDetail(latest);
    setReload((value) => value + 1);
  }
  const missingStatuses = missingBusinessDictValues(statusDict.options, INVENTORY_ADJUSTMENT_STATUS_VALUES);
  const missingReasons = missingBusinessDictValues(reasonDict.options, INVENTORY_ADJUSTMENT_REASON_VALUES);
  const statusOptions = missingStatuses.length ? statusFallback : statusDict.options;
  const reasonOptions = missingReasons.length ? reasonFallback : reasonDict.options;
  const dictIssues = [
    (statusDict.error || missingStatuses.length > 0) && "状态选项暂不可用，当前显示默认选项。",
    (reasonDict.error || missingReasons.length > 0) && "原因选项暂不可用，当前显示默认选项。",
  ].filter(Boolean);

  return <>
    <PageHeader title="库存调整" description="查询库存调整单，登记期初、盘盈、盘亏或报损。草稿保存不改变库存。" actions={<Button onClick={() => { setActionError(""); setCreateOpen(true); }}>新建调整单</Button>} />
    <SearchFilterBar actions={<><Button variant="secondary" onClick={() => { setProductFilter(null); applyFilters(blankFilters); }}>重置</Button><Button onClick={() => applyFilters(draftFilters)}>查询</Button></>}>
      <Input placeholder="调整单号" value={draftFilters.documentNo} onChange={(event) => setDraftFilters({ ...draftFilters, documentNo: event.target.value })} />
      <Select value={draftFilters.status} onChange={(event) => setDraftFilters({ ...draftFilters, status: event.target.value })}><option value="">全部状态</option>{statusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select>
      <ProductFilter selected={productFilter} onChoose={(product) => { setProductFilter(product); setDraftFilters({ ...draftFilters, productId: String(product.id) }); }} onClear={() => { setProductFilter(null); setDraftFilters({ ...draftFilters, productId: "" }); }} />
      <label className="grid gap-1 text-xs text-text-secondary">创建时间起（含）<Input type="date" value={draftFilters.createdFrom} onChange={(event) => setDraftFilters({ ...draftFilters, createdFrom: event.target.value })} /></label>
      <label className="grid gap-1 text-xs text-text-secondary">创建时间止（含当日）<Input type="date" value={draftFilters.createdTo} onChange={(event) => setDraftFilters({ ...draftFilters, createdTo: event.target.value })} /></label>
    </SearchFilterBar>
    {filterError && <p role="alert" className="mb-3 text-sm text-danger">{filterError}</p>}
    {dictIssues.length > 0 && <div role="alert" className="mb-3 flex items-center justify-between gap-3 rounded-admin border border-warning/30 bg-warning/5 p-3 text-sm text-text-secondary"><span>{dictIssues.join(" ")}</span><Button size="sm" variant="secondary" onClick={() => { statusDict.reload(); reasonDict.reload(); }}>重试</Button></div>}
    {actionError && <p role="alert" className="mb-3 text-sm text-danger">{actionError}</p>}
    <DataTableCard pagination={<Pagination page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} />}>
      <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} error={error} />
    </DataTableCard>
    {createOpen && <AdjustmentItemsDialog reasons={reasonOptions} reasonError={reasonDict.error || (missingReasons.length ? "原因选项暂不可用，当前显示默认选项。" : "")} onRetryReasons={reasonDict.reload} onCancel={() => setCreateOpen(false)} onSave={saveDraft} />}
    {editAdjustment && <AdjustmentItemsDialog key={`edit-${editAdjustment.id}-${editAdjustment.version}`} adjustment={editAdjustment} reasons={reasonOptions} reasonError={reasonDict.error || (missingReasons.length ? "原因选项暂不可用，当前显示默认选项。" : "")} onRetryReasons={reasonDict.reload} onCancel={() => { setEditAdjustment(null); setDetail(editAdjustment); }} onSave={(items) => updateInventoryAdjustment(editAdjustment.id, { version: editAdjustment.version, items })} onSaved={finishEdit} onReloadLatest={() => reloadLatest(editAdjustment.id)} />}
    {cancelAdjustment && <CancelAdjustmentDialog key={`cancel-${cancelAdjustment.id}-${cancelAdjustment.version}`} adjustment={cancelAdjustment} onCancel={() => { setCancelAdjustment(null); setDetail(cancelAdjustment); }} reasonOptions={reasonOptions} onSave={async (reason) => cancelInventoryAdjustment(cancelAdjustment.id, { version: cancelAdjustment.version, reason })} onSaved={finishCancel} onReloadLatest={() => reloadLatest(cancelAdjustment.id)} />}
    {postAdjustment && <PostAdjustmentDialog key={`post-${postAdjustment.id}-${postAdjustment.version}`} adjustment={postAdjustment} reasonOptions={reasonOptions} onCancel={() => { setPostAdjustment(null); setDetail(postAdjustment); }} onSave={() => postInventoryAdjustment(postAdjustment.id, { version: postAdjustment.version })} onSaved={finishPost} onReloadLatest={() => reloadLatest(postAdjustment.id)} />}
    {(detailLoading || postLoading) && <div role="status" className="fixed inset-0 z-50 flex items-center justify-center bg-black/20"><div className="rounded-admin bg-surface p-6">{postLoading ? "正在加载待过账单据…" : "正在加载调整单详情…"}</div></div>}
    {detail && <AdjustmentDetailDialog adjustment={detail} onClose={() => setDetail(null)} onEdit={() => openEdit(detail)} onCancelDraft={() => openCancel(detail)} onPost={() => openPost(detail)} onCancelPosted={() => openCancelPosted(detail)} />}
  </>;
}

function toApiFilters(filters: Filters) {
  const from = filters.createdFrom ? localMidnightToUtc(filters.createdFrom) : "";
  const to = filters.createdTo ? localNextMidnightToUtc(filters.createdTo) : "";
  return { ...filters, createdFrom: from, createdTo: to };
}
function localMidnightToUtc(date: string) { return new Date(`${date}T00:00:00`).toISOString(); }
function localNextMidnightToUtc(date: string) {
  const next = new Date(`${date}T00:00:00`); next.setDate(next.getDate() + 1); return next.toISOString();
}
function formatDateTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(date);
}

function ProductFilter({ selected, onChoose, onClear }: { selected: ProductRecord | null; onChoose: (product: ProductRecord) => void; onClear: () => void }) {
  const [keyword, setKeyword] = useState("");
  const [activeKeyword, setActiveKeyword] = useState("");
  const [page, setPage] = useState(1);
  const [run, setRun] = useState(0);
  const [products, setProducts] = useState<ProductRecord[]>([]);
  const [total, setTotal] = useState(0);
  useEffect(() => {
    let active = true;
    void productPage({ page, pageSize: 10, keyword: activeKeyword }).then((result) => {
      if (!active) return;
      setProducts(result.records); setTotal(result.total);
    }).catch(() => { if (active) { setProducts([]); setTotal(0); } });
    return () => { active = false; };
  }, [activeKeyword, page, run]);
  return <div className="grid min-w-[240px] gap-1">
    <div className="flex gap-1"><Input placeholder={selected ? `${selected.code} · ${selected.name}` : "搜索商品编码或名称"} value={keyword} onChange={(event) => setKeyword(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); setActiveKeyword(keyword.trim()); setPage(1); setRun((value) => value + 1); } }} /><Button type="button" size="sm" variant="secondary" onClick={() => { setActiveKeyword(keyword.trim()); setPage(1); setRun((value) => value + 1); }}>搜索</Button></div>
    <Select aria-label="按商品筛选" value={selected ? String(selected.id) : ""} onChange={(event) => { if (!event.target.value) { onClear(); return; } const product = products.find((item) => item.id === Number(event.target.value)); if (product) onChoose(product); }}><option value="">全部商品</option>{selected && !products.some((item) => item.id === selected.id) && <option value={selected.id}>{selected.code} · {selected.name} · {selected.unit}</option>}{products.map((product) => <option key={product.id} value={product.id}>{product.code} · {product.name} · {product.unit}</option>)}</Select>
    <div className="flex items-center gap-2 text-xs text-text-tertiary"><span>第 {page}/{Math.max(1, Math.ceil(total / 10))} 页</span><button type="button" className="text-primary disabled:text-text-tertiary" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button><button type="button" className="text-primary disabled:text-text-tertiary" disabled={page >= Math.max(1, Math.ceil(total / 10))} onClick={() => setPage((value) => value + 1)}>下一页</button>{selected && <button type="button" className="text-primary" onClick={onClear}>清除</button>}</div>
  </div>;
}

function ProductPicker({ selected, savedSnapshot, onChoose }: { selected: ProductRecord | null; savedSnapshot: boolean; onChoose: (product: ProductRecord) => void }) {
  const [keyword, setKeyword] = useState("");
  const [activeKeyword, setActiveKeyword] = useState("");
  const [page, setPage] = useState(1);
  const [run, setRun] = useState(0);
  const [products, setProducts] = useState<ProductRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [currentProduct, setCurrentProduct] = useState<ProductRecord | null>(null);
  const [currentProductError, setCurrentProductError] = useState(false);
  const selectedId = selected?.id;
  useEffect(() => {
    let active = true;
    setCurrentProduct(null);
    setCurrentProductError(false);
    if (!selectedId || !savedSnapshot) return () => { active = false; };
    void getProduct(selectedId).then((product) => { if (active) setCurrentProduct(product); })
      .catch(() => { if (active) setCurrentProductError(true); });
    return () => { active = false; };
  }, [savedSnapshot, selectedId]);
  useEffect(() => {
    let active = true; setLoading(true);
    void productPage({ page, pageSize: 10, keyword: activeKeyword, type: "GOODS", status: 1 })
      .then((result) => { if (!active) return; setProducts(result.records.filter((product) => product.type === "GOODS" && product.status === 1)); setTotal(result.total); setError(""); })
      .catch((reason: unknown) => { if (!active) return; setProducts([]); setTotal(0); setError(reason instanceof Error ? reason.message : "商品查询失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [activeKeyword, page, run]);
  const pageCount = Math.max(1, Math.ceil(total / 10));
  const currentDiffers = Boolean(savedSnapshot && selected && currentProduct && (
    currentProduct.status !== 1 || currentProduct.type !== "GOODS" ||
    selected.type !== currentProduct.type || selected.unit !== currentProduct.unit
  ));
  const selectedValue = selected ? savedSnapshot ? `snapshot:${selected.id}` : String(selected.id) : "";
  return <div className="grid gap-2">
    <div className="flex gap-2"><Input placeholder="按编码、名称、型号或规格搜索商品" value={keyword} onChange={(event) => setKeyword(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); setActiveKeyword(keyword.trim()); setPage(1); setRun((value) => value + 1); } }} /><Button type="button" variant="secondary" onClick={() => { setActiveKeyword(keyword.trim()); setPage(1); setRun((value) => value + 1); }}>搜索</Button></div>
    <Select aria-label="选择启用实物商品" value={selectedValue} onChange={(event) => { const product = products.find((item) => item.id === Number(event.target.value)); if (product) onChoose(product); }}>
      <option value="">{loading ? "正在查询商品…" : "选择启用的实物商品"}</option>
      {selected && (savedSnapshot || !products.some((item) => item.id === selected.id)) && <option value={selectedValue}>{savedSnapshot ? "原确认" : "已选择"}：{selected.code} · {selected.name} · {selected.model || "无型号"} · {selected.specification || "无规格"} · {selected.unit}</option>}
      {products.map((product) => <option key={product.id} value={product.id}>{product.code} · {product.name} · {product.model || "无型号"} · {product.specification || "无规格"} · {product.unit}</option>)}
    </Select>
    {currentDiffers && selected && <p role="status" className="text-xs text-warning">商品资料已变化；当前仍按原确认单位“{selected.unit}”和类型“{selected.type === "GOODS" ? "实物商品" : "服务"}”保留。搜索并重新选择该商品后，才会按当前资料确认。</p>}
    {savedSnapshot && selected && currentProductError && <p role="status" className="text-xs text-warning">暂时无法核对商品当前资料；当前仍保留原确认单位“{selected.unit}”。</p>}
    <div className="flex items-center justify-between text-xs text-text-tertiary"><span>{error || `共 ${total} 个匹配商品 · 第 ${page}/${pageCount} 页`}</span><span className="flex gap-1"><Button type="button" size="sm" variant="secondary" disabled={page <= 1 || loading} onClick={() => setPage((value) => value - 1)}>上一页</Button><Button type="button" size="sm" variant="secondary" disabled={page >= pageCount || loading} onClick={() => setPage((value) => value + 1)}>下一页</Button><Button type="button" size="sm" variant="secondary" onClick={() => setRun((value) => value + 1)}>重试</Button></span></div>
  </div>;
}

function AdjustmentItemsDialog({ adjustment, reasons, reasonError, onRetryReasons, onCancel, onSave, onSaved, onReloadLatest }: {
  adjustment?: InventoryAdjustment;
  reasons: readonly DictSelectOption<AdjustmentReason>[]; reasonError: string; onRetryReasons: () => void;
  onCancel: () => void; onSave: (items: InventoryAdjustmentDraftItem[]) => Promise<InventoryAdjustment | void>;
  onSaved?: (adjustment: InventoryAdjustment) => void;
  onReloadLatest?: () => Promise<void>;
}) {
  const [initialLines] = useState<DraftLine[]>(() => adjustment?.items?.length ? adjustment.items.map(lineFromSnapshot) : [newLine()]);
  const [lines, setLines] = useState<DraftLine[]>(initialLines);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [versionConflict, setVersionConflict] = useState(false);
  const [confirmReload, setConfirmReload] = useState(false);
  const [reloading, setReloading] = useState(false);
  const leaveGuard = useBusinessLeaveGuard({ dirty: JSON.stringify(lines) !== JSON.stringify(initialLines), busy: loading || reloading });
  function update(key: number, patch: Partial<DraftLine>) { setLines((current) => current.map((line) => line.key === key ? { ...line, ...patch } : line)); }
  async function submit() {
    const issue = validateLines(lines);
    if (issue) { setError(issue); return; }
    setError(""); setLoading(true);
    try {
      const saved = await onSave(lines.map((line) => ({ productId: line.product!.id, productType: line.product!.type, unit: line.product!.unit, quantity: line.quantity.trim(), reason: line.reason as AdjustmentReason, remark: line.remark.trim() || undefined })));
      if (saved && onSaved) onSaved(saved);
    } catch (reason) {
      if (isVersionConflict(reason)) {
        setVersionConflict(true);
        setConfirmReload(false);
        setError("此草稿已被其他操作修改，本次内容没有保存。请检查最新版本后再编辑。");
      } else setError(reason instanceof Error ? reason.message : "保存草稿失败；输入内容已保留，请检查后重试。");
    }
    finally { setLoading(false); }
  }
  async function reloadLatest() {
    if (!onReloadLatest) return;
    setReloading(true);
    try { await onReloadLatest(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "重新加载失败；当前输入仍保留。"); setConfirmReload(false); }
    finally { setReloading(false); }
  }
  const selected = new Set(lines.flatMap((line) => line.product ? [line.product.id] : []));
  return <>
  <FormDialog open title={adjustment ? "编辑库存调整草稿" : "新建库存调整草稿"} description={adjustment ? `单号 ${adjustment.documentNo} · 当前版本 ${adjustment.version}。修改草稿不会改变库存。` : "选择启用实物商品并逐行填写调整数量和原因。保存后单据为草稿，不改变库存。"} loading={loading || reloading} submitDisabled={versionConflict} submitText={adjustment ? "保存修改" : "保存草稿"} onCancel={() => leaveGuard.requestClose(onCancel)} onSubmit={submit} trapFocus={!leaveGuard.open} closeOnEscape={!leaveGuard.open} closeOnOverlayClick={!leaveGuard.open} contentClassName="w-[min(1040px,96vw)]" bodyClassName="overflow-auto">
    {reasonError && <div role="alert" className="mb-3 flex items-center justify-between gap-3 rounded-admin border border-warning/30 p-3 text-sm text-warning"><span>{reasonError}</span><Button type="button" size="sm" variant="secondary" onClick={onRetryReasons}>重试</Button></div>}
    <div className="grid gap-4">{lines.map((line, index) => <section key={line.key} className="grid gap-3 rounded-admin border border-border p-3">
      <div className="flex items-center justify-between"><h3 className="text-sm font-semibold">明细 {index + 1}</h3><Button type="button" size="sm" variant="secondary" disabled={lines.length === 1} onClick={() => setLines((current) => current.filter((item) => item.key !== line.key))}>移除</Button></div>
      <ProductPicker selected={line.product} savedSnapshot={line.savedSnapshot} onChoose={(product) => { if (selected.has(product.id) && line.product?.id !== product.id) { setError(`商品「${product.name}」已在其他明细中选择，同一商品不能重复。`); return; } update(line.key, { product, savedSnapshot: false }); setError(""); }} />
      {line.product && <div className="grid gap-2 text-xs text-text-secondary sm:grid-cols-4"><span>编码：{line.product.code}</span><span>名称：{line.product.name}</span><span>型号/规格：{line.product.model || "-"} / {line.product.specification || "-"}</span><span>单位：{line.product.unit}</span></div>}
      <div className="grid gap-3 sm:grid-cols-3">
        <label className="grid gap-1 text-sm">调整数量（正数增加，负数减少）<Input type="text" inputMode="decimal" placeholder="例如 2 或 -0.125" value={line.quantity} onChange={(event) => update(line.key, { quantity: event.target.value })} /></label>
        <label className="grid gap-1 text-sm">调整原因<Select value={line.reason} onChange={(event) => update(line.key, { reason: event.target.value as AdjustmentReason | "" })}><option value="">请选择</option>{reasons.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select></label>
        <label className="grid gap-1 text-sm">说明{line.reason === "OTHER" ? "（必填）" : "（选填）"}<Input maxLength={500} value={line.remark} onChange={(event) => update(line.key, { remark: event.target.value })} placeholder={line.reason === "OTHER" ? "其他原因须填写说明" : "可补充调整背景"} /></label>
      </div>
      {line.reason && <p className="text-xs text-text-tertiary">{reasonHelp(line.reason)}</p>}
    </section>)}
      <Button type="button" variant="secondary" onClick={() => setLines((current) => [...current, newLine()])}>添加明细</Button>
      <p className="text-xs text-text-tertiary">数量最多三位小数；正数增加，负数减少。</p>
      {versionConflict && (confirmReload ? <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-admin border border-warning/30 bg-warning/5 p-3 text-sm"><span>重新加载会放弃当前未保存的输入。是否继续？</span><span className="flex gap-2"><Button type="button" size="sm" variant="secondary" onClick={() => setConfirmReload(false)}>继续编辑</Button><Button type="button" size="sm" variant="danger" disabled={reloading} onClick={() => void reloadLatest()}>{reloading ? "正在加载…" : "放弃输入并重新加载"}</Button></span></div> : <Button type="button" size="sm" variant="secondary" onClick={() => setConfirmReload(true)}>重新加载最新草稿</Button>)}
      {error && <p role="alert" className="text-sm text-danger">{error}</p>}
    </div>
  </FormDialog>
  <BusinessLeaveConfirm guard={leaveGuard} title={adjustment ? "放弃库存调整修改？" : "放弃新建库存调整单？"} stayText="继续编辑" leaveText="放弃并关闭" />
  </>;
}
function newLine(): DraftLine { return { key: nextLineKey++, product: null, savedSnapshot: false, quantity: "", reason: "", remark: "" }; }
function lineFromSnapshot(item: InventoryAdjustmentItem): DraftLine {
  return {
    key: nextLineKey++,
    product: {
      id: item.productId, code: item.productCode, name: item.productName,
      type: item.productType, model: item.productModel, specification: item.productSpecification,
      unit: item.unit, status: 1,
    },
    savedSnapshot: true,
    quantity: item.quantity,
    reason: item.reason,
    remark: item.remark ?? "",
  };
}
function reasonHelp(reason: AdjustmentReason) {
  if (reason === "OPENING" || reason === "SURPLUS") return "此原因只允许正数增加库存。";
  if (reason === "SHORTAGE" || reason === "DAMAGE") return "此原因只允许负数减少库存。";
  return "其他原因允许正数或负数，必须填写说明。";
}
function validateLines(lines: DraftLine[]) {
  if (lines.length < 1) return "至少添加一条库存调整明细。";
  const ids = lines.flatMap((line) => line.product ? [line.product.id] : []);
  if (ids.length !== lines.length) return "每条明细都必须选择一个启用的实物商品。";
  if (new Set(ids).size !== ids.length) return "同一商品只能在调整单中出现一次。";
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]; const displayIndex = index + 1;
    if (!line.reason) return `明细 ${displayIndex} 请选择调整原因。`;
    const scaled = parseScaledQuantity(line.quantity);
    if (scaled === null) return `明细 ${displayIndex} ${quantityError(line.quantity)}输入内容已保留。`;
    if ((line.reason === "OPENING" || line.reason === "SURPLUS") && scaled < 0n) return `明细 ${displayIndex} 的${reasonHelp(line.reason)}`;
    if ((line.reason === "SHORTAGE" || line.reason === "DAMAGE") && scaled > 0n) return `明细 ${displayIndex} 的${reasonHelp(line.reason)}`;
    if (line.reason === "OTHER" && !line.remark.trim()) return `明细 ${displayIndex} 选择“其他”时必须填写说明。`;
  }
  return "";
}
const MAX_I64 = 9223372036854775807n;
const MIN_I64 = -9223372036854775808n;
function quantityError(raw: string) {
  const value = raw.trim();
  const match = /^([+-]?)(\d+)(?:\.(\d+))?$/.exec(value);
  if (!match) return "请输入有效的十进制数量。";
  if ((match[3]?.length ?? 0) > 3) return "数量最多保留三位小数。";
  if (value.length > 64) return "数量超出允许范围。";
  const fraction = (match[3] ?? "").padEnd(3, "0");
  const magnitude = BigInt(match[2]) * 1000n + BigInt(fraction || "0");
  if (magnitude === 0n) return "数量不能为零。";
  return "数量超出允许范围。";
}
function parseScaledQuantity(raw: string): bigint | null {
  const value = raw.trim();
  const match = /^([+-]?)(\d+)(?:\.(\d{1,3}))?$/.exec(value);
  if (!match) return null;
  const sign = match[1] === "-" ? -1n : 1n;
  const fraction = (match[3] ?? "").padEnd(3, "0");
  const scaled = sign * (BigInt(match[2]) * 1000n + BigInt(fraction || "0"));
  if (scaled === 0n || scaled < MIN_I64 || scaled > MAX_I64) return null;
  return scaled;
}

function isVersionConflict(error: unknown) {
  return isApiError(error) && (error.status === 409 || error.code === 409);
}

// Posting is the irreversible step of the whole document: the dialog states the
// confirmed version and every line impact before the operator commits it.
function PostAdjustmentDialog({ adjustment, reasonOptions, onCancel, onSave, onSaved, onReloadLatest }: {
  adjustment: InventoryAdjustment;
  reasonOptions: readonly DictSelectOption<AdjustmentReason>[];
  onCancel: () => void;
  onSave: () => Promise<InventoryAdjustment>;
  onSaved: (adjustment: InventoryAdjustment) => void;
  onReloadLatest: () => Promise<void>;
}) {
  const items = adjustment.items ?? [];
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [versionConflict, setVersionConflict] = useState(false);
  const [confirmReload, setConfirmReload] = useState(false);
  const [reloading, setReloading] = useState(false);
  const increases = items.filter((item) => !item.quantity.startsWith("-"));
  const decreases = items.filter((item) => item.quantity.startsWith("-"));
  async function submit() {
    if (!confirmed) { setError("请先确认整单影响。"); return; }
    setError(""); setLoading(true);
    try { onSaved(await onSave()); }
    catch (submitError) {
      if (isVersionConflict(submitError)) {
        setVersionConflict(true); setConfirmReload(false);
        setError("单据状态或版本已变化，本次过账没有生效。请查看最新版本后再处理。");
      } else setError(submitError instanceof Error ? submitError.message : "过账失败，本次过账没有生效，请检查后重试。");
    } finally { setLoading(false); }
  }
  async function reloadLatest() {
    setReloading(true);
    try { await onReloadLatest(); }
    catch (reloadError) { setError(reloadError instanceof Error ? reloadError.message : "重新加载失败；当前确认状态已取消。"); setConfirmReload(false); }
    finally { setReloading(false); }
  }
  return <FormDialog
    open title="过账库存调整单"
    description={`单号 ${adjustment.documentNo} · 确认版本 ${adjustment.version}。过账会在同一事务内写入库存余额和不可变库存流水，成功后单据不可编辑；需要纠错时，可按库存约束整单取消并生成冲销流水。`}
    loading={loading || reloading}
    submitText="确认并过账"
    submitDisabled={!confirmed || versionConflict || items.length === 0}
    onCancel={onCancel} onSubmit={submit}
    contentClassName="w-[min(900px,96vw)]" bodyClassName="overflow-auto"
  >
    <ul className="mb-3 grid gap-1 text-sm">
      <li>增加明细 {increases.length} 条</li>
      <li>减少明细 {decreases.length} 条</li>
    </ul>
    {items.length === 0 ? <p role="alert" className="text-sm text-danger">该单据没有明细，无法过账。请返回编辑草稿后重试。</p> : (
      <div className="overflow-x-auto"><table className="w-full min-w-[760px] border-collapse text-sm">
        <thead><tr className="border-b border-border bg-background text-left"><th className="p-2">商品</th><th className="p-2">型号 / 规格</th><th className="p-2">单位</th><th className="p-2">调整数量</th><th className="p-2">原因</th><th className="p-2">说明</th></tr></thead>
        <tbody>{items.map((item) => <AdjustmentItemRow key={item.id} item={item} reasonOptions={reasonOptions} impact={false} />)}</tbody>
      </table></div>
    )}
    <label className="mt-3 flex items-start gap-2 text-sm">
      <Checkbox className="mt-1" checked={confirmed} onChange={(event) => { setConfirmed(event.target.checked); setError(""); }} />
      <span>我已核对整单明细：正数增加库存、负数减少库存，任一商品减少后小于 0 或超出可表示范围时整单不会过账。</span>
    </label>
    {versionConflict && (confirmReload ? <div role="alert" className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-admin border border-warning/30 bg-warning/5 p-3 text-sm"><span>重新加载会放弃本次确认。是否继续？</span><span className="flex gap-2"><Button type="button" size="sm" variant="secondary" onClick={() => setConfirmReload(false)}>继续查看</Button><Button type="button" size="sm" variant="danger" disabled={reloading} onClick={() => void reloadLatest()}>{reloading ? "正在加载…" : "放弃确认并重新加载"}</Button></span></div> : <Button type="button" size="sm" variant="secondary" className="mt-3" onClick={() => setConfirmReload(true)}>重新加载最新单据</Button>)}
    {error && <p role="alert" className="mt-3 text-sm text-danger">{error}</p>}
  </FormDialog>;
}

function CancelAdjustmentDialog({ adjustment, reasonOptions, onCancel, onSave, onSaved, onReloadLatest }: {
  adjustment: InventoryAdjustment;
  reasonOptions: readonly DictSelectOption<AdjustmentReason>[];
  onCancel: () => void;
  onSave: (reason: string) => Promise<InventoryAdjustment>;
  onSaved: (adjustment: InventoryAdjustment) => void;
  onReloadLatest: () => Promise<void>;
}) {
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [versionConflict, setVersionConflict] = useState(false);
  const [confirmReload, setConfirmReload] = useState(false);
  const [reloading, setReloading] = useState(false);
  // A posted document is cancelled by reversing every line, so the dialog has to
  // state the opposite movement the whole document will apply.
  const posted = adjustment.status === "POSTED";
  const items = adjustment.items ?? [];
  const reversals = items.map((item) => ({ item, reversed: negateQuantity(item.quantity) }));
  async function submit() {
    const value = reason.trim();
    const length = Array.from(value).length;
    if (!value) { setError("请填写取消原因。"); return; }
    if (length > 500) { setError("取消原因最多填写 500 个字。"); return; }
    setError(""); setLoading(true);
    try { onSaved(await onSave(value)); }
    catch (submitError) {
      if (isVersionConflict(submitError)) {
        setVersionConflict(true); setConfirmReload(false);
        setError(posted ? "单据状态或版本已变化，本次取消没有生效。请查看最新版本后再处理。" : "草稿状态或版本已变化，本次取消没有生效。请查看最新版本后再处理。");
      } else if (posted) {
        setError(submitError instanceof Error ? submitError.message : "取消已过账单据失败，单据仍为已过账，库存没有变化。如因冲销会导致负库存，请按实物核对结果新建调整并在说明中注明原单号。");
      } else setError(submitError instanceof Error ? submitError.message : "取消草稿失败；原因已保留，请检查后重试。");
    } finally { setLoading(false); }
  }
  async function reloadLatest() {
    setReloading(true);
    try { await onReloadLatest(); }
    catch (reloadError) { setError(reloadError instanceof Error ? reloadError.message : "重新加载失败；当前原因仍保留。"); setConfirmReload(false); }
    finally { setReloading(false); }
  }
  return <FormDialog
    open
    title={posted ? "取消已过账库存调整单" : "取消库存调整草稿"}
    description={posted
      ? `单号 ${adjustment.documentNo} · 确认版本 ${adjustment.version}。取消会按原明细生成反向库存流水，原流水和过账记录保留；任一商品冲销后小于 0 时整单取消失败。`
      : `单号 ${adjustment.documentNo} · 当前版本 ${adjustment.version}。取消后保留记录，不改变库存。`}
    loading={loading || reloading}
    submitText={posted ? "确认冲销并取消" : "确认取消草稿"}
    submitDisabled={versionConflict || !reason.trim() || Array.from(reason.trim()).length > 500}
    onCancel={onCancel} onSubmit={submit}
    contentClassName={posted ? "w-[min(900px,96vw)]" : undefined}
    bodyClassName={posted ? "overflow-auto" : undefined}
  >
    {posted && (items.length === 0
      ? <p role="alert" className="mb-3 text-sm text-danger">该单据没有明细，无法冲销。请核查数据后处理。</p>
      : <div className="mb-3 overflow-x-auto"><table className="w-full min-w-[760px] border-collapse text-sm">
        <thead><tr className="border-b border-border bg-background text-left"><th className="p-2">商品</th><th className="p-2">单位</th><th className="p-2">过账数量</th><th className="p-2">取消冲销数量</th><th className="p-2">原因</th></tr></thead>
        <tbody>{reversals.map(({ item, reversed }) => <tr key={item.id} className="border-b border-border"><td className="p-2"><div>{item.productName}</div><div className="text-xs text-text-tertiary">{item.productCode}</div></td><td className="p-2">{item.unit}</td><td className="p-2 tabular-nums">{item.quantity}</td><td className="p-2 tabular-nums">{reversed}</td><td className="p-2">{businessDictLabel(reasonOptions, item.reason)}</td></tr>)}</tbody>
      </table></div>)}
    <label className="grid gap-1 text-sm font-medium">取消原因（必填）
      <Textarea maxLength={1000} rows={4} value={reason} onChange={(event) => setReason(event.target.value)} placeholder={posted ? "说明取消这张已过账单据的原因" : "说明取消这张草稿的原因"} />
    </label>
    <p className="mt-1 text-xs text-text-tertiary">最多 500 个字。{posted && "冲销数量为过账数量的相反数，使用过账时的商品资料快照。"}</p>
    {posted && <p className="mt-2 text-xs text-text-tertiary">因负库存无法取消时，不要强制冲销：请按实物核对结果新建调整单，并在说明中注明原单号 {adjustment.documentNo}。</p>}
    {versionConflict && (confirmReload ? <div role="alert" className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-admin border border-warning/30 bg-warning/5 p-3 text-sm"><span>重新加载会放弃当前填写的取消原因。是否继续？</span><span className="flex gap-2"><Button type="button" size="sm" variant="secondary" onClick={() => setConfirmReload(false)}>继续填写</Button><Button type="button" size="sm" variant="danger" disabled={reloading} onClick={() => void reloadLatest()}>{reloading ? "正在加载…" : "放弃原因并重新加载"}</Button></span></div> : <Button type="button" size="sm" variant="secondary" className="mt-3" onClick={() => setConfirmReload(true)}>重新加载最新草稿</Button>)}
    {error && <p role="alert" className="mt-3 text-sm text-danger">{error}</p>}
  </FormDialog>;
}

function AdjustmentDetailDialog({ adjustment, onClose, onEdit, onCancelDraft, onPost, onCancelPosted }: {
  adjustment: InventoryAdjustment; onClose: () => void;
  onEdit: () => void; onCancelDraft: () => void; onPost: () => void; onCancelPosted: () => void;
}) {
  // The read-only body is shared with the ledger's source-document view; only
  // the maintenance actions are specific to this page.
  return <InventoryAdjustmentDetail
    adjustment={adjustment}
    onClose={onClose}
    actions={adjustment.status === "DRAFT" ? <>
      <Button variant="secondary" onClick={onEdit}>编辑草稿</Button>
      <Button variant="danger" onClick={onCancelDraft}>取消草稿</Button>
      <Button onClick={onPost}>过账</Button>
    </> : adjustment.status === "POSTED" ? <Button variant="danger" onClick={onCancelPosted}>取消并冲销</Button> : undefined}
  />;
}
function AdjustmentItemRow({ item, reasonOptions, impact }: { item: InventoryAdjustmentItem; reasonOptions: readonly DictSelectOption<AdjustmentReason>[]; impact: boolean }) {
  const balance = item.balanceBefore != null && item.balanceAfter != null ? `${item.balanceBefore} → ${item.balanceAfter}` : "-";
  return <tr className="border-b border-border"><td className="p-2"><div>{item.productName}</div><div className="text-xs text-text-tertiary">{item.productCode}</div></td><td className="p-2">{item.productModel || "-"} / {item.productSpecification || "-"}</td><td className="p-2">{item.unit}</td><td className="p-2 tabular-nums">{item.quantity}</td><td className="p-2">{businessDictLabel(reasonOptions, item.reason)}</td>{impact && <td className="p-2 tabular-nums">{balance}</td>}<td className="max-w-56 whitespace-pre-wrap p-2">{item.remark || "-"}</td></tr>;
}

// The reversal of a stored decimal quantity is its sign flip, done on the digit
// string so no float rounding can reach the API.
function negateQuantity(quantity: string) {
  const value = quantity.trim();
  if (!/^[+-]?\d+(?:\.\d{1,3})?$/.test(value)) return value;
  return value.startsWith("-") ? value.slice(1) : value.startsWith("+") ? `-${value.slice(1)}` : `-${value}`;
}
