import { useCallback } from "react";
import { getPartner, getProduct, getPurchase, purchasePage, partnerPage, productPage, type PartnerRecord, type ProductRecord, type PurchaseDraft } from "@/api/business";
import { PagedRecordSelect } from "@/components/common/paged-record-select";

type Selection<T> = { label: string; value: number; fallback?: string; disabled?: boolean; historical?: boolean; onChange: (record: T | null) => void };

export function ProductSelect({ type, historical = false, ...props }: Selection<ProductRecord> & { type?: "GOODS" | "SERVICE" }) {
  const loadPage = useCallback((keyword: string, page: number, pageSize: number) => productPage({
    keyword, page, pageSize, ...(type ? { type } : {}), ...(historical ? {} : { status: 1 }),
  }), [type, historical]);
  return <PagedRecordSelect {...props} loadPage={loadPage} loadRecord={getProduct}
    describe={p => [p.code, p.name, p.brand, p.model, p.specification, p.type === "SERVICE" ? "服务" : "实物", `单位：${p.unit}`, p.status === 0 ? "已停用" : ""].filter(Boolean).join(" · ")}
    warning={p => historical ? "" : p.status !== 1 ? "已停用：保留草稿原选择，请更换启用商品后保存。" : type && p.type !== type ? "商品类型已变化，请重新选择。" : ""} />;
}

export function PartnerSelect({ identity, historical = false, ...props }: Selection<PartnerRecord> & { identity?: "CUSTOMER" | "SUPPLIER" }) {
  const loadPage = useCallback((keyword: string, page: number, pageSize: number) => partnerPage({
    keyword, page, pageSize, ...(identity ? { identity } : {}), ...(historical ? {} : { status: 1 }),
  }), [identity, historical]);
  return <PagedRecordSelect {...props} loadPage={loadPage} loadRecord={getPartner}
    describe={p => [p.code, p.name, p.contact, p.phone, p.status === 0 ? "已停用" : ""].filter(Boolean).join(" · ")}
    warning={p => historical ? "" : p.status !== 1 ? "已停用：保留草稿原选择，请更换启用往来单位后保存。" : (identity === "SUPPLIER" && !p.isSupplier) || (identity === "CUSTOMER" && !p.isCustomer) ? "往来身份已变化，请重新选择。" : ""} />;
}

export function PurchaseSourceSelect(props: Selection<PurchaseDraft>) {
  const loadPage = useCallback((keyword: string, page: number, pageSize: number) => purchasePage({ documentNo: keyword.trim(), page, pageSize }), []);
  return <PagedRecordSelect {...props} loadPage={loadPage} loadRecord={getPurchase}
    describe={d => `${d.documentNo} · ${d.partnerName} · ${d.businessDate} · ${d.directDelivery ? "直送" : "普通采购"} · ${{DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[d.status]}`}
    warning={d => !d.directDelivery ? "普通采购不能关联直送销售" : d.status === "CANCELLED" ? "已取消采购不能关联" : ""} />;
}
