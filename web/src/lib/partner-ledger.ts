import type { PartnerBalanceEntry } from "@/api/business";
export const entryTypeLabel: Record<string,string>={OPENING:"期初",RECEIPT:"客户收款",PAYMENT:"供应商付款",CUSTOMER_REFUND:"客户退款",SUPPLIER_REFUND:"供应商退款",PURCHASE:"采购入库",SALE:"销售出库",PURCHASE_RETURN:"采购退货",SALE_RETURN:"销售退货",REVERSAL:"取消 / 往来冲销"};
export function ledgerSource(entry:PartnerBalanceEntry){
 if(entry.saleReturnId)return `/business/sale-returns?returnId=${entry.saleReturnId}`;
 if(entry.purchaseReturnId)return `/business/purchase-returns?returnId=${entry.purchaseReturnId}`;
 if(entry.saleId)return `/business/sales?saleId=${entry.saleId}`;
 if(entry.purchaseId)return `/business/purchases?purchaseId=${entry.purchaseId}`;
 return `/business/partner-ledger?partnerId=${entry.partnerId}&direction=${entry.direction}&entryId=${entry.reversesId??entry.id}`;
}
export function localPeriod(from:string,to:string){
 if(!from||!to)throw new Error("请选择完整期间");
 const start=new Date(`${from}T00:00:00`),end=new Date(`${to}T00:00:00`);end.setDate(end.getDate()+1);
 if(!Number.isFinite(start.getTime())||!Number.isFinite(end.getTime())||start>=end)throw new Error("期间开始不能晚于结束");
 return {from:start.toISOString(),to:end.toISOString()};
}
export function localToday(){const now=new Date();return `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,"0")}-${String(now.getDate()).padStart(2,"0")}`}
