import { formatDateTime } from "@/lib/datetime";
import { postingImpact, cancellationImpact } from "@/lib/document-feedback";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { useDocumentSearch } from "@/hooks/use-document-search";
import { withBusinessReturn } from "@/lib/business-navigation";
import { PartnerSelect, ProductSelect } from "@/components/business/master-data-select";
import { DirectDeliveryTrace } from "@/components/business/direct-delivery-trace";
import { purchasePostingConfirmation } from "@/pages/business/purchase-posting-impact";
import { useCallback, useEffect, useState } from "react";
import {
  cancelPurchase,
  getPurchase,
  postPurchase,
  purchasePage,
  purchaseItemPage,
  type PurchaseDraft,
  type PurchaseItemPage,
  type PurchaseItemRecord,
} from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { useNavigate, useLocation } from "react-router-dom";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { DataTableColumn } from "@/types";

const PAGE_SIZE = 10;

export function PurchasesPage() {
  const { feedback, notify } = useBusinessFeedback();
  const navigate=useNavigate();
  const location=useLocation();
  const {params:searchParams,setParams: setSearchParams,query,filters,setFilters,page,setPage,apply}=useDocumentSearch();
  const goTo=(path:string)=>navigate(withBusinessReturn(path,location.pathname+location.search));
  const [records, setRecords] = useState<PurchaseDraft[]>([]);
  const [itemRecords, setItemRecords] = useState<PurchaseItemRecord[]>([]);
  const [itemSummary, setItemSummary] = useState<PurchaseItemPage["summary"]>({ amounts: [], quantities: [] });
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [saving, setSaving] = useState(false);
  const [detail, setDetail] = useState<PurchaseDraft | null>(null);
  const [cancelTarget, setCancelTarget] = useState<PurchaseDraft | null>(null);
  const [cancelReason, setCancelReason] = useState("");
  const [postTarget, setPostTarget] = useState<PurchaseDraft | null>(null);
  const itemView = searchParams.get("view") === "items";

  const load = useCallback(() => {
    let active=true;
    setLoading(true);
    const request = itemView ? purchaseItemPage({ page, pageSize: PAGE_SIZE, ...query }) : purchasePage({
      page,
      pageSize: PAGE_SIZE,
      directDelivery: query.directDelivery,
      documentNo: query.documentNo,
      partnerId: query.partnerId,
      productId: query.productId,
      partnerKeyword: query.partnerKeyword, productKeyword: query.productKeyword,
      status: query.status,
      businessFrom: query.businessFrom,
      businessTo: query.businessTo,
    });
    void request
      .then((result) => {
        if(!active)return;
        if ("summary" in result) { setRecords([]); setItemRecords(result.records); setItemSummary(result.summary); }
        else { setRecords(result.records); setItemRecords([]); }
        setTotal(result.total);
        setError("");
      })
      .catch((reason: unknown) => {
        if(!active)return;
        setRecords([]);
        setTotal(0);
        setError(reason instanceof Error ? reason.message : "采购单加载失败");
      })
      .finally(() => {if(active)setLoading(false);});
    return ()=>{active=false;};
  }, [page, query, itemView]);

  useEffect(() => load(), [load, reload]);
  useEffect(() => { let active=true; const id=Number(searchParams.get("purchaseId")); if(id>0) void getPurchase(id).then(d=>{if(active)setDetail(d)}).catch((reason:unknown)=>{if(active)setError(reason instanceof Error?reason.message:"读取采购单失败")});else setDetail(null); return()=>{active=false;}; }, [searchParams]);
  function closeDetail(){setDetail(null);const p=new URLSearchParams(searchParams);p.delete("purchaseId");setSearchParams(p,{replace:true});}

  function startNew() { goTo("/business/purchases/new"); }
  function startEdit(record: PurchaseDraft) { goTo(`/business/purchases/${record.id}/edit`); }

  function openDetail(record:PurchaseDraft){const p=new URLSearchParams(searchParams);p.set("purchaseId",String(record.id));setSearchParams(p);}

  async function confirmCancel() {
    if (!cancelTarget) return;
    setSaving(true);
    try {
      await cancelPurchase(cancelTarget.id, {
        version: cancelTarget.version,
        reason: cancelReason,
      });
      notify({ type: "success", title: cancelTarget.status === "POSTED" ? "采购单冲销取消成功" : "采购草稿取消成功", description: cancellationImpact(cancelTarget, "purchase"), documentNo: cancelTarget.documentNo, status: "已取消" });
      setError(""); setCancelTarget(null);
      setReload((value) => value + 1);
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : "取消失败，采购草稿仍保留"; setError(message); notify({ type: "error", title: "取消失败", description: message });
    } finally {
      setSaving(false);
    }
  }
  async function confirmPost(){if(!postTarget)return;setSaving(true);try{const posted=await postPurchase(postTarget.id,postTarget.version);notify({type:"success",title:"采购单过账成功",description:postingImpact(posted,"purchase"),documentNo:posted.documentNo,status:"已过账"});setError("");setPostTarget(null);setDetail(null);setReload(v=>v+1);}catch(reason){const message=reason instanceof Error?reason.message:"过账失败，单据仍保留";setError(message);notify({type:"error",title:"过账失败",description:message});}finally{setSaving(false);}}

  function applyFilters(next = filters) {
    if (next.businessFrom && next.businessTo && next.businessFrom > next.businessTo) {
      setError("业务日期起始不能晚于结束日期。");
      return;
    }
    apply(next);
    setReload(v=>v+1);
  }

  function resetFilters() {
    apply({documentNo:"",partnerId:"",productId:"",partnerKeyword:"",productKeyword:"",status:"",directDelivery:"",businessFrom:"",businessTo:""});
    setReload(v=>v+1);
  }

  const columns: DataTableColumn<PurchaseDraft>[] = [
    {
      title: "采购单号",
      dataIndex: "documentNo",
      nowrap: true,
      render: (value, record) => (
        <button className="text-primary hover:underline" onClick={() => void openDetail(record)}>
          {String(value ?? "")}{record.matchesHistory&&<span className="block text-xs text-primary">匹配历史资料</span>}
        </button>
      ),
    },
    { title: "供应商", dataIndex: "partnerName" },
    { title: "采购内容", key: "items", render: (_, record) => { const full = record.items.map(item => `${item.productCode} · ${item.productName} ${item.productModel ?? ""} ${item.productSpecification ?? ""} ×${item.quantity} ${item.unit}`).join("、"); return <span className="block max-w-[22rem] truncate text-sm" title={full}>{full || "—"}</span>; } },
    { title: "交付方式", dataIndex: "directDelivery", render: value => value ? "直送" : "普通" },
    { title: "业务日期", dataIndex: "businessDate", nowrap: true },
    { title: "金额", dataIndex: "totalAmount", align: "right", render: (value) => `¥${String(value ?? "0.00")}` },
    { title: "状态", dataIndex: "status", render: (value) => ({DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[String(value)] ?? String(value)) },
    { title: "创建人", dataIndex: "createdByName" },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      render: (_, record) => (
        <div className="flex gap-2">
          <Button size="sm" variant="secondary" onClick={() => void openDetail(record)}>查看</Button>
          {record.status === "DRAFT" && (
            <>
              <Button size="sm" variant="secondary" onClick={() => void startEdit(record)}>编辑</Button>
              <Button size="sm" onClick={() => setPostTarget(record)}>过账</Button>
              <Button size="sm" variant="secondary" onClick={() => { setCancelTarget(record); setCancelReason(""); }}>取消</Button>
            </>
          )}
          {record.status === "POSTED" && <><Button size="sm" variant="secondary" onClick={() => { setCancelTarget(record); setCancelReason(""); }}>整单取消</Button><Button size="sm" onClick={() => goTo(`/business/purchase-returns?purchaseId=${record.id}`)}>办理退货</Button></>}
        </div>
      ),
    },
  ];
  const itemColumns: DataTableColumn<PurchaseItemRecord>[] = [
    { title: "采购单号", dataIndex: "documentNo", render: (value, row) => <button className="text-primary hover:underline" onClick={() => openDetail({ id: row.purchaseId } as PurchaseDraft)}>{String(value)}{row.matchesHistory && <span className="block text-xs">匹配历史资料</span>}</button> },
    { title: "供应商", dataIndex: "partnerName" },
    { title: "商品", key: "product", render: (_, row) => <div>{row.productCode} · {row.productName}<div className="text-xs text-text-tertiary">{row.productModel || "-"} / {row.productSpecification || "-"} · {row.unit}</div></div> },
    { title: "业务日期", dataIndex: "businessDate" },
    { title: "数量", dataIndex: "quantity", align: "right" },
    { title: "成交单价", dataIndex: "unitPrice", align: "right", render: value => `¥${value}` },
    { title: "原金额", dataIndex: "amount", align: "right", render: value => `¥${value}` },
    { title: "有效退货", dataIndex: "returnedAmount", align: "right", render: value => value == null ? "—" : `¥${value}` },
    { title: "净采购", dataIndex: "netAmount", align: "right", render: value => value == null ? "—" : `¥${value}` },
    { title: "状态", dataIndex: "status", render: value => ({DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"} as Record<string,string>)[String(value)] ?? String(value) },
  ];

  function toggleView(next: "documents" | "items") {
    const params = new URLSearchParams(searchParams);
    if (next === "items") params.set("view", "items"); else params.delete("view");
    params.delete("page");
    setSearchParams(params);
  }

  return (
    <div className="space-y-space-4">
      <PageHeader
        title="采购入库"
        description="记录实际收货；草稿保存和取消不改变库存或应付款。"
        actions={<Button onClick={startNew}>新建采购单</Button>}
      />
      <BusinessFeedback feedback={feedback} />
<SearchFilterBar onSubmit={applyFilters} actions={(
        <>
          <Button onClick={() => applyFilters()}>查询</Button>
          <Button variant="secondary" onClick={()=>setReload(v=>v+1)}>刷新</Button>
          <Button variant="secondary" onClick={resetFilters}>重置</Button>
        </>
      )}>
        <Field label="采购单号">
          <Input value={filters.documentNo} onChange={(event) => setFilters({ ...filters, documentNo: event.target.value })} placeholder="输入单号" />
        </Field>
        <Field label="供应商">
          <PartnerSelect inlineBrowse label="筛选供应商" historical value={Number(filters.partnerId)} keyword={filters.partnerKeyword} onDraftKeywordChange={k=>setFilters(current=>({...current,partnerKeyword:k}))} onKeywordChange={k=>{const next={...filters,partnerId:"",partnerKeyword:k};setFilters(next);applyFilters(next)}} onChange={p=>setFilters(current=>({...current,partnerId:p?String(p.id):"",partnerKeyword:""}))} />
        </Field>
        <Field label="交付方式"><Select value={filters.directDelivery} onChange={event=>setFilters({...filters,directDelivery:event.target.value})}><option value="">全部</option><option value="false">普通</option><option value="true">直送</option></Select></Field>
        <Field label="状态">
          <Select value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}>
            <option value="">全部状态</option><option value="DRAFT">草稿</option><option value="POSTED">已过账</option><option value="CANCELLED">已取消</option>
          </Select>
        </Field>
        <Field label="商品">
          <ProductSelect inlineBrowse label="筛选商品" historical value={Number(filters.productId)} keyword={filters.productKeyword} onDraftKeywordChange={k=>setFilters(current=>({...current,productKeyword:k}))} onKeywordChange={k=>{const next={...filters,productId:"",productKeyword:k};setFilters(next);applyFilters(next)}} onChange={p=>setFilters(current=>({...current,productId:p?String(p.id):"",productKeyword:""}))} />
        </Field>
        <Field label="业务日期起">
          <Input type="date" value={filters.businessFrom} onChange={(event) => setFilters({ ...filters, businessFrom: event.target.value })} />
        </Field>
        <Field label="业务日期止">
          <Input type="date" value={filters.businessTo} onChange={(event) => setFilters({ ...filters, businessTo: event.target.value })} />
        </Field>
      </SearchFilterBar>
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
      <p className="text-xs text-text-secondary">关键词查当前档案及已保存的商品快照；旧品牌和供应商联系方式仅查当前档案。</p>
      {itemView && <div className="grid gap-2 rounded-admin border border-border bg-surface p-3 sm:grid-cols-3">{itemSummary.amounts.map(summary => <div key={summary.status} className="text-sm"><div className="text-text-tertiary">{{DRAFT:"草稿原金额",POSTED:"已过账原金额",CANCELLED:"已取消原金额"}[summary.status]}</div><strong>¥{summary.originalAmount}</strong>{summary.netAmount != null && <span className="ml-2 text-text-secondary">退货 ¥{summary.returnedAmount} · 净采购 ¥{summary.netAmount}</span>}</div>)}</div>}
      <DataTableCard
        toolbar={<TableToolbar title={itemView ? "采购商品明细" : "采购入库单"} description={itemView ? "跨单查看命中明细；金额与退货合计覆盖所有筛选结果。" : "支持按单号、供应商、状态、商品与业务日期查询。"} actions={<div className="flex gap-2"><Button size="sm" variant={itemView ? "secondary" : "primary"} onClick={() => toggleView("documents")}>按单据</Button><Button size="sm" variant={itemView ? "primary" : "secondary"} onClick={() => toggleView("items")}>按明细</Button></div>} />}
        pagination={<Pagination page={page} pageSize={PAGE_SIZE} total={total} disabled={loading} onPageChange={setPage} />}
      >
        {itemView ? <DataTable columns={itemColumns} dataSource={itemRecords} rowKey="id" loading={loading} error={error || undefined} minWidth={1200} empty="暂无采购明细。" /> : <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} error={error || undefined} minWidth={960} empty="暂无采购单。" />}
      </DataTableCard>

      <DetailDialog footer={<BusinessReturnLink />}
        open={detail !== null}
        title={`采购入库单${detail ? ` · ${detail.documentNo}` : "详情"}`}
        description={detail ? `${detail.partnerName} · ${detail.businessDate} · ${{DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[detail.status]}` : undefined}
        onCancel={closeDetail}
      >
        {detail && (
          <>
            {detail.status === "DRAFT" && <Button onClick={()=>startEdit(detail)}>编辑草稿</Button>}
            <p className="mb-3">{detail.directDelivery ? "直送：先单独过账采购，再单独过账销售；取消时先取消关联销售。" : "普通备货采购"}</p>
            {detail.directDelivery && <div className="mb-3 space-y-2">{detail.directDocuments.map(d=><p key={d.id}><Button variant="secondary" onClick={()=>goTo(`/business/sales?saleId=${d.id}`)}>{d.documentNo} · {statusText(d.status)}</Button> {d.postedAt&&`过账：${d.postedByName} ${formatDateTime(d.postedAt)}`} {d.cancelReason&&`取消：${d.cancelledByName} ${d.cancelReason}`}</p>)}{detail.status!=="CANCELLED" && !detail.directDocuments.some(d=>d.status!=="CANCELLED") && <Button onClick={()=>goTo(`/business/sales/new?directPurchaseId=${detail.id}`)}>关联新建直送销售</Button>}</div>}
            <DataTable
              columns={[
                { title: "商品", key: "product", render: (_, item) => <div className="min-w-[12rem] space-y-1"><div>{[item.productCode,item.productName,item.productModel,item.productSpecification].filter(Boolean).join(" · ")}</div>{item.remark?.trim() && <div className="whitespace-pre-wrap break-words text-xs text-text-secondary">备注：{item.remark.trim()}</div>}</div> },
                { title: "数量", dataIndex: "quantity" },
                { title: "单位", dataIndex: "unit" },
                { title: "成交单价", dataIndex: "unitPrice", align: "right" },
                { title: "金额", dataIndex: "amount", align: "right" },
              ]}
              dataSource={detail.items}
              rowKey="id"
              minWidth={720}
            />
            <p className="mt-space-4 text-right font-medium">合计：¥{detail.totalAmount}</p>
            <p className="mt-space-2 text-sm text-text-tertiary">
              创建人：{detail.createdByName} · 创建时间：{formatDateTime(detail.createTime)}
              {detail.postedByName && ` · 过账人：${detail.postedByName}${detail.postedAt ? ` · 实际过账时间：${formatDateTime(detail.postedAt)}` : ""}`}
              {detail.cancelledByName && ` · 取消人：${detail.cancelledByName}`}
              {detail.cancelledAt && ` · 取消时间：${formatDateTime(detail.cancelledAt)}`}
            </p>
            <DirectDeliveryTrace trace={detail.directTrace}/>
            {detail.cancelReason && <p className="mt-space-2 text-sm">取消原因：{detail.cancelReason}</p>}
            {detail.remark?.trim() && <p className="mt-space-2 whitespace-pre-wrap break-words text-sm">单据备注：{detail.remark.trim()}</p>}
            <Button className="mt-space-3" variant="secondary" onClick={() => goTo(`/business/partner-ledger?partnerId=${detail.partnerId}&direction=SUPPLIER`)}>查看供应商往来</Button>
          </>
        )}
      </DetailDialog>

      <FormDialog
        open={cancelTarget !== null}
        title={cancelTarget?.status === "POSTED" ? "整单取消采购入库" : "取消采购草稿"}
        description={cancelTarget ? `单号 ${cancelTarget.documentNo}。已付款保留；若库存不足，整单取消将被拒绝。` : undefined}
        submitText="确认取消"
        loading={saving}
        submitDisabled={!cancelReason.trim()}
        onCancel={() => setCancelTarget(null)}
        onSubmit={confirmCancel}
      >
        <Field label="取消原因" required>
          <Textarea value={cancelReason} onChange={(event) => setCancelReason(event.target.value)} maxLength={500} placeholder="请说明取消原因" />
        </Field>
      </FormDialog>
      <FormDialog open={postTarget !== null} title="过账采购入库单" description={postTarget ? `${postTarget.documentNo} · ${postTarget.partnerName} · 合计 ¥${postTarget.totalAmount} · 当前版本 ${postTarget.version}` : undefined} submitText="确认过账" loading={saving} onCancel={() => setPostTarget(null)} onSubmit={confirmPost}>
        {postTarget && <div className="max-h-56 space-y-2 overflow-auto text-sm"><p className="font-medium">本次过账影响</p>{purchasePostingConfirmation(postTarget).map((impact) => <p key={impact}>{impact}</p>)}</div>}
      </FormDialog>
    </div>
  );
}

function statusText(s:string){return {DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[s]??s}
