import { useCallback } from "react";
import { getPartner, getProduct, getPurchase, purchasePage, partnerPage, productPage, type PartnerRecord, type ProductRecord, type PurchaseDraft } from "@/api/business";
import { PagedRecordSelect } from "@/components/common/paged-record-select";

import { RecordPicker } from "@/components/common/record-picker";
import { useAuthStore } from "@/store/auth-store";

type Selection<T> = { inlineBrowse?: boolean; placeholder?: string; label: string; value: number; fallback?: string; disabled?: boolean; historical?: boolean; keyword?: string; onKeywordChange?: (keyword:string)=>void; onQuery?:()=>void; onDraftKeywordChange?:(keyword:string)=>void; onChange: (record: T | null) => void | boolean | Promise<void | boolean> };

export function ProductSelect({ type, historical = false, ...props }: Selection<ProductRecord> & { type?: "GOODS" | "SERVICE" }) {
  const user = useAuthStore(s=>s.user);
  const loadPage = useCallback((keyword: string, page: number, pageSize: number) => productPage({
    keyword, page, pageSize, ...(type ? { type } : {}), ...(historical ? {} : { status: 1 }),
  }), [type, historical]);
  const warning = useCallback((p: ProductRecord) => historical ? "" : p.status !== 1 ? "已停用：请更换启用商品。" : type && p.type !== type ? "商品类型已变化，请重新选择。" : "",[historical,type]);
  return <RecordPicker {...props} recentKey={user?`simple-inventory:recent:${user.id}:product:${`${historical?"history:":""}${type??"sale"}`}`:undefined} loadPage={loadPage} loadRecord={getProduct}
    describe={p => ({name:p.name,details:[p.model,p.specification,p.brand].filter((v):v is string=>Boolean(v)),code:p.code,tags:[p.type === "SERVICE"?"服务":"实物",`单位：${p.unit}`,p.status===0?"已停用":""].filter(Boolean)})}
    warning={warning} />;
}

export function PartnerSelect({ identity, historical = false, ...props }: Selection<PartnerRecord> & { identity?: "CUSTOMER" | "SUPPLIER" }) {
  const user = useAuthStore(s=>s.user);
  const loadPage = useCallback((keyword: string, page: number, pageSize: number) => partnerPage({
    keyword, page, pageSize, ...(identity ? { identity } : {}), ...(historical ? {} : { status: 1 }),
  }), [identity, historical]);
  const warning = useCallback((p: PartnerRecord) => historical ? "" : p.status !== 1 ? "已停用：请更换启用往来单位。" : (identity === "SUPPLIER" && !p.isSupplier) || (identity === "CUSTOMER" && !p.isCustomer) ? "往来身份已变化，请重新选择。" : "",[historical,identity]);
  return <RecordPicker {...props} recentKey={user?`simple-inventory:recent:${user.id}:partner:${`${historical?"history:":""}${identity??"all"}`}`:undefined} loadPage={loadPage} loadRecord={getPartner}
    describe={p => ({name:p.name,details:[p.contact===p.name?null:p.contact,p.phone].filter((v):v is string=>Boolean(v)),code:p.code,tags:[p.isCustomer?"客户":"",p.isSupplier?"供应商":"",p.status===0?"已停用":""].filter(Boolean)})}
    warning={warning} />;
}

export function PurchaseSourceSelect(props: Selection<PurchaseDraft>) {
  const loadPage = useCallback((keyword: string, page: number, pageSize: number) => purchasePage({ documentNo: keyword.trim(), page, pageSize }), []);
  return <PagedRecordSelect {...props} loadPage={loadPage} loadRecord={getPurchase}
    describe={d => `${d.documentNo} · ${d.partnerName} · ${d.businessDate} · ${d.directDelivery ? "直送" : "普通采购"} · ${{DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[d.status]}`}
    warning={d => !d.directDelivery ? "普通采购不能关联直送销售" : d.status === "CANCELLED" ? "已取消采购不能关联" : ""} />;
}
