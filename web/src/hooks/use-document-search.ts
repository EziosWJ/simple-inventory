import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
export type DocumentFilters = { documentNo:string; partnerId:string; productId:string; partnerKeyword:string; productKeyword:string; status:string; businessFrom:string; businessTo:string };
export const blankDocumentFilters: DocumentFilters = {documentNo:"",partnerId:"",productId:"",partnerKeyword:"",productKeyword:"",status:"",businessFrom:"",businessTo:""};
export function useDocumentSearch() {
  const [params,setParams] = useSearchParams();
  const query = useMemo(() => Object.fromEntries(Object.keys(blankDocumentFilters).map(key=>[key,params.get(key)??""])) as DocumentFilters,[params]);
  const [filters,setFilters] = useState(query);
  useEffect(()=>setFilters(query),[query]);
  const rawPage = Number(params.get("page"));
  const page = Number.isSafeInteger(rawPage)&&rawPage>0 ? rawPage : 1;
  function setPage(next:number){const p=new URLSearchParams(params);p.set("page",String(next));setParams(p);}
  function apply(next:DocumentFilters){next={...next};if(next.partnerKeyword.trim())next.partnerId="";if(next.productKeyword.trim())next.productId="";const p=new URLSearchParams(params);for(const [key,value] of Object.entries(next)){if(value.trim())p.set(key,value.trim());else p.delete(key);}p.delete("page");setParams(p);}
  return {params,setParams,query,filters,setFilters,page,setPage,apply};
}
