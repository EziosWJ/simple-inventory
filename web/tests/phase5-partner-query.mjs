// Isolated API only. Public HTTP setup, real query/source navigation and A4 PDF.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const api=process.env.PHASE5_API_URL??'http://127.0.0.1:18109',base='http://127.0.0.1:4195',prefix=`L5-${Date.now()}`;
const vite=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4195','--strictPort'],{cwd:new URL('..',import.meta.url),env:{...process.env,VITE_API_BASE_URL:api},stdio:'ignore'});
let browser,token;
async function call(path,body,method=body===undefined?'GET':'POST'){const r=await fetch(`${api}/api${path}`,{method,headers:{Authorization:token??'','Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});const j=await r.json();assert.equal(r.status,200,`${path}: ${j.message}`);return j.data;}
async function select(page,label,code){const g=page.getByRole('group',{name:label,exact:true});await g.getByRole('button').first().click();await g.getByRole('textbox').fill(code);await g.locator('button[aria-pressed]').filter({hasText:code}).last().click();}
try{
 for(let n=0;n<60;n++){try{if((await fetch(base)).ok)break;}catch{}await delay(250);}
 token=(await call('/auth/login',{username:'admin',password:process.env.PHASE5_UI_PASSWORD??'admin123'})).tokenValue;
 const partner=await call('/v1/partners',{code:prefix,name:'双向查账对象',type:'COMPANY',isSupplier:true,isCustomer:true,phone:'021-777777'});
 const goods=await call('/v1/products',{code:prefix,name:'查账实物',type:'GOODS',unit:'台'}), service=await call('/v1/products',{code:`${prefix}-S`,name:'查账服务',type:'SERVICE',unit:'次'});
 const openings=[];
 for(let n=0;n<25;n++)openings.push(await call('/v1/partner-balances/opening',{requestKey:`${prefix}-O-${n}`,partnerId:partner.id,direction:'CUSTOMER',amount:'1.00',businessDate:'2026-09-01',description:'补录旧业务期初'}));
 await call('/v1/partner-balances/opening',{requestKey:`${prefix}-OS`,partnerId:partner.id,direction:'SUPPLIER',amount:'2.00',businessDate:'2026-09-01',description:'供应商期初'});
 const item=(p,price)=>({productId:p.id,productType:p.type,unit:p.unit,quantity:'1',unitPrice:price});
 let purchase=await call('/v1/purchases',{partnerId:partner.id,businessDate:'2026-09-15',items:[item(goods,'10.00')]});purchase=await call(`/v1/purchases/${purchase.id}/post`,{version:1});
 let sale=await call('/v1/sales',{partnerId:partner.id,businessDate:'2026-09-15',items:[item(goods,'20.00')]});sale=await call(`/v1/sales/${sale.id}/post`,{version:1});
 await call(`/v1/partner-balances/entries/${openings[0].id}/reverse`,{reason:'期初重复补录'});
 const funds=(direction,amount,key,type='settlements')=>call(`/v1/partner-balances/${type}`,{requestKey:`${prefix}-${key}`,partnerId:partner.id,direction,amount,businessDate:'2026-09-15',paymentMethod:'CASH',transactionNo:'',remark:'查账验收'});
 await funds('CUSTOMER','44.00','R');await funds('SUPPLIER','12.00','P');
 let sr=await call('/v1/sale-returns',{saleId:sale.id,businessDate:'2026-09-16',items:[{saleItemId:sale.items[0].id,quantity:'1'}]});await call(`/v1/sale-returns/${sr.id}/post`,{version:1});
 let pr=await call('/v1/purchase-returns',{purchaseId:purchase.id,businessDate:'2026-09-16',items:[{purchaseItemId:purchase.items[0].id,quantity:'1'}]});await call(`/v1/purchase-returns/${pr.id}/post`,{version:1});
 await funds('CUSTOMER','5.00','RF-C','refunds');await funds('SUPPLIER','5.00','RF-S','refunds');
 await call(`/v1/partners/${partner.id}`,{name:partner.name,type:'COMPANY',isSupplier:true,isCustomer:false,phone:'021-777777'},'PUT');await call(`/v1/partners/${partner.id}/status`,{status:0},'PUT');
 browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH}:{})});
 const context=await browser.newContext({timezoneId:'Asia/Shanghai',viewport:{width:1500,height:1000}});await context.addInitScript(t=>localStorage.setItem('web-auth',JSON.stringify({token:t,user:null})),token);
 const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(`${base}/business/partner-balances`);await select(page,'筛选往来单位',partner.code);
 await page.getByRole('row').filter({hasText:'店铺待退客户'}).waitFor();await page.getByRole('row').filter({hasText:'供应商待退店铺'}).waitFor();
 const rows=page.getByRole('row');assert.equal(await rows.count(),3);assert.match(await rows.nth(1).innerText(),/-15.00/);assert.match(await rows.nth(2).innerText(),/-5.00/);
 const balanceURL=page.url();await rows.nth(1).getByRole('button',{name:'查看明细'}).click();
 const today=new Date().toLocaleDateString('en-CA',{timeZone:'Asia/Shanghai'});
 await page.getByLabel('生效开始日').fill(today);await page.getByLabel('生效结束日（含当日）').fill(today);await page.getByRole('button',{name:'查询',exact:true}).click();
 await page.getByRole('region',{name:'期间余额'}).waitFor();
 const ledgerURL=page.url(),query=new URL(ledgerURL).searchParams;
 assert.equal(query.get('partnerId'),String(partner.id));assert.equal(query.get('direction'),'CUSTOMER');assert.ok(query.get('from')&&query.get('to'));
 await page.reload();assert.equal(await page.getByLabel('生效开始日').inputValue(),today);
 await page.getByRole('button',{name:'下一页'}).click();assert.equal(new URL(page.url()).searchParams.get('page'),'2');await page.reload();
 await page.getByRole('row').filter({hasText:openings[24].documentNo}).waitFor();
 await page.getByRole('link',{name:'返回来源页面'}).last().click();assert.equal(page.url(),balanceURL);
 // Source kinds are opened from the actual ledger detail, with period preserved on return.
 for(const direction of ['CUSTOMER','SUPPLIER']){
  const entries=(await call(`/v1/partner-balances/entries?partnerId=${partner.id}&direction=${direction}&pageSize=100`)).records;
  const types=new Set();
  for(const entry of entries){
   if(types.has(entry.entryType))continue;types.add(entry.entryType);
   const target=new URL(ledgerURL);target.searchParams.set('direction',direction);target.searchParams.set('entryId',entry.id);
   await page.goto(target.href);const detail=page.getByRole('dialog');await detail.getByRole('button',{name:'打开来源业务'}).click();
   const expected=entry.entryType==='REVERSAL'?'/business/opening-balances':entry.entryType==='OPENING'?'/business/opening-balances':entry.entryType==='SALE'?'/business/sales':entry.entryType==='PURCHASE'?'/business/purchases':entry.entryType==='SALE_RETURN'?'/business/sale-returns':entry.entryType==='PURCHASE_RETURN'?'/business/purchase-returns':entry.entryType.endsWith('_REFUND')?'/business/refunds':'/business/settlements';
   await page.waitForURL(url=>url.pathname===expected);
   await page.getByRole('dialog').getByRole('link',{name:'返回来源页面'}).click();
   const back=new URL(page.url()).searchParams;assert.equal(back.get('partnerId'),query.get('partnerId'));assert.equal(back.get('direction'),direction);assert.equal(back.get('from'),query.get('from'));assert.equal(back.get('to'),query.get('to'));
  }
  assert.ok(types.has('OPENING')&&types.has(direction==='CUSTOMER'?'SALE':'PURCHASE')&&types.has(direction==='CUSTOMER'?'CUSTOMER_REFUND':'SUPPLIER_REFUND'));
 }
 await page.goto(ledgerURL);await page.getByRole('button',{name:'打印期间对账单'}).click();await page.locator('.statement-page').nth(2).waitFor();
 assert.equal(await page.locator('.statement-table tbody tr').count(),30);
 await page.pdf({path:'/tmp/phase5-partner-statement.pdf',format:'A4',preferCSSPageSize:true,printBackground:true});
 await page.getByRole('link',{name:'返回来源页面'}).last().click();assert.equal(new URL(page.url()).searchParams.get('from'),query.get('from'));
 assert.deepEqual(errors,[]);console.log('PASS real browser: shared two-direction negative balances, historical identity/inactive selection, effective period/page/refresh, all original/reversal sources and return context, complete A4 statement PDF');
}finally{if(browser)await browser.close();vite.kill();}
