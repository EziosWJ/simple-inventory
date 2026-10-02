import { postingImpact, cancellationImpact } from "@/lib/document-feedback";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { useDocumentSearch } from "@/hooks/use-document-search";
import { withBusinessReturn } from "@/lib/business-navigation";
import { Field } from "@/components/common/field";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { DataTableCard } from "@/components/common/data-table-card";
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
 function reset(){apply({documentNo:"",partnerId:"",productId:"",status:"",businessFrom:"",businessTo:""});setReload(v=>v+1)}
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
 const columns:DataTableColumn<SaleDraft>[]=[{title:"单号",dataIndex:"documentNo"},{title:"客户",dataIndex:"partnerName"},{title:"业务日期",dataIndex:"businessDate"},{title:"状态",dataIndex:"status",render:v=>v==="DRAFT"?"草稿":v==="POSTED"?"已过账":"已取消"},{title:"金额",dataIndex:"totalAmount",render:v=>`¥${v}`},{title:"操作",dataIndex:"id",render:(_,r)=><div className="flex gap-2"><Button size="sm" variant="secondary" onClick={()=>openDetail(r)}>详情</Button>{r.status==="DRAFT"&&<><Button size="sm" variant="secondary" onClick={()=>editDraft(r)}>编辑</Button><Button size="sm" onClick={()=>void post(r)}>过账</Button><Button size="sm" variant="secondary" onClick={()=>void cancel(r)}>取消</Button></>}{r.status==="POSTED"&&<Button size="sm" variant="secondary" onClick={()=>void cancel(r)}>取消已过账单</Button>}</div>}];
 return <div className="space-y-4"><PageHeader title="销售出库" description="维护客户交付草稿；草稿不改变库存或应收。" actions={<Button onClick={newDraft}>新建销售草稿</Button>}/>
  {error&&<div role="alert" className="rounded border border-red-300 p-3 text-red-700">{error}</div>}
  <BusinessFeedback feedback={feedback} />
<SearchFilterBar actions={<><Button onClick={search}>查询</Button><Button onClick={reset}>重置</Button><Button onClick={()=>setReload(v=>v+1)}>刷新</Button></>}>
    <Field label="销售单号"><Input aria-label="销售单号" placeholder="销售单号" value={filters.documentNo} onChange={e=>setFilters({...filters,documentNo:e.target.value})}/></Field>
    <Field label="客户"><PartnerSelect label="客户筛选" historical value={Number(filters.partnerId)} onChange={p=>setFilters({...filters,partnerId:p?String(p.id):""})}/></Field>
    <Field label="商品/服务"><ProductSelect label="商品或服务筛选" historical value={Number(filters.productId)} onChange={p=>setFilters({...filters,productId:p?String(p.id):""})}/></Field>
    <Field label="状态"><Select aria-label="销售状态" value={filters.status} onChange={e=>setFilters({...filters,status:e.target.value})}><option value="">全部状态</option><option value="DRAFT">草稿</option><option value="POSTED">已过账</option><option value="CANCELLED">已取消</option></Select></Field>
    <Field label="业务日期起"><Input aria-label="业务日期起" type="date" value={filters.businessFrom} onChange={e=>setFilters({...filters,businessFrom:e.target.value})}/></Field>
    <Field label="业务日期止"><Input aria-label="业务日期止" type="date" value={filters.businessTo} onChange={e=>setFilters({...filters,businessTo:e.target.value})}/></Field>
  </SearchFilterBar>
  <DataTableCard toolbar={<TableToolbar title="销售出库单" description="按单号、客户、商品/服务、状态和业务日期组合查询。"/>} pagination={<Pagination page={page} pageSize={10} total={total} disabled={loading} onPageChange={setPage}/>}>
    <DataTable<SaleDraft> columns={columns} dataSource={records} loading={loading} error={error} rowKey="id" empty="暂无销售单"/>
  </DataTableCard>
  {detail&&<div role="dialog" className="fixed inset-0 z-40 grid place-items-center bg-black/30 p-4" onClick={closeDetail}><div className="max-h-[90vh] w-full max-w-2xl overflow-auto rounded bg-white p-6" onClick={e=>e.stopPropagation()}><div className="flex justify-between"><h2 className="text-lg font-semibold">销售单 {detail.documentNo}</h2><Button variant="secondary" onClick={closeDetail}>关闭</Button></div><BusinessReturnLink/><p>{detail.status==="DRAFT"&&<Button onClick={()=>editDraft(detail)}>编辑草稿</Button>}</p><p>{detail.partnerName} · {detail.businessDate} · {detail.status}</p><p>{detail.directDelivery?"直送销售：采购与销售分别过账。":"普通销售"}</p>{detail.directDocuments.map(d=><p key={d.id}><Button variant="secondary" onClick={()=>{goTo(`/business/purchases?purchaseId=${d.id}`)}}>{d.documentNo} · {statusText(d.status)}</Button> {d.postedAt&&`过账：${d.postedByName} ${new Date(d.postedAt).toLocaleString()}`} {d.cancelReason&&`取消：${d.cancelledByName} ${d.cancelReason}`}</p>)}<p>送货：{detail.deliveryContact??""} / {detail.deliveryPhone??""} / {detail.deliveryAddress??""}</p>{detail.status!=="CANCELLED"&&<Button variant="secondary" onClick={()=>{goTo(`/business/sales/${detail.id}/delivery-note`)}}>打印送货单</Button>}{detail.status!=="DRAFT"&&detail.status!=="CANCELLED"&&<Button variant="secondary" onClick={()=>{goTo(`/business/sale-returns?saleId=${detail.id}`)}}>办理退货</Button>}{detail.status!=="DRAFT"&&<p>经营者：{detail.ownerName} / {detail.ownerPhone} / {detail.ownerAddress}</p>}{detail.items.map(i=><p key={i.id}>{i.productCode} {i.productName}（{i.productModel??""} {i.productSpecification??""}，{i.unit}，{i.productType==="SERVICE"?"服务":"实物"}） {i.quantity} × ¥{i.unitPrice} = ¥{i.amount}</p>)}<strong>合计 ¥{detail.totalAmount}</strong><p className="mt-2 text-sm text-text-tertiary">创建人：{detail.createdByName} · 创建时间：{new Date(detail.createTime).toLocaleString()}{detail.postedByName?` · 过账人：${detail.postedByName}${detail.postedAt?` · ${new Date(detail.postedAt).toLocaleString()}`:""}`:""}{detail.cancelledByName?` · 取消人：${detail.cancelledByName}${detail.cancelledAt?` · ${new Date(detail.cancelledAt).toLocaleString()}`:""}`:""}</p><DirectDeliveryTrace trace={detail.directTrace}/>{detail.cancelReason&&<p>取消原因：{detail.cancelReason}</p>}<Button className="mt-3" variant="secondary" onClick={()=>goTo(`/business/partner-ledger?partnerId=${detail.partnerId}&direction=CUSTOMER`)}>查看客户往来</Button></div></div>}
 </div>
}

function statusText(s:string){return {DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[s]??s}
