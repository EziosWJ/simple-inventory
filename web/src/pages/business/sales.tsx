import { businessMonthRange, formatDateTime } from "@/lib/datetime";
import { postingImpact, cancellationImpact } from "@/lib/document-feedback";
import { BusinessFeedback } from "@/components/business/business-feedback";
import { useBusinessFeedback } from "@/hooks/use-business-feedback";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { blankDocumentFilters, useDocumentSearch, type DocumentFilters } from "@/hooks/use-document-search";
import { withBusinessReturn } from "@/lib/business-navigation";
import { cn } from "@/lib/utils";
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
import { cancelSale, getSale, postSale, saleItemPage, salePage, type SaleDraft, type SaleItemPage, type SaleItemRecord, type SaleItemSummary, type SaleLine } from "@/api/business";
import { PageHeader } from "@/components/common/page-header";
import { DataTable } from "@/components/common/data-table";
import { Pagination } from "@/components/common/pagination";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { DataTableColumn } from "@/types";

const pageSizes = [10, 20, 50];
const sortFields = ["businessDate", "unitPrice", "amount", "netAmount"] as const;
type SortField = (typeof sortFields)[number];

export function SalesPage(){
  const { feedback, notify } = useBusinessFeedback();
 const navigate=useNavigate();
 const location=useLocation();
 const {params,setParams,query,filters,setFilters,page,setPage,apply}=useDocumentSearch();
 const view=params.get("view")==="item"?"item":"document";
 const rawSize=Number(params.get("size"));
 const pageSize=pageSizes.includes(rawSize)?rawSize:10;
 const rawItemPage=Number(params.get("ipage"));
 const itemPage=Number.isSafeInteger(rawItemPage)&&rawItemPage>0?rawItemPage:1;
 const rawSort=params.get("sort")??"";
 const sort=(sortFields as readonly string[]).includes(rawSort)?(rawSort as SortField):"";
 const dir=params.get("dir")==="asc"?"asc":"desc";
 const [records,setRecords]=useState<SaleDraft[]>([]);
 const [total,setTotal]=useState(0),[reload,setReload]=useState(0),[error,setError]=useState(""),[loading,setLoading]=useState(false);
 const [detail,setDetail]=useState<SaleDraft|null>(null);
 const [expanded,setExpanded]=useState<string[]>([]);
 const [itemResult,setItemResult]=useState<SaleItemPage|null>(null);
 const [itemError,setItemError]=useState(""),[itemLoading,setItemLoading]=useState(false);
 const patch=useCallback((changes:Record<string,string|null>)=>{const p=new URLSearchParams(params);for(const [key,value] of Object.entries(changes)){if(value===null||value==="")p.delete(key);else p.set(key,value);}setParams(p)},[params,setParams]);
 const load=useCallback(()=>{let active=true;setLoading(true);void salePage({page,pageSize,...query}).then(r=>{if(active){setRecords(r.records);setTotal(r.total);setError("")}}).catch((e:unknown)=>{if(active){setRecords([]);setTotal(0);setError(e instanceof Error?e.message:"销售单加载失败")}}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[page,pageSize,query]);
 const loadItems=useCallback(()=>{let active=true;setItemLoading(true);const args:Record<string,string|number>={page:itemPage,pageSize,...query};if(sort){args.sortBy=sort;args.sortDir=dir;}void saleItemPage(args).then(r=>{if(active){setItemResult(r);setItemError("")}}).catch((e:unknown)=>{if(active){setItemResult(null);setItemError(e instanceof Error?e.message:"销售明细加载失败")}}).finally(()=>{if(active)setItemLoading(false)});return()=>{active=false}},[itemPage,pageSize,sort,dir,query]);
 function search(){if(filters.businessFrom&&filters.businessTo&&filters.businessFrom>filters.businessTo){setError("业务日期起始不能晚于结束日期。");return}setError("");apply(filters,["ipage"]);setReload(v=>v+1)}
 function applyConditions(next:DocumentFilters){setFilters(next);setError("");apply(next,["ipage"]);setReload(v=>v+1)}
 function reset(){setError("");apply({...blankDocumentFilters},["ipage"]);setReload(v=>v+1)}
 function switchView(next:"document"|"item"){if(next===view)return;patch({view:next==="item"?"item":null,[next==="item"?"ipage":"page"]:null})}
 function setItemPage(next:number){patch({ipage:String(next)})}
 function changePageSize(next:number){patch({size:next===10?null:String(next),[view==="item"?"ipage":"page"]:null})}
 function toggleSort(field:SortField){const next=sort===field?(dir==="asc"?"desc":"asc"):field==="businessDate"?"desc":"asc";patch({sort:field,dir:next,ipage:null})}
 function toggleExpanded(id:number){const key=String(id);setExpanded(current=>current.includes(key)?current.filter(value=>value!==key):[...current,key])}
 useEffect(()=>{if(view!=="document")return;return load()},[view,load,reload]);
 useEffect(()=>{if(view!=="item")return;return loadItems()},[view,loadItems,reload]);
 const directSourceId=params.get("directPurchaseId")??"";
 useEffect(()=>{if(directSourceId)navigate(`/business/sales/new?directPurchaseId=${encodeURIComponent(directSourceId)}`,{replace:true})},[directSourceId,navigate]);
 useEffect(()=>{let active=true;const id=Number(params.get("saleId"));if(id>0)void getSale(id).then(d=>{if(active)setDetail(d)}).catch((e:unknown)=>{if(active)setError(e instanceof Error?e.message:"读取销售来源单失败")});else setDetail(null);return()=>{active=false}},[params]);
 function closeDetail(){setDetail(null);const p=new URLSearchParams(params);p.delete("saleId");setParams(p,{replace:true})}
 function openDetail(id:number){const p=new URLSearchParams(params);p.set("saleId",String(id));setParams(p)}
 function newDraft(){goTo("/business/sales/new")}
 function editDraft(row:SaleDraft){goTo(`/business/sales/${row.id}/edit`)}
 async function cancel(d:SaleDraft){const reason=window.prompt("请输入取消原因");if(!reason?.trim())return;try{await cancelSale(d.id,{version:d.version,reason});setError("");notify({type:"success",title:d.status==="POSTED"?"销售单冲销取消成功":"销售草稿取消成功",description:cancellationImpact(d,"sale"),documentNo:d.documentNo,status:"已取消"});setReload(v=>v+1)}catch(e){const message=e instanceof Error?e.message:"取消失败";setError(message);notify({type:"error",title:"取消失败",description:message})}}
 async function post(d:SaleDraft){const goods=new Map<string,number>();for(const i of d.items)if(i.productType==="GOODS")goods.set(i.productName,(goods.get(i.productName)??0)+Number(i.quantity));const impact=[...goods].map(([name,n])=>`${name}：-${n}`).join("\n")||"无实物库存变化";if(!window.confirm(`确认过账 ${d.documentNo}？\n客户应收增加 ¥${d.totalAmount}\n${impact}`))return;try{const posted=await postSale(d.id,d.version);setError("");notify({type:"success",title:"销售单过账成功",description:postingImpact(posted,"sale"),documentNo:posted.documentNo,status:"已过账"});setDetail(posted);setReload(v=>v+1)}catch(e){const message=e instanceof Error?e.message:"过账失败，单据内容已保留";setError(message);notify({type:"error",title:"过账失败",description:message})}}
 const goTo=(path:string)=>navigate(withBusinessReturn(path,location.pathname+location.search));
 const matchedLine=(item:SaleLine)=>{
   if(query.productId)return String(item.productId)===query.productId;
   const keyword=query.productKeyword.trim().toLowerCase();
   if(!keyword)return false;
   const text=[item.productCode,item.productName,item.productModel,item.productSpecification].filter(Boolean).join(" ").toLowerCase();
   return keyword.split(/\s+/).every(word=>text.includes(word));
 };
 const summaryParts=(row:SaleDraft)=>{
   const parts=row.items.map(item=>({text:`${item.productName} ×${item.quantity}${item.unit}`,matched:matchedLine(item)}));
   return {parts,count:parts.length,full:parts.map(part=>part.text).join("、")};
 };
 const columns:DataTableColumn<SaleDraft>[]=[{title:"单号",dataIndex:"documentNo",render:(v,r)=><>{String(v)}{r.matchesHistory&&<span className="block text-xs text-primary">匹配历史资料</span>}</>},{title:"客户",dataIndex:"partnerName"},{title:"业务日期",dataIndex:"businessDate"},{title:"销售内容",key:"items",width:"30%",render:(_,r)=>{const summary=summaryParts(r);return <div className="min-w-[14rem] space-y-1"><p className="line-clamp-2 text-sm text-text-secondary" title={summary.full}>{summary.parts.map((part,index)=><span key={index} data-matched={part.matched?"true":undefined} className={cn(part.matched&&"font-medium text-primary")}>{index>0?"、":""}{part.text}</span>)}</p><Button size="sm" variant="ghost" aria-expanded={expanded.includes(String(r.id))} onClick={()=>toggleExpanded(r.id)}>{expanded.includes(String(r.id))?"收起明细":"展开明细"}</Button></div>}},{title:"状态",dataIndex:"status",render:v=>v==="DRAFT"?"草稿":v==="POSTED"?"已过账":"已取消"},{title:"金额",dataIndex:"totalAmount",render:v=>`¥${v}`},{title:"操作",dataIndex:"id",render:(_,r)=><div className="flex gap-2"><Button size="sm" variant="secondary" onClick={()=>openDetail(r.id)}>详情</Button>{r.status==="DRAFT"&&<><Button size="sm" variant="secondary" onClick={()=>editDraft(r)}>编辑</Button><Button size="sm" onClick={()=>void post(r)}>过账</Button><Button size="sm" variant="secondary" onClick={()=>void cancel(r)}>取消</Button></>}{r.status==="POSTED"&&<Button size="sm" variant="secondary" onClick={()=>void cancel(r)}>取消已过账单</Button>}</div>}];
 const sortHeader=(label:string,field:SortField)=><button type="button" aria-label={`按${label}排序`} aria-pressed={sort===field} className={cn("inline-flex items-center gap-1 font-medium",sort===field?"text-primary":"text-text-secondary hover:text-text-primary")} onClick={()=>toggleSort(field)}>{label}<span aria-hidden="true">{sort===field?(dir==="asc"?"▲":"▼"):"↕"}</span></button>;
 const itemColumns:DataTableColumn<SaleItemRecord>[]=[
   {title:sortHeader("业务日期","businessDate"),key:"businessDate",nowrap:true,render:(_,r)=>r.businessDate},
   {title:"客户",key:"partner",render:(_,r)=><span className="text-block"><span>{r.partnerName}</span>{r.matchesHistory&&<span className="block text-xs text-primary">匹配历史资料</span>}</span>},
   {title:"商品/服务",key:"product",width:"22%",render:(_,r)=>{const description=[r.productModel,r.productSpecification].filter(value=>Boolean(value?.trim())).join(" · ");return <div className="min-w-[12rem] space-y-1"><div className="font-medium text-text-primary">{[r.productCode,r.productName].filter(Boolean).join(" · ")}</div>{description&&<div className="text-sm text-text-secondary">{description}</div>}<div className="text-xs text-text-tertiary">{r.productType==="SERVICE"?"服务":"实物"}</div></div>}},
   {title:"数量",key:"quantity",align:"right",nowrap:true,render:(_,r)=><span className="tabular-nums">{r.quantity} {r.unit}</span>},
   {title:sortHeader("成交单价","unitPrice"),key:"unitPrice",align:"right",nowrap:true,render:(_,r)=>`¥${r.unitPrice}`},
   {title:sortHeader("原金额","amount"),key:"amount",align:"right",nowrap:true,render:(_,r)=>`¥${r.amount}`},
   {title:"有效退货",key:"returned",align:"right",nowrap:true,render:(_,r)=>r.returnsApplicable?<span className="tabular-nums">{r.returnedQuantity} {r.unit}<span className="block text-xs text-text-tertiary">-¥{r.returnedAmount}</span></span>:<span className="text-text-tertiary" title="服务不能退货">不适用</span>},
   {title:sortHeader("净销售额","netAmount"),key:"netAmount",align:"right",nowrap:true,render:(_,r)=>r.netApplicable&&r.netQuantity!=null&&r.netAmount!=null?<span className="tabular-nums font-medium">{r.netQuantity} {r.unit}<span className="block text-xs text-text-tertiary">¥{r.netAmount}</span></span>:<span className="text-text-tertiary" title="草稿与已取消销售不是有效销售">不适用</span>},
   {title:"来源单号",key:"documentNo",render:(_,r)=><Button size="sm" variant="secondary" onClick={()=>openDetail(r.saleId)}>{r.documentNo}</Button>},
   {title:"状态",key:"status",render:(_,r)=><StatusTag tone={statusTone(r.status)}>{statusText(r.status)}</StatusTag>},
 ];
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
 const activeError=view==="item"?itemError:error;
 return <div className="space-y-4"><PageHeader title="销售出库" description="维护客户交付草稿；草稿不改变库存或应收。" actions={<Button onClick={newDraft}>新建销售草稿</Button>}/>
  {activeError&&<div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded border border-red-300 p-3 text-red-700"><span>{activeError}</span><Button size="sm" variant="secondary" onClick={()=>setReload(v=>v+1)}>重试查询</Button></div>}
  <BusinessFeedback feedback={feedback} />
<SearchFilterBar
    stacked
    leadingActions={<>
      <Button variant="secondary" onClick={()=>{const month=businessMonthRange();applyConditions({...filters,businessFrom:month.from,businessTo:month.to})}}>本月</Button>
      <Button variant="secondary" aria-pressed={filters.status==="POSTED"} onClick={()=>applyConditions({...filters,status:"POSTED"})}>已过账</Button>
    </>}
    actions={<><Button onClick={search}>查询</Button><Button onClick={reset}>重置</Button></>}
  >
    <Field label="销售单号"><Input aria-label="销售单号" placeholder="销售单号" value={filters.documentNo} onChange={e=>setFilters({...filters,documentNo:e.target.value})}/></Field>
    <Field label="客户"><PartnerSelect label="客户筛选" historical value={Number(filters.partnerId)} keyword={query.partnerKeyword} onDraftKeywordChange={k=>setFilters(current=>({...current,partnerKeyword:k}))} onKeywordChange={k=>{const next={...filters,partnerId:"",partnerKeyword:k};setFilters(next);apply(next,["ipage"])}} onChange={p=>{const next={...filters,partnerId:p?String(p.id):"",partnerKeyword:""};setFilters(next);apply(next,["ipage"])}} /></Field>
    <Field label="商品/服务"><ProductSelect label="商品或服务筛选" historical value={Number(filters.productId)} keyword={query.productKeyword} onDraftKeywordChange={k=>setFilters(current=>({...current,productKeyword:k}))} onKeywordChange={k=>{const next={...filters,productId:"",productKeyword:k};setFilters(next);apply(next,["ipage"])}} onChange={p=>{const next={...filters,productId:p?String(p.id):"",productKeyword:""};setFilters(next);apply(next,["ipage"])}} /></Field>
    <Field label="状态"><Select aria-label="销售状态" value={filters.status} onChange={e=>setFilters({...filters,status:e.target.value})}><option value="">全部状态</option><option value="DRAFT">草稿</option><option value="POSTED">已过账</option><option value="CANCELLED">已取消</option></Select></Field>
    <Field label="业务日期起"><Input aria-label="业务日期起" type="date" value={filters.businessFrom} onChange={e=>setFilters({...filters,businessFrom:e.target.value})}/></Field>
    <Field label="业务日期止"><Input aria-label="业务日期止" type="date" value={filters.businessTo} onChange={e=>setFilters({...filters,businessTo:e.target.value})}/></Field>
  </SearchFilterBar>
  <p className="text-xs text-text-secondary">默认查询全部日期与状态。关键词查当前档案及已保存的商品快照；旧品牌和供应商联系方式仅查当前档案。销售还可匹配已保存的客户名称。按明细的合计覆盖全部筛选结果，并扣除截至当前仍已过账的销售退货。</p>
  {view==="document"
    ? <DataTableCard
        toolbar={<TableToolbar title="销售出库单" description="按单号、客户、商品/服务、状态和业务日期组合查询；商品筛选时整单展示并突出匹配商品。" actions={<div className="flex flex-wrap items-center gap-space-2"><ViewSwitch view={view} onChange={switchView}/><Button size="sm" variant="secondary" onClick={()=>setReload(v=>v+1)}>刷新</Button></div>} />}
        pagination={<Pagination page={page} pageSize={pageSize} pageSizeOptions={pageSizes} total={total} disabled={loading} onPageChange={setPage} onPageSizeChange={changePageSize} />}
      >
        <DataTable<SaleDraft>
          columns={columns}
          dataSource={records}
          loading={loading}
          error={error}
          rowKey="id"
          minWidth={1080}
          empty="暂无销售单"
          expandedRowKeys={expanded}
          renderExpandedRow={row=><ul className="space-y-1" aria-label={`${row.documentNo} 全部明细`}>{row.items.map(item=><li key={item.id} className="flex flex-wrap items-baseline gap-x-space-4 gap-y-1 text-sm"><span data-matched={matchedLine(item)?"true":undefined} className={cn("min-w-[12rem] flex-1",matchedLine(item)&&"font-medium text-primary")}>{[item.productCode,item.productName].filter(Boolean).join(" · ")}<span className="ml-2 text-xs text-text-tertiary">{[item.productModel,item.productSpecification].filter(Boolean).join(" · ")}</span><span className="ml-2 text-xs text-text-tertiary">{item.productType==="SERVICE"?"服务":"实物"}</span></span><span className="tabular-nums">¥{item.unitPrice}</span><span className="tabular-nums font-medium">¥{item.amount}</span><span className="whitespace-nowrap tabular-nums">{item.quantity} {item.unit}</span></li>)}</ul>}
        />
      </DataTableCard>
    : <DataTableCard
        toolbar={<TableToolbar title="销售明细" description="跨单查看销售对象、数量与成交价；合计覆盖全部筛选结果而不是当前页。" actions={<div className="flex flex-wrap items-center gap-space-2"><ViewSwitch view={view} onChange={switchView}/><Button size="sm" variant="secondary" onClick={()=>setReload(v=>v+1)}>刷新</Button></div>} />}
        pagination={<Pagination page={itemPage} pageSize={pageSize} pageSizeOptions={pageSizes} total={itemResult?.total??0} disabled={itemLoading} onPageChange={setItemPage} onPageSizeChange={changePageSize} />}
      >
        <ItemSummaryPanel summary={itemResult?.summary} loading={itemLoading}/>
        <DataTable<SaleItemRecord>
          columns={itemColumns}
          dataSource={itemResult?.records??[]}
          loading={itemLoading}
          error={itemError}
          rowKey="id"
          minWidth={1240}
          empty="没有匹配的销售明细"
        />
      </DataTableCard>}
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

function ViewSwitch({ view, onChange }: { view: "document" | "item"; onChange: (next: "document" | "item") => void }) {
  const options = [{ value: "document", label: "按单据" }, { value: "item", label: "按明细" }] as const;
  return (
    <div role="group" aria-label="销售浏览视图" className="flex shrink-0 gap-1 rounded-control border border-border p-0.5">
      {options.map(option => (
        <Button
          key={option.value}
          size="sm"
          variant={view === option.value ? "primary" : "ghost"}
          aria-pressed={view === option.value}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  );
}

function ItemSummaryPanel({ summary, loading }: { summary?: SaleItemSummary; loading: boolean }) {
  const amounts = summary?.amounts ?? [];
  const quantities = summary?.quantities ?? [];
  return (
    <section className="space-y-space-3 border-b border-border px-card py-space-4" aria-label="销售明细全结果合计" aria-busy={loading}>
      <div>
        <h3 className="text-card-title font-semibold">全部匹配明细合计</h3>
        <p className="mt-space-1 text-body-secondary text-text-tertiary">
          按销售业务日期筛选，扣除截至当前仍已过账的有效退货；草稿与已取消销售保留原值，但不计入净销售。
        </p>
      </div>
      <div className="grid gap-space-3 sm:grid-cols-3">
        {amounts.map(amount => (
          <div key={amount.status} className="space-y-space-2 rounded-admin border border-border p-space-3">
            <div className="flex items-center justify-between gap-2">
              <StatusTag tone={statusTone(amount.status)}>{statusText(amount.status)}</StatusTag>
              <span className="text-sm text-text-tertiary">{amount.itemCount} 项</span>
            </div>
            <dl className="space-y-1 text-sm">
              <div className="flex items-center justify-between gap-2">
                <dt className="text-text-secondary">原金额</dt>
                <dd className="tabular-nums">¥{amount.originalAmount}</dd>
              </div>
              {amount.returnedAmount != null && (
                <div className="flex items-center justify-between gap-2">
                  <dt className="text-text-secondary">有效退货</dt>
                  <dd className="tabular-nums">-¥{amount.returnedAmount}</dd>
                </div>
              )}
              {amount.netAmount != null && (
                <div className="flex items-center justify-between gap-2 font-medium">
                  <dt>净销售额</dt>
                  <dd className="tabular-nums">¥{amount.netAmount}</dd>
                </div>
              )}
            </dl>
          </div>
        ))}
      </div>
      <div className="space-y-space-2">
        <h4 className="text-sm font-medium">数量合计（按商品身份与单位）</h4>
        {quantities.length === 0 ? (
          <p className="text-sm text-text-secondary">没有匹配的数量。</p>
        ) : (
          <ul className="max-h-56 space-y-1 overflow-y-auto pr-1 text-sm" aria-label="数量合计明细">
            {quantities.map(group => (
              <li key={`${group.status}-${group.productId}-${group.unit}`} className="flex flex-wrap items-center justify-between gap-x-space-3 gap-y-1 border-b border-border pb-1 last:border-0">
                <span className="flex min-w-0 items-center gap-2">
                  <StatusTag tone={statusTone(group.status)}>{statusText(group.status)}</StatusTag>
                  <span className="truncate">{[group.productCode, group.productName].filter(Boolean).join(" · ")}</span>
                </span>
                <span className="tabular-nums text-text-secondary">
                  原 {group.quantity} {group.unit}
                  {group.returnsApplicable && group.returnedQuantity != null && ` · 退 ${group.returnedQuantity}`}
                  {group.netQuantity != null && ` · 净 ${group.netQuantity}`}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
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
