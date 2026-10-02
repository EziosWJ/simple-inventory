import type { PartnerBalanceEntry } from "@/api/business";
export const entryTypeLabel: Record<string,string>={OPENING:"期初",RECEIPT:"客户收款",PAYMENT:"供应商付款",CUSTOMER_REFUND:"客户退款",SUPPLIER_REFUND:"供应商退款",PURCHASE:"采购入库",SALE:"销售出库",PURCHASE_RETURN:"采购退货",SALE_RETURN:"销售退货",REVERSAL:"取消 / 往来冲销"};
export function cents(value:string){if(!/^-?\d+(\.\d{1,2})?$/.test(value))return null;const [whole,fraction=""]=value.split(".");try{return (value.startsWith("-")?-1n:1n)*(BigInt(whole.replace("-",""))*100n+BigInt((fraction+"00").slice(0,2)))}catch{return null}}
export function money(value:bigint){const magnitude=value<0n?-value:value;return `${value<0n?"-":""}${magnitude/100n}.${String(magnitude%100n).padStart(2,"0")}`}
export function balanceState(amount:string){const value=cents(amount);return value===null?"余额异常":value<0n?"待退款":value>0n?"欠款":"净结清"}
export function ledgerSource(entry:PartnerBalanceEntry){
 if(entry.saleReturnId)return `/business/sale-returns?returnId=${entry.saleReturnId}`;
 if(entry.purchaseReturnId)return `/business/purchase-returns?returnId=${entry.purchaseReturnId}`;
 if(entry.saleId)return `/business/sales?saleId=${entry.saleId}`;
 if(entry.purchaseId)return `/business/purchases?purchaseId=${entry.purchaseId}`;
 if(entry.entryType==="OPENING")return `/business/opening-balances?entryId=${entry.id}`;
 if(entry.entryType==="RECEIPT"||entry.entryType==="PAYMENT")return `/business/settlements?entryId=${entry.id}`;
 if(entry.entryType==="CUSTOMER_REFUND"||entry.entryType==="SUPPLIER_REFUND")return `/business/refunds?entryId=${entry.id}`;
 return `/business/partner-ledger?partnerId=${entry.partnerId}&direction=${entry.direction}&entryId=${entry.reversesId??entry.id}`;
}
export function localPeriod(from:string,to:string){
 if(!from||!to)throw new Error("请选择完整期间");
 const start=new Date(`${from}T00:00:00`),end=new Date(`${to}T00:00:00`);end.setDate(end.getDate()+1);
 if(!Number.isFinite(start.getTime())||!Number.isFinite(end.getTime())||start>=end)throw new Error("期间开始不能晚于结束");
 return {from:start.toISOString(),to:end.toISOString()};
}
export function localToday(){const now=new Date();return `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,"0")}-${String(now.getDate()).padStart(2,"0")}`}

export function periodDateInput(value:string, exclusiveEnd=false){
 const d=new Date(value);if(!Number.isFinite(d.getTime()))return "";
 if(exclusiveEnd)d.setTime(d.getTime()-1);
 return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,"0")}-${String(d.getDate()).padStart(2,"0")}`;
}
