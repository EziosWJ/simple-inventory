import { useExcelExport } from "@/hooks/use-excel-export";
import { exportSaleItems } from "@/api/business";
import { SaleDocumentView, SaleItemView } from "./sale-results";
import { statusText, statusTone } from "./sale-query-status";
import { SaleDetail } from "./sale-detail";
import { saleInventoryImpact } from "./sale-posting-impact";
import { DocumentItems, DocumentSummary } from "@/components/business/document-presentation";
import { DocumentRowActions } from "@/components/business/document-row-actions";
import { FormDialog } from "@/components/common/form-dialog";
import { businessMonthRange } from "@/lib/datetime";
import { postingImpact, cancellationImpact } from "@/lib/document-feedback";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { blankDocumentFilters, useDocumentSearch, type DocumentFilters } from "@/hooks/use-document-search";
import { withBusinessReturn } from "@/lib/business-navigation";
import { cn } from "@/lib/utils";
import { Field } from "@/components/common/field";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { PartnerSelect, ProductSelect } from "@/components/business/master-data-select";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { cancelSale, getSale, postSale, saleItemPage, salePage, type SaleDraft, type SaleItemPage, type SaleItemRecord, type SaleLine } from "@/api/business";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { DataTableColumn } from "@/types";
const pageSizes = [10, 20, 50];
const sortFields = ["businessDate", "unitPrice", "amount", "netAmount"] as const;
type SortField = (typeof sortFields)[number];
export function SalesPage() {
    const { feedback, notify } = useBusinessFeedback();
  const excelExport = useExcelExport(notify);
    const navigate = useNavigate();
    const location = useLocation();
    const { params, setParams, query, filters, setFilters, page, setPage, apply } = useDocumentSearch();
    const view = params.get("view") === "item" ? "item" : "document";
    const rawSize = Number(params.get("size"));
    const pageSize = pageSizes.includes(rawSize) ? rawSize : 10;
    const rawItemPage = Number(params.get("ipage"));
    const itemPage = Number.isSafeInteger(rawItemPage) && rawItemPage > 0 ? rawItemPage : 1;
    const rawSort = params.get("sort") ?? "";
    const sort = (sortFields as readonly string[]).includes(rawSort) ? (rawSort as SortField) : "";
    const dir = params.get("dir") === "asc" ? "asc" : "desc";
    const [records, setRecords] = useState<SaleDraft[]>([]);
    const [total, setTotal] = useState(0), [reload, setReload] = useState(0), [error, setError] = useState(""), [loading, setLoading] = useState(false);
    const [detail, setDetail] = useState<SaleDraft | null>(null);
    const [expanded, setExpanded] = useState<string[]>([]);
    const [itemResult, setItemResult] = useState<SaleItemPage | null>(null);
    const [itemError, setItemError] = useState(""), [itemLoading, setItemLoading] = useState(false);
    const [validationError, setValidationError] = useState("");
    const [detailError, setDetailError] = useState("");
    const [detailLoading, setDetailLoading] = useState(false);
    const [detailReload, setDetailReload] = useState(0);
    const [postTarget, setPostTarget] = useState<SaleDraft | null>(null);
    const [postBusy, setPostBusy] = useState(false);
    const [postError, setPostError] = useState("");
    const postBusyRef = useRef(false);
    const patch = useCallback((changes: Record<string, string | null>) => {
        setParams(current => {
            const updated = new URLSearchParams(current);
            for (const [key, value] of Object.entries(changes)) {
                if (value === null || value === "")
                    updated.delete(key);
                else
                    updated.set(key, value);
            }
            return updated;
        });
    }, [setParams]);
    const patchRef = useRef(patch);
    patchRef.current = patch;
    const load = useCallback(() => {
        let active = true;
        setLoading(true);
        setError("");
        void salePage({ page, pageSize, ...query }).then(result => {
            if (!active)
                return;
            const lastPage = Math.max(1, Math.ceil(result.total / pageSize));
            if (page > lastPage) {
                patchRef.current({ page: lastPage === 1 ? null : String(lastPage) });
                return;
            }
            setRecords(result.records);
            setTotal(result.total);
        }).catch((cause: unknown) => {
            if (!active)
                return;
            setRecords([]);
            setTotal(0);
            setError(cause instanceof Error ? cause.message : "销售单加载失败");
        }).finally(() => { if (active)
            setLoading(false); });
        return () => { active = false; };
    }, [page, pageSize, query]);
    const loadItems = useCallback(() => {
        let active = true;
        setItemLoading(true);
        setItemError("");
        setItemResult(null);
        const args: Record<string, string | number> = { page: itemPage, pageSize, ...query };
        if (sort) {
            args.sortBy = sort;
            args.sortDir = dir;
        }
        void saleItemPage(args).then(result => {
            if (!active)
                return;
            const lastPage = Math.max(1, Math.ceil(result.total / pageSize));
            if (itemPage > lastPage) {
                patchRef.current({ ipage: lastPage === 1 ? null : String(lastPage) });
                return;
            }
            setItemResult(result);
        }).catch((cause: unknown) => {
            if (!active)
                return;
            setItemError(cause instanceof Error ? cause.message : "销售明细加载失败");
        }).finally(() => { if (active)
            setItemLoading(false); });
        return () => { active = false; };
    }, [itemPage, pageSize, sort, dir, query]);
    function applyConditions(next: DocumentFilters) {
        setFilters(next);
        if (next.businessFrom && next.businessTo && next.businessFrom > next.businessTo) {
            setValidationError("业务日期起始不能晚于结束日期。");
            return;
        }
        setValidationError("");
        apply(next, ["ipage"]);
        setReload(value => value + 1);
    }
    function search() { applyConditions(filters); }
    function reset() { applyConditions({ ...blankDocumentFilters }); }
    function switchView(next: "document" | "item") {
        if (next === view)
            return;
        patch({ view: next === "item" ? "item" : null, [next === "item" ? "ipage" : "page"]: null });
    }
    function setItemPage(next: number) { patch({ ipage: String(next) }); }
    function changePageSize(next: number) {
        patch({ size: next === 10 ? null : String(next), [view === "item" ? "ipage" : "page"]: null });
    }
    function toggleSort(field: SortField) {
        const direction = sort === field ? (dir === "asc" ? "desc" : "asc") : field === "businessDate" ? "desc" : "asc";
        patch({ sort: field, dir: direction, ipage: null });
    }
    function toggleExpanded(id: number) {
        const key = String(id);
        setExpanded(current => current.includes(key) ? current.filter(value => value !== key) : [...current, key]);
    }
    useEffect(() => { if (view !== "document")
        return; return load(); }, [view, load, reload]);
    useEffect(() => { if (view !== "item")
        return; return loadItems(); }, [view, loadItems, reload]);
    const directSourceId = params.get("directPurchaseId") ?? "";
    useEffect(() => { if (directSourceId)
        navigate(`/business/sales/new?directPurchaseId=${encodeURIComponent(directSourceId)}`, { replace: true }); }, [directSourceId, navigate]);
    const saleId = Number(params.get("saleId")) || 0;
    useEffect(() => {
        let active = true;
        setDetail(null);
        setDetailError("");
        setDetailLoading(saleId > 0);
        if (saleId > 0) {
            void getSale(saleId).then(result => {
                if (active)
                    setDetail(result);
            }).catch((cause: unknown) => {
                if (active)
                    setDetailError(cause instanceof Error ? cause.message : "读取销售来源单失败");
            }).finally(() => { if (active)
                setDetailLoading(false); });
        }
        return () => { active = false; };
    }, [saleId, detailReload]);
    function closeDetail() {
        setDetail(null);
        const updated = new URLSearchParams(params);
        updated.delete("saleId");
        setParams(updated, { replace: true });
    }
    function openDetail(id: number) { patch({ saleId: String(id) }); }
    function newDraft() { goTo("/business/sales/new"); }
    function editDraft(row: SaleDraft) { goTo(`/business/sales/${row.id}/edit`); }
    async function cancel(document: SaleDraft) {
        const reason = window.prompt("请输入取消原因");
        if (!reason?.trim())
            return;
        try {
            await cancelSale(document.id, { version: document.version, reason });
            notify({ type: "success", title: document.status === "POSTED" ? "销售单冲销取消成功" : "销售草稿取消成功", description: cancellationImpact(document, "sale"), documentNo: document.documentNo, status: "已取消" });
            if (saleId === document.id)
                setDetailReload(value => value + 1);
            setReload(value => value + 1);
        }
        catch (cause) {
            notify({ type: "error", title: "取消失败", description: cause instanceof Error ? cause.message : "取消失败" });
        }
    }
    function post(document: SaleDraft) {
        setPostError("");
        setPostTarget(document);
    }
    async function confirmPost() {
        const document = postTarget;
        if (!document || postBusyRef.current || postError) return;
        postBusyRef.current = true;
        setPostBusy(true);
        setPostError("");
        try {
            const posted = await postSale(document.id, document.version);
            notify({ type: "success", title: "销售单过账成功", description: postingImpact(posted, "sale"), documentNo: posted.documentNo, status: "已过账" });
            setPostTarget(null);
            if (saleId === posted.id)
                setDetailReload(value => value + 1);
            setReload(value => value + 1);
        }
        catch (cause) {
            const message = cause instanceof Error ? cause.message : "过账请求结果尚未确认";
            setPostError(`过账请求未能确认：${message}。请打开单据详情核实当前状态。`);
        } finally {
            postBusyRef.current = false;
            setPostBusy(false);
        }
    }
    const goTo = (path: string) => navigate(withBusinessReturn(path, location.pathname + location.search));
    const matchedLine = (item: SaleLine) => Boolean(item.matchesProduct);
    const summaryParts = (row: SaleDraft) => {
        const parts = row.items.map(item => ({ text: `${item.productName} ×${item.quantity}${item.unit}`, matched: matchedLine(item) }));
        return { parts, count: parts.length, full: parts.map(part => part.text).join("、") };
    };
    const columns: DataTableColumn<SaleDraft>[] = [{ title: "客户", dataIndex: "partnerName", render: (_, r) => <span className="block min-w-[9rem]">{r.partnerName}{r.matchesHistory && <span className="block text-xs text-primary">匹配历史资料</span>}</span> }, { title: "业务日期", dataIndex: "businessDate", nowrap: true }, { title: "销售内容", key: "items", width: "30%", render: (_, r) => { const summary = summaryParts(r); return <div className="min-w-[11rem] space-y-1"><p className="line-clamp-2 text-sm text-text-secondary" title={summary.full}>{summary.parts.map((part, index) => <span key={index} data-matched={part.matched ? "true" : undefined} className={cn(part.matched && "font-medium text-primary")}>{index > 0 ? "、" : ""}{part.text}</span>)}</p><Button size="sm" variant="ghost" aria-expanded={expanded.includes(String(r.id))} onClick={() => toggleExpanded(r.id)}>{expanded.includes(String(r.id)) ? "收起明细" : "展开明细"}</Button></div>; } }, { title: "状态", dataIndex: "status", nowrap: true, render: v => v === "DRAFT" ? "草稿" : v === "POSTED" ? "已过账" : "已取消" }, { title: "金额", dataIndex: "totalAmount", nowrap: true, render: v => `¥${v}` }, { title: "操作", key: "actions", nowrap: true, render: (_, r) => <DocumentRowActions documentNo={r.documentNo}><Button size="sm" variant="secondary" onClick={() => openDetail(r.id)}>详情</Button>{r.status === "DRAFT" && !r.sourceOrderId && <><Button size="sm" variant="secondary" onClick={() => editDraft(r)}>编辑</Button><Button size="sm" onClick={() => void post(r)}>过账</Button><Button size="sm" variant="secondary" onClick={() => void cancel(r)}>取消</Button></>}{r.status === "POSTED" && !r.sourceOrderId && <Button size="sm" variant="secondary" onClick={() => void cancel(r)}>取消已过账单</Button>}{r.sourceOrderId && <Button size="sm" onClick={() => goTo(`/business/direct-orders/${r.sourceOrderId}`)}>来源直送单</Button>}</DocumentRowActions> }];
    const sortHeader = (label: string, field: SortField) => <button type="button" aria-label={`按${label}排序`} aria-pressed={sort === field} className={cn("inline-flex items-center gap-1 font-medium", sort === field ? "text-primary" : "text-text-secondary hover:text-text-primary")} onClick={() => toggleSort(field)}>{label}<span aria-hidden="true">{sort === field ? (dir === "asc" ? "▲" : "▼") : "↕"}</span></button>;
    const itemColumns: DataTableColumn<SaleItemRecord>[] = [
        { title: sortHeader("业务日期", "businessDate"), key: "businessDate", nowrap: true, render: (_, r) => r.businessDate },
        { title: "客户", key: "partner", width: "200px", render: (_, r) => <span className="block min-w-[12rem]"><span>{r.partnerName}</span>{r.matchesHistory && <span className="block text-xs text-primary">匹配历史资料</span>}</span> },
        { title: "商品/服务", key: "product", width: "22%", render: (_, r) => { const description = [r.productModel, r.productSpecification].filter(value => Boolean(value?.trim())).join(" · "); return <div className="min-w-[11rem] space-y-1"><div className="font-medium text-text-primary">{[r.productCode, r.productName].filter(Boolean).join(" · ")}</div>{description && <div className="text-sm text-text-secondary">{description}</div>}<div className="text-xs text-text-tertiary">{r.productType === "SERVICE" ? "服务" : "实物"}</div></div>; } },
        { title: "数量", key: "quantity", align: "right", nowrap: true, render: (_, r) => <span className="tabular-nums">{r.quantity} {r.unit}</span> },
        { title: sortHeader("成交单价", "unitPrice"), key: "unitPrice", align: "right", nowrap: true, render: (_, r) => `¥${r.unitPrice}` },
        { title: sortHeader("原金额", "amount"), key: "amount", align: "right", nowrap: true, render: (_, r) => `¥${r.amount}` },
        { title: "有效退货", key: "returned", align: "right", nowrap: true, render: (_, r) => r.returnsApplicable ? <span className="tabular-nums">¥{r.returnedAmount}<span className="block text-xs text-text-tertiary">{r.returnedQuantity === "0" ? "无退货" : `已退 ${r.returnedQuantity} ${r.unit}`}</span></span> : <span className="text-text-tertiary" title="服务不能退货">不适用</span> },
        { title: sortHeader("净销售额", "netAmount"), key: "netAmount", align: "right", nowrap: true, render: (_, r) => r.netApplicable && r.netQuantity != null && r.netAmount != null ? <span className="tabular-nums font-medium">¥{r.netAmount}<span className="block text-xs text-text-tertiary">净 {r.netQuantity} {r.unit}</span></span> : <span className="text-text-tertiary" title="草稿与已取消销售不是有效销售">不适用</span> },
        { title: "来源单号", key: "documentNo", render: (_, r) => <a href={`?${new URLSearchParams({ ...Object.fromEntries(params), saleId: String(r.saleId) })}`} title={r.documentNo} className="block max-w-[12rem] truncate text-primary hover:underline" onClick={event => { if (event.button === 0 && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey) {
                event.preventDefault();
                openDetail(r.saleId);
            } }}>{r.documentNo}</a> },
        { title: "状态", key: "status", nowrap: true, width: "90px", render: (_, r) => <span className="whitespace-nowrap"><StatusTag tone={statusTone(r.status)}>{statusText(r.status)}</StatusTag></span> },
    ];
    const exportAction = <Button size="sm" variant="secondary" disabled={excelExport.exporting} onClick={() => { const args = { ...filters, partnerId: filters.partnerKeyword.trim() ? "" : filters.partnerId, productId: filters.productKeyword.trim() ? "" : filters.productId, ...(sort ? { sortBy: sort, sortDir: dir } : {}) }; void excelExport.start({ count: () => saleItemPage({ ...args, page: 1, pageSize: 1 }), download: () => exportSaleItems(args) }); }}>{excelExport.exporting ? "导出中" : "导出明细 Excel"}</Button>;
    const activeError = validationError || (view === "item" ? itemError : error);
    const postItems = postTarget?.items.map(item => ({ productCode: item.productCode, productName: item.productName, productModel: item.productModel, productSpecification: item.productSpecification, productType: item.productType, quantity: item.quantity, unit: item.unit, unitPrice: item.unitPrice, amount: item.amount, remark: item.remark })) ?? [];
    const postInventory = postTarget ? saleInventoryImpact(postTarget.items, postTarget.directDelivery) : "";
    return <div className="space-y-4"><PageHeader title="销售出库" description="维护客户交付草稿；草稿不改变库存或应收。" actions={<Button onClick={newDraft}>新建销售草稿</Button>}/>
  {activeError && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded border border-red-300 p-3 text-red-700"><span>{activeError}</span><Button size="sm" variant="secondary" onClick={() => setReload(v => v + 1)}>重试查询</Button></div>}
  <BusinessFeedback feedback={feedback}/>
 {excelExport.confirmation}
    <SearchFilterBar stacked onSubmit={search} leadingActions={<>
      <Button variant="secondary" onClick={() => { const month = businessMonthRange(); applyConditions({ ...filters, businessFrom: month.from, businessTo: month.to }); }}>本月</Button>
      <Button variant="secondary" aria-pressed={filters.status === "POSTED"} onClick={() => applyConditions({ ...filters, status: "POSTED" })}>已过账</Button>
    </>} actions={<><Button onClick={search}>查询</Button><Button onClick={reset}>重置</Button></>}>
    <Field label="销售单号"><Input aria-label="销售单号" placeholder="销售单号" value={filters.documentNo} onChange={e => setFilters({ ...filters, documentNo: e.target.value })}/></Field>
    <Field label="客户"><PartnerSelect inlineBrowse placeholder="搜索名称、编码、联系人或电话" label="客户筛选" historical value={Number(filters.partnerId)} keyword={filters.partnerKeyword} onDraftKeywordChange={k => setFilters(current => ({ ...current, partnerKeyword: k }))} onKeywordChange={k => { const next = { ...filters, partnerId: "", partnerKeyword: k }; applyConditions(next); }} onChange={p => { setFilters(current => ({ ...current, partnerId: p ? String(p.id) : "", partnerKeyword: "" })); }}/></Field>
    <Field label="商品/服务"><ProductSelect inlineBrowse placeholder="搜索名称、编码、品牌或型号" label="商品或服务筛选" historical value={Number(filters.productId)} keyword={filters.productKeyword} onDraftKeywordChange={k => setFilters(current => ({ ...current, productKeyword: k }))} onKeywordChange={k => { const next = { ...filters, productId: "", productKeyword: k }; applyConditions(next); }} onChange={p => { setFilters(current => ({ ...current, productId: p ? String(p.id) : "", productKeyword: "" })); }}/></Field>
    <Field label="交付方式"><Select aria-label="交付方式" value={filters.directDelivery} onChange={e => setFilters({ ...filters, directDelivery: e.target.value })}><option value="">全部</option><option value="false">普通</option><option value="true">直送</option></Select></Field>
    <Field label="状态"><Select aria-label="销售状态" value={filters.status} onChange={e => setFilters({ ...filters, status: e.target.value })}><option value="">全部状态</option><option value="DRAFT">草稿</option><option value="POSTED">已过账</option><option value="CANCELLED">已取消</option></Select></Field>
    <Field label="业务日期起"><Input aria-label="业务日期起" type="date" value={filters.businessFrom} onChange={e => setFilters({ ...filters, businessFrom: e.target.value })}/></Field>
    <Field label="业务日期止"><Input aria-label="业务日期止" type="date" value={filters.businessTo} onChange={e => setFilters({ ...filters, businessTo: e.target.value })}/></Field>
  </SearchFilterBar>
  <p className="text-xs text-text-secondary">默认查询全部日期与状态。关键词查当前档案及已保存的商品快照；旧品牌和供应商联系方式仅查当前档案。销售还可匹配已保存的客户名称。按明细的合计覆盖全部筛选结果，并扣除截至当前仍已过账的销售退货。</p>
  {view === "document" ?
            <SaleDocumentView exportAction={exportAction} page={page} pageSize={pageSize} total={total} records={records} loading={loading} error={error} columns={columns} expanded={expanded} setPage={setPage} switchView={switchView} onRefresh={() => setReload(value => value + 1)} changePageSize={changePageSize} renderExpandedRow={row => <ul className="space-y-1" aria-label={`${row.documentNo} 全部明细`}>{row.items.map(item => <li key={item.id} className="flex flex-wrap items-baseline gap-x-space-4 gap-y-1 text-sm"><span data-matched={matchedLine(item) ? "true" : undefined} className={cn("min-w-[12rem] flex-1", matchedLine(item) && "font-medium text-primary")}>{[item.productCode, item.productName].filter(Boolean).join(" · ")}<span className="ml-2 text-xs text-text-tertiary">{[item.productModel, item.productSpecification].filter(Boolean).join(" · ")}</span><span className="ml-2 text-xs text-text-tertiary">{item.productType === "SERVICE" ? "服务" : "实物"}</span></span><span className="tabular-nums">¥{item.unitPrice}</span><span className="tabular-nums font-medium">¥{item.amount}</span><span className="whitespace-nowrap tabular-nums">{item.quantity} {item.unit}</span></li>)}</ul>}/> :
            <SaleItemView exportAction={exportAction} itemPage={itemPage} pageSize={pageSize} itemResult={itemResult} itemLoading={itemLoading} itemError={itemError} columns={itemColumns} setItemPage={setItemPage} switchView={switchView} onRefresh={() => setReload(value => value + 1)} changePageSize={changePageSize}/>}
  <SaleDetail detail={detail} saleId={saleId} loading={detailLoading} error={detailError} onRetry={() => setDetailReload(value => value + 1)} onClose={closeDetail} goTo={goTo} onEdit={editDraft} onChange={setDetail}/>
 <FormDialog documentLayout open={postTarget !== null} title="确认销售单过账" description={postTarget ? `${postTarget.documentNo} · ${postTarget.partnerName} · 业务日期 ${postTarget.businessDate}` : undefined} cancelText="返回核对" submitText="确认过账" submitDisabled={Boolean(postError)} loadingText="正在过账…" loading={postBusy} onCancel={() => { setPostTarget(null); setPostError(""); }} onSubmit={confirmPost}>
   {postTarget && <div className="space-y-space-5">
     {postError && <div role="alert" className="space-y-space-2 rounded-control border border-error-border bg-error-background px-space-4 py-space-3 text-sm text-error"><p>{postError}</p><Button onClick={() => { const id = postTarget.id; setPostTarget(null); setPostError(""); openDetail(id); }}>打开单据详情核实</Button></div>}
     <DocumentSummary totals={[{ label: "销售金额", value: `¥${postTarget.totalAmount}` }]} hint={`共 ${postTarget.items.length} 行 · 实物 ${postTarget.items.filter(item => item.productType === "GOODS").length} 行 · 服务 ${postTarget.items.filter(item => item.productType === "SERVICE").length} 行`}>
       <p>{postTarget.items.some(item => item.productType === "GOODS") ? "确认后实物商品将减少库存；" : "仅含服务，不产生库存变动；"}{/^0(?:\.0+)?$/.test(postTarget.totalAmount) ? "零金额，客户往来余额无变化。" : `客户方向往来余额增加 ¥${postTarget.totalAmount}。`}</p>
       {postTarget.directDelivery && <p>旧版直送销售：扣减此前采购增加的账面库存，沿用采购与销售分别过账规则。</p>}
     </DocumentSummary>
     <DocumentItems title="即将过账的销售明细" items={postItems}/>
     <section aria-label="库存影响" className="space-y-space-2"><h3 className="text-base font-semibold">确认后库存影响</h3><ul className="list-inside list-disc whitespace-pre-wrap text-sm">{postInventory.split("\n").map((line, index) => <li key={index}>{line}</li>)}</ul></section>
   </div>}
 </FormDialog>

 </div>;
}
