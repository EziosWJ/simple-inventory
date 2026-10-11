import { formatDateTime } from "@/lib/datetime";
import { postingImpact, cancellationImpact } from "@/lib/document-feedback";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { useDocumentSearch } from "@/hooks/use-document-search";
import { withBusinessReturn } from "@/lib/business-navigation";
import { Field } from "@/components/common/field";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { StatusTag } from "@/components/common/status-tag";
import { TableToolbar } from "@/components/common/table-toolbar";
import { PartnerSelect, ProductSelect } from "@/components/business/master-data-select";
import { DirectDeliveryTrace } from "@/components/business/direct-delivery-trace";
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { cancelSale, getSale, postSale, salePage, type SaleDraft } from "@/api/business";
import { PageHeader } from "@/components/common/page-header";
import { DataTable } from "@/components/common/data-table";
import { Pagination } from "@/components/common/pagination";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { DataTableColumn } from "@/types";

export function SalesPage(){
  const { feedback, notify } = useBusinessFeedback();
 const navigate=useNavigate();
 const location=useLocation();
 const {params:searchParams,setParams:setSearchParams,query,filters,setFilters,page,setPage,apply}=useDocumentSearch();
 const goTo=(path:string)=>navigate(withBusinessReturn(path,location.pathname+location.search));
 const [records,setRecords]=useState<SaleDraft[]>([]);
 const [total,setTotal]=useState(0),[reload,setReload]=useState(0),[error,setError]=useState(""),[loading,setLoading]=useState(false);
 const [detail,setDetail]=useState<SaleDraft|null>(null);
 const load=useCallback(()=>{let active=true;setLoading(true);void salePage({page,pageSize:10,...query}).then(r=>{if(active){setRecords(r.records);setTotal(r.total);setError("")}}).catch((e:unknown)=>{if(active){setRecords([]);setTotal(0);setError(e instanceof Error?e.message:"销售单加载失败")}}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[page,query]);
 function search(){if(filters.businessFrom&&filters.businessTo&&filters.businessFrom>filters.businessTo){setError("业务日期起始不能晚于结束日期。");return;}apply(filters);setReload(v=>v+1)}
 function reset(){apply({documentNo:"",partnerId:"",productId:"",partnerKeyword:"",productKeyword:"",status:"",businessFrom:"",businessTo:""});setReload(v=>v+1)}
 useEffect(()=>load(),[load,reload]);
 const directSourceId=searchParams.get("directPurchaseId")??"";
 useEffect(()=>{if(directSourceId)navigate(`/business/sales/new?directPurchaseId=${encodeURIComponent(directSourceId)}`,{replace:true})},[directSourceId,navigate]);
 useEffect(()=>{let active=true;const id=Number(searchParams.get("saleId"));if(id>0)void getSale(id).then(d=>{if(active)setDetail(d)}).catch((e:unknown)=>{if(active)setError(e instanceof Error?e.message:"读取销售来源单失败")});else setDetail(null);return()=>{active=false}},[searchParams]);
 function closeDetail(){setDetail(null);const p=new URLSearchParams(searchParams);p.delete("saleId");setSearchParams(p,{replace:true})}
 function openDetail(d:SaleDraft){const p=new URLSearchParams(searchParams);p.set("saleId",String(d.id));setSearchParams(p)}
 function newDraft(){goTo("/business/sales/new")}
 function editDraft(row:SaleDraft){goTo(`/business/sales/${row.id}/edit`)}
 async function cancel(d:SaleDraft){const reason=window.prompt("请输入取消原因");if(!reason?.trim())return;try{await cancelSale(d.id,{version:d.version,reason});setError("");notify({type:"success",title:d.status==="POSTED"?"销售单冲销取消成功":"销售草稿取消成功",description:cancellationImpact(d,"sale"),documentNo:d.documentNo,status:"已取消"});setReload(v=>v+1)}catch(e){const message=e instanceof Error?e.message:"取消失败";setError(message);notify({type:"error",title:"取消失败",description:message})}}
 async function post(d:SaleDraft){const goods=new Map<string,number>();for(const i of d.items)if(i.productType==="GOODS")goods.set(i.productName,(goods.get(i.productName)??0)+Number(i.quantity));const impact=[...goods].map(([name,n])=>`${name}：-${n}`).join("\n")||"无实物库存变化";if(!window.confirm(`确认过账 ${d.documentNo}？\n客户应收增加 ¥${d.totalAmount}\n${impact}`))return;try{const posted=await postSale(d.id,d.version);setError("");notify({type:"success",title:"销售单过账成功",description:postingImpact(posted,"sale"),documentNo:posted.documentNo,status:"已过账"});setDetail(posted);setReload(v=>v+1)}catch(e){const message=e instanceof Error?e.message:"过账失败，单据内容已保留";setError(message);notify({type:"error",title:"过账失败",description:message})}}
 const columns:DataTableColumn<SaleDraft>[]=[{title:"单号",dataIndex:"documentNo",render:(v,r)=><>{String(v)}{r.matchesHistory&&<span className="block text-xs text-primary">匹配历史资料</span>}</>},{title:"客户",dataIndex:"partnerName"},{title:"业务日期",dataIndex:"businessDate"},{title:"状态",dataIndex:"status",render:v=>v==="DRAFT"?"草稿":v==="POSTED"?"已过账":"已取消"},{title:"金额",dataIndex:"totalAmount",render:v=>`¥${v}`},{title:"操作",dataIndex:"id",render:(_,r)=><div className="flex gap-2"><Button size="sm" variant="secondary" onClick={()=>openDetail(r)}>详情</Button>{r.status==="DRAFT"&&<><Button size="sm" variant="secondary" onClick={()=>editDraft(r)}>编辑</Button><Button size="sm" onClick={()=>void post(r)}>过账</Button><Button size="sm" variant="secondary" onClick={()=>void cancel(r)}>取消</Button></>}{r.status==="POSTED"&&<Button size="sm" variant="secondary" onClick={()=>void cancel(r)}>取消已过账单</Button>}</div>}];
 const deliveryInfo = [
   { label: "送货联系人", value: detail?.deliveryContact },
   { label: "联系电话", value: detail?.deliveryPhone },
   { label: "送货地址", value: detail?.deliveryAddress },
 ].filter(field => Boolean(field.value?.trim()));
 const ownerInfo = detail && detail.status !== "DRAFT"
   ? [
       { label: "经营者", value: detail.ownerName },
       { label: "联系电话", value: detail.ownerPhone },
       { label: "联系地址", value: detail.ownerAddress },
     ].filter(field => Boolean(field.value?.trim()))
   : [];
 return <div className="space-y-4"><PageHeader title="销售出库" description="维护客户交付草稿；草稿不改变库存或应收。" actions={<Button onClick={newDraft}>新建销售草稿</Button>}/>
  {error&&<div role="alert" className="rounded border border-red-300 p-3 text-red-700">{error}</div>}
  <BusinessFeedback feedback={feedback} />
<SearchFilterBar actions={<><Button onClick={search}>查询</Button><Button onClick={reset}>重置</Button><Button onClick={()=>setReload(v=>v+1)}>刷新</Button></>}>
    <Field label="销售单号"><Input aria-label="销售单号" placeholder="销售单号" value={filters.documentNo} onChange={e=>setFilters({...filters,documentNo:e.target.value})}/></Field>
    <Field label="客户"><PartnerSelect label="客户筛选" historical value={Number(filters.partnerId)} keyword={query.partnerKeyword} onDraftKeywordChange={k=>setFilters(current=>({...current,partnerKeyword:k}))} onKeywordChange={k=>{const next={...filters,partnerId:"",partnerKeyword:k};setFilters(next);apply(next)}} onChange={p=>{const next={...filters,partnerId:p?String(p.id):"",partnerKeyword:""};setFilters(next);apply(next)}} /></Field>
    <Field label="商品/服务"><ProductSelect label="商品或服务筛选" historical value={Number(filters.productId)} keyword={query.productKeyword} onDraftKeywordChange={k=>setFilters(current=>({...current,productKeyword:k}))} onKeywordChange={k=>{const next={...filters,productId:"",productKeyword:k};setFilters(next);apply(next)}} onChange={p=>{const next={...filters,productId:p?String(p.id):"",productKeyword:""};setFilters(next);apply(next)}} /></Field>
    <Field label="状态"><Select aria-label="销售状态" value={filters.status} onChange={e=>setFilters({...filters,status:e.target.value})}><option value="">全部状态</option><option value="DRAFT">草稿</option><option value="POSTED">已过账</option><option value="CANCELLED">已取消</option></Select></Field>
    <Field label="业务日期起"><Input aria-label="业务日期起" type="date" value={filters.businessFrom} onChange={e=>setFilters({...filters,businessFrom:e.target.value})}/></Field>
    <Field label="业务日期止"><Input aria-label="业务日期止" type="date" value={filters.businessTo} onChange={e=>setFilters({...filters,businessTo:e.target.value})}/></Field>
  </SearchFilterBar>
  <p className="text-xs text-text-secondary">关键词查当前档案及已保存的商品快照；旧品牌和供应商联系方式仅查当前档案。销售还可匹配已保存的客户名称。</p>
<DataTableCard
    toolbar={<TableToolbar title="销售出库单" description="按单号、客户、商品/服务、状态和业务日期组合查询。" />}
    pagination={<Pagination page={page} pageSize={10} total={total} disabled={loading} onPageChange={setPage} />}
  >
    <DataTable<SaleDraft> columns={columns} dataSource={records} loading={loading} error={error} rowKey="id" empty="暂无销售单"/>
  </DataTableCard>
  <DetailDialog
    open={detail !== null}
    title={detail ? `销售单 · ${detail.documentNo}` : "销售单详情"}
    description={detail ? (
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span>{detail.partnerName}</span>
        <span aria-hidden="true">·</span>
        <span>{detail.businessDate}</span>
        <StatusTag tone={statusTone(detail.status)}>{statusText(detail.status)}</StatusTag>
        <span>{detail.directDelivery ? "直送销售" : "普通销售"}</span>
      </div>
    ) : undefined}
    onCancel={closeDetail}
    footer={detail && (
      <div className="flex flex-col gap-space-3 border-t border-border pt-space-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <div className="[&>div]:mb-0"><BusinessReturnLink /></div>
          <Button
            variant="secondary"
            onClick={() => goTo(`/business/partner-ledger?partnerId=${detail.partnerId}&direction=CUSTOMER`)}
          >
            查看客户往来
          </Button>
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          {detail.status === "DRAFT" && (
            <Button onClick={() => editDraft(detail)}>编辑草稿</Button>
          )}
          {detail.status !== "CANCELLED" && (
            <Button
              variant="secondary"
              onClick={() => goTo(`/business/sales/${detail.id}/delivery-note`)}
            >
              打印送货单
            </Button>
          )}
          {detail.status === "POSTED" && (
            <Button onClick={() => goTo(`/business/sale-returns?saleId=${detail.id}`)}>
              办理退货
            </Button>
          )}
        </div>
      </div>
    )}
  >
    {detail && (
      <div className="space-y-space-5">
        {detail.directDelivery && (
          <p className="rounded-control bg-neutral-background p-space-3 text-sm text-text-secondary">
            直送销售：采购与销售单据分别过账。
          </p>
        )}

        {(deliveryInfo.length > 0 || ownerInfo.length > 0) && (
          <section className="grid gap-x-space-6 gap-y-space-4 sm:grid-cols-2" aria-label="送货及经营者资料">
            {deliveryInfo.length > 0 && (
              <div className="space-y-space-2">
                <h3 className="font-medium">送货资料</h3>
                {deliveryInfo.map(field => (
                  <InfoField key={field.label} label={field.label} value={field.value} />
                ))}
              </div>
            )}
            {ownerInfo.length > 0 && (
              <div className="space-y-space-2">
                <h3 className="font-medium">经营者资料</h3>
                {ownerInfo.map(field => (
                  <InfoField key={field.label} label={field.label} value={field.value} />
                ))}
              </div>
            )}
          </section>
        )}

        {detail.directDocuments.length > 0 && (
          <section className="space-y-space-3 border-t border-border pt-space-4" aria-label="关联采购单">
            <h3 className="font-medium">关联采购单</h3>
            {detail.directDocuments.map(document => (
              <div key={document.id} className="flex flex-wrap items-center justify-between gap-2">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => goTo(`/business/purchases?purchaseId=${document.id}`)}
                >
                  {document.documentNo}
                </Button>
                <div className="flex flex-wrap items-center gap-2 text-sm text-text-secondary">
                  <StatusTag tone={statusTone(document.status)}>{statusText(document.status)}</StatusTag>
                  {document.postedAt && (
                    <span>过账：{document.postedByName ?? "—"} · {formatDateTime(document.postedAt)}</span>
                  )}
                  {document.cancelReason && (
                    <span>取消：{document.cancelledByName ?? "—"} · {document.cancelReason}</span>
                  )}
                </div>
              </div>
            ))}
          </section>
        )}

        <section className="space-y-space-3" aria-label="销售商品明细">
          <h3 className="font-medium">商品明细</h3>
          <DataTable
            columns={[
              {
                title: "商品 / 服务",
                key: "product",
                width: "40%",
                render: (_, item) => {
                  const description = [item.productModel, item.productSpecification]
                    .filter(value => Boolean(value?.trim()))
                    .join(" · ");
                  return (
                    <div className="min-w-[12rem] space-y-1">
                      <div className="font-medium text-text-primary">
                        {[item.productCode, item.productName].filter(Boolean).join(" · ")}
                      </div>
                      {description && <div className="text-sm text-text-secondary">{description}</div>}
                      <div className="flex flex-wrap items-center gap-x-2 text-xs text-text-tertiary">
                        <span>{item.productType === "SERVICE" ? "服务" : "实物"}</span>
                        {item.remark?.trim() && <span>备注：{item.remark.trim()}</span>}
                      </div>
                    </div>
                  );
                },
              },
              { title: "数量", dataIndex: "quantity", align: "right", nowrap: true, width: "15%" },
              { title: "单位", dataIndex: "unit", align: "center", nowrap: true, width: "10%" },
              {
                title: "成交单价",
                dataIndex: "unitPrice",
                align: "right",
                nowrap: true,
                width: "17.5%",
                render: value => `¥${value ?? ""}`,
              },
              {
                title: "金额",
                dataIndex: "amount",
                align: "right",
                nowrap: true,
                width: "17.5%",
                render: value => `¥${value ?? ""}`,
              },
            ]}
            dataSource={detail.items}
            rowKey="id"
            minWidth={760}
            empty="销售单尚无商品明细。"
          />
          <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-space-3">
            <span className="text-sm text-text-tertiary">共 {detail.items.length} 项</span>
            <p className="text-right font-semibold">
              合计 <span className="ml-2 text-lg tabular-nums">¥{detail.totalAmount}</span>
            </p>
          </div>
        </section>

        {detail.remark?.trim() && (
          <section className="space-y-1 border-t border-border pt-space-4" aria-label="单据备注">
            <h3 className="font-medium">单据备注</h3>
            <p className="whitespace-pre-wrap break-words text-sm text-text-secondary">{detail.remark.trim()}</p>
          </section>
        )}

        <section className="grid gap-x-space-6 gap-y-space-2 border-t border-border pt-space-4 text-sm text-text-tertiary sm:grid-cols-2" aria-label="单据操作记录">
          <p>创建人：{detail.createdByName}</p>
          <p>创建时间：{formatDateTime(detail.createTime)}</p>
          {detail.postedByName && <p>过账人：{detail.postedByName}</p>}
          {detail.postedAt && <p>过账时间：{formatDateTime(detail.postedAt)}</p>}
          {(detail.cancelledByName || detail.cancelledAt) && (
            <p>取消人：{detail.cancelledByName ?? "—"}</p>
          )}
          {detail.cancelledAt && <p>取消时间：{formatDateTime(detail.cancelledAt)}</p>}
          {detail.cancelReason && (
            <p className="sm:col-span-2">取消原因：{detail.cancelReason}</p>
          )}
        </section>

        <DirectDeliveryTrace trace={detail.directTrace} />
      </div>
    )}
  </DetailDialog>
 </div>
}

function statusText(s: string) {
  return { DRAFT: "草稿", POSTED: "已过账", CANCELLED: "已取消" }[s] ?? s;
}

function statusTone(s: string): "success" | "warning" | "neutral" {
  return s === "POSTED" ? "success" : s === "DRAFT" ? "warning" : "neutral";
}

function InfoField({ label, value }: { label: string; value?: string | null }) {
  return (
    <p className="text-sm">
      <span className="mr-2 text-text-tertiary">{label}</span>
      <span className="break-words">{value}</span>
    </p>
  );
}
