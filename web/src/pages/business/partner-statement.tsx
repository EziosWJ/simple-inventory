import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { getPartnerStatement, type PartnerStatement, type PartnerBalanceEntry } from "@/api/business";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { entryTypeLabel } from "@/lib/partner-ledger";
import "./delivery-note.css";
import "./partner-statement.css";
const rowsPerPage=12;
function pages(records:PartnerBalanceEntry[]){if(!records.length)return [[]];const result:PartnerBalanceEntry[][]=[];for(let i=0;i<records.length;i+=rowsPerPage)result.push(records.slice(i,i+rowsPerPage));return result}
export function PartnerStatementPage(){
 const [params]=useSearchParams();const partnerId=params.get("partnerId")??"",direction=params.get("direction")??"",from=params.get("from")??"",to=params.get("to")??"";
 const [statement,setStatement]=useState<PartnerStatement|null>(null),[error,setError]=useState(""),[loading,setLoading]=useState(false);
 const load=useCallback(()=>{let active=true;setLoading(true);setStatement(null);void getPartnerStatement({partnerId,direction,from,to}).then(value=>{if(active){setStatement(value);setError("")}}).catch(e=>{if(active)setError(e instanceof Error?e.message:"读取对账单失败")}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[partnerId,direction,from,to]);
 useEffect(load,[load]);const sheets=statement?pages(statement.records):[];
 return <div className="space-y-space-4"><div className="print-hide"><PageHeader title="往来对账单 / A4打印" description="完整期间数据来自服务器。生效时间含起点、不含终点；期初为起点前累计，后续补录及冲销计入实际发生期间。" actions={<Button disabled={!statement||loading} onClick={()=>window.print()}>打印 A4</Button>}/>{error&&<p role="alert" className="text-error">{error}</p>}{loading&&<p role="status">正在读取完整期间…</p>}{statement&&<p className="text-sm text-text-secondary">完整期间共 {statement.records.length} 笔 / {sheets.length} 页。正余额：欠款；负余额：待退款。</p>}</div>
 {statement&&<div className="delivery-note-print partner-statement-print">{sheets.map((records,index)=><section className="delivery-page statement-page" key={index} aria-label={`往来对账单第${index+1}页`}><header><h1 className="delivery-title">往来对账单</h1><div className="statement-meta"><strong>往来单位：{statement.partnerName}</strong><span>方向：{statement.direction==="CUSTOMER"?"客户（应收）":"供应商（应付）"}</span><span>标识：ST-{statement.partnerId}-{statement.direction}-{statement.from}</span><span>实际生效期间：{new Date(statement.from).toLocaleString()}（含）至 {new Date(statement.to).toLocaleString()}（不含）</span><span>期间期初余额：{statement.openingAmount} 元</span><span>正余额为欠款，负余额为待退款；币种：人民币</span></div></header>
 <table className="delivery-table statement-table"><colgroup><col style={{width:"8%"}}/><col style={{width:"14%"}}/><col style={{width:"18%"}}/><col style={{width:"30%"}}/><col style={{width:"15%"}}/><col style={{width:"15%"}}/></colgroup><thead><tr><th>序号</th><th>业务日期</th><th>实际生效</th><th>类型 / 来源单号</th><th>变动金额</th><th>变动后余额</th></tr></thead><tbody>{records.map((r,i)=><tr key={r.id}><td>{index*rowsPerPage+i+1}</td><td>{r.businessDate.slice(0,10)}</td><td>{new Date(r.effectiveAt).toLocaleString()}</td><td><span>{entryTypeLabel[r.entryType]??r.entryType}</span><br/><span>{r.documentNo}</span>{r.reversedDocumentNo&&<><br/><span>原：{r.reversedDocumentNo}</span></>}</td><td className="tabular-nums">{r.amount}</td><td className="tabular-nums">{r.balanceAfter}</td></tr>)}{!statement.records.length&&<tr><td colSpan={6}>期间无金额变动</td></tr>}</tbody></table>
 {index===sheets.length-1&&<div className="delivery-tail statement-totals"><p>期间增加：{statement.increaseAmount} 元　期间减少：{statement.decreaseAmount} 元</p><p>期间净变动：{statement.netChange} 元</p><p>期末余额：{statement.closingAmount} 元</p><p className="statement-check">对账确认：________________　日期：________________</p></div>}
 <footer className="delivery-foot">ST-{statement.partnerId}-{statement.direction} · 第 {index+1} / {sheets.length} 页</footer></section>)}</div>}</div>;
}
