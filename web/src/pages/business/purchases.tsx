import { postingImpact, cancellationImpact } from "@/lib/document-feedback";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { useDocumentSearch } from "@/hooks/use-document-search";
import { withBusinessReturn } from "@/lib/business-navigation";
import { PartnerSelect, ProductSelect } from "@/components/business/master-data-select";
import { DirectDeliveryTrace } from "@/components/business/direct-delivery-trace";
import { useCallback, useEffect, useState } from "react";
import {
  cancelPurchase,
  getPurchase,
  postPurchase,
  purchasePage,
  type PurchaseDraft,
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
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [saving, setSaving] = useState(false);
  const [detail, setDetail] = useState<PurchaseDraft | null>(null);
  const [cancelTarget, setCancelTarget] = useState<PurchaseDraft | null>(null);
  const [cancelReason, setCancelReason] = useState("");
  const [postTarget, setPostTarget] = useState<PurchaseDraft | null>(null);

  const load = useCallback(() => {
    let active=true;
    setLoading(true);
    void purchasePage({
      page,
      pageSize: PAGE_SIZE,
      documentNo: query.documentNo,
      partnerId: query.partnerId,
      productId: query.productId,
      status: query.status,
      businessFrom: query.businessFrom,
      businessTo: query.businessTo,
    })
      .then((result) => {
        if(!active)return;
        setRecords(result.records);
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
  }, [page, query]);

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

  function applyFilters() {
    if (filters.businessFrom && filters.businessTo && filters.businessFrom > filters.businessTo) {
      setError("业务日期起始不能晚于结束日期。");
      return;
    }
    apply(filters);
    setReload(v=>v+1);
  }

  function resetFilters() {
    apply({documentNo:"",partnerId:"",productId:"",status:"",businessFrom:"",businessTo:""});
    setReload(v=>v+1);
  }

  const columns: DataTableColumn<PurchaseDraft>[] = [
    {
      title: "采购单号",
      dataIndex: "documentNo",
      nowrap: true,
      render: (value, record) => (
        <button className="text-primary hover:underline" onClick={() => void openDetail(record)}>
          {String(value ?? "")}
        </button>
      ),
    },
    { title: "供应商", dataIndex: "partnerName" },
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

  return (
    <div className="space-y-space-4">
      <PageHeader
        title="采购入库"
        description="记录实际收货；草稿保存和取消不改变库存或应付款。"
        actions={<Button onClick={startNew}>新建采购单</Button>}
      />
      <BusinessFeedback feedback={feedback} />
<SearchFilterBar actions={(
        <>
          <Button onClick={applyFilters}>查询</Button>
          <Button variant="secondary" onClick={()=>setReload(v=>v+1)}>刷新</Button>
          <Button variant="secondary" onClick={resetFilters}>重置</Button>
        </>
      )}>
        <Field label="采购单号">
          <Input value={filters.documentNo} onChange={(event) => setFilters({ ...filters, documentNo: event.target.value })} placeholder="输入单号" />
        </Field>
        <Field label="供应商">
          <PartnerSelect label="筛选供应商" historical value={Number(filters.partnerId)} onChange={partner => setFilters({ ...filters, partnerId: partner ? String(partner.id) : "" })} />
        </Field>
        <Field label="状态">
          <select className="h-control w-full rounded-control border border-border bg-surface px-space-3" value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}>
            <option value="">全部状态</option><option value="DRAFT">草稿</option><option value="POSTED">已过账</option><option value="CANCELLED">已取消</option>
          </select>
        </Field>
        <Field label="商品">
          <ProductSelect label="筛选商品" historical value={Number(filters.productId)} onChange={product => setFilters({ ...filters, productId: product ? String(product.id) : "" })} />
        </Field>
        <Field label="业务日期起">
          <Input type="date" value={filters.businessFrom} onChange={(event) => setFilters({ ...filters, businessFrom: event.target.value })} />
        </Field>
        <Field label="业务日期止">
          <Input type="date" value={filters.businessTo} onChange={(event) => setFilters({ ...filters, businessTo: event.target.value })} />
        </Field>
      </SearchFilterBar>
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
      <DataTableCard
        toolbar={<TableToolbar title="采购入库单" description="支持按单号、供应商、状态、商品与业务日期查询。" />}
        pagination={<Pagination page={page} pageSize={PAGE_SIZE} total={total} disabled={loading} onPageChange={setPage} />}
      >
        <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} error={error || undefined} minWidth={960} empty="暂无采购单。" />
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
            {detail.directDelivery && <div className="mb-3 space-y-2">{detail.directDocuments.map(d=><p key={d.id}><Button variant="secondary" onClick={()=>goTo(`/business/sales?saleId=${d.id}`)}>{d.documentNo} · {statusText(d.status)}</Button> {d.postedAt&&`过账：${d.postedByName} ${new Date(d.postedAt).toLocaleString()}`} {d.cancelReason&&`取消：${d.cancelledByName} ${d.cancelReason}`}</p>)}{detail.status!=="CANCELLED" && !detail.directDocuments.some(d=>d.status!=="CANCELLED") && <Button onClick={()=>goTo(`/business/sales/new?directPurchaseId=${detail.id}`)}>关联新建直送销售</Button>}</div>}
            <DataTable
              columns={[
                { title: "商品", key: "product", render: (_, item) => [item.productCode,item.productName,item.productModel,item.productSpecification].filter(Boolean).join(" · ") },
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
              创建人：{detail.createdByName} · 创建时间：{new Date(detail.createTime).toLocaleString()}
              {detail.postedByName && ` · 过账人：${detail.postedByName}${detail.postedAt ? ` · 实际过账时间：${new Date(detail.postedAt).toLocaleString()}` : ""}`}
              {detail.cancelledByName && ` · 取消人：${detail.cancelledByName}`}
            </p>
            <DirectDeliveryTrace trace={detail.directTrace}/>
            {detail.cancelReason && <p className="mt-space-2 text-sm">取消原因：{detail.cancelReason}</p>}
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
      <FormDialog open={postTarget !== null} title="过账采购入库单" description={postTarget ? `${postTarget.documentNo} · ${postTarget.partnerName} · 合计 ¥${postTarget.totalAmount}。确认后库存与应付同时生效。` : undefined} submitText="确认过账" loading={saving} onCancel={() => setPostTarget(null)} onSubmit={confirmPost}>
        {postTarget && <p className="text-sm">明细 {postTarget.items.length} 行 · 当前版本 {postTarget.version}</p>}
      </FormDialog>
    </div>
  );
}

function statusText(s:string){return {DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[s]??s}
