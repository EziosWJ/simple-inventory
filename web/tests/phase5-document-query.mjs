// Real public API fixtures and Chromium; use only an isolated database.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const api=process.env.PHASE5_API_URL??'http://127.0.0.1:18109',base='http://127.0.0.1:4194',prefix=`Q5-${Date.now()}`;
const vite=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4194','--strictPort'],{cwd:new URL('..',import.meta.url),env:{...process.env,VITE_API_BASE_URL:api},stdio:'ignore'});
let browser,token;
async function call(path,body,method=body===undefined?'GET':'POST'){const r=await fetch(`${api}/api${path}`,{method,headers:{Authorization:token??'','Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});const j=await r.json();assert.equal(r.status,200,`${path}: ${j.message}`);return j.data;}
async function select(page,label,code){const g=page.getByRole('group',{name:label,exact:true});await g.getByRole('button').first().click();await g.getByRole('textbox').fill(code);await g.locator('button[aria-pressed]').filter({hasText:code}).last().click();}
try{
 for(let n=0;n<60;n++){try{if((await fetch(base)).ok)break;}catch{}await delay(250);}
 token=(await call('/auth/login',{username:'admin',password:process.env.PHASE5_UI_PASSWORD??'admin123'})).tokenValue;
 const partner=await call('/v1/partners',{code:prefix,name:'同名历史对象',type:'COMPANY',isSupplier:true,isCustomer:true});
 const other=await call('/v1/partners',{code:`${prefix}-OTHER`,name:'同名历史对象',type:'COMPANY',isSupplier:true,isCustomer:true});
 const product=await call('/v1/products',{code:prefix,name:'历史型号商品',type:'GOODS',unit:'台'});
 const fixtures={};
 for(const kind of ['purchases','sales']){
  const documents=[];
  for(let n=0;n<14;n++)documents.push(await call(`/v1/${kind}`,{partnerId:n===13?other.id:partner.id,businessDate:'2026-09-15',items:[1,2].map(price=>({productId:product.id,productType:'GOODS',unit:'台',quantity:'1',unitPrice:String(price)}))}));
  documents[0]=await call(`/v1/${kind}/${documents[0].id}/post`,{version:1});
  documents[1]=await call(`/v1/${kind}/${documents[1].id}/cancel`,{version:1,reason:'历史取消'});
  fixtures[kind]=documents;
 }
 await call(`/v1/products/${product.id}`,{name:'后来商品名',type:'GOODS',unit:'台',model:'后来型号'},'PUT');await call(`/v1/products/${product.id}/status`,{status:0},'PUT');
 await call(`/v1/partners/${partner.id}/status`,{status:0},'PUT');
 browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH}:{})});
 const context=await browser.newContext({viewport:{width:1500,height:1050}});await context.addInitScript(t=>localStorage.setItem('web-auth',JSON.stringify({token:t,user:null})),token);
 const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));page.on('dialog',d=>d.accept());
 for(const kind of ['purchases','sales']){
  const purchase=kind==='purchases';
  await call(`/v1/partners/${partner.id}`,{name:'同名历史对象',type:'COMPANY',isCustomer:purchase,isSupplier:!purchase},'PUT');
  await page.goto(`${base}/business/${kind}`);
  await select(page,purchase?'筛选供应商':'客户筛选',partner.code);
  await select(page,purchase?'筛选商品':'商品或服务筛选',product.code);
  await page.locator('input[type=date]').first().fill('2026-09-15');await page.locator('input[type=date]').last().fill('2026-09-15');
  await page.getByRole('button',{name:'查询',exact:true}).click();
  await page.getByRole('row').filter({hasText:fixtures[kind][12].documentNo}).waitFor();
  assert.equal(await page.getByRole('row').count(),11);
  await page.getByRole('button',{name:'下一页'}).click();
  await page.getByRole('row').filter({hasText:fixtures[kind][0].documentNo}).waitFor();
  assert.equal(await page.getByRole('row').count(),4);
  const contextURL=page.url();assert.equal(new URL(contextURL).searchParams.get('page'),'2');
  await page.reload();await page.getByRole('row').filter({hasText:fixtures[kind][0].documentNo}).waitFor();
  const draftRow=page.getByRole('row').filter({hasText:fixtures[kind][2].documentNo});
  await draftRow.getByRole('button',{name:purchase?'查看':'详情',exact:true}).click();
  const dialog=page.getByRole('dialog');await dialog.getByText(fixtures[kind][2].documentNo,{exact:false}).first().waitFor();
  await dialog.getByRole('button',{name:purchase?'编辑草稿':'编辑草稿',exact:true}).click();
  await page.waitForURL(/\/business\/(purchases|sales)\/\d+\/edit(?:\?.*)?$/);
  await page.getByRole('button',{name:purchase?'返回采购列表':'返回销售列表'}).click();
  assert.equal(new URL(page.url()).searchParams.get('page'),'2');
  await page.getByRole('dialog').getByRole('button',{name:purchase?'关闭详情弹窗':'关闭',exact:true}).click();
  // A slow page-two result cannot replace the newer cancelled-only filter.
  let delayed=false;
  await page.route(`**/api/v1/${kind}?*`,async route=>{const u=new URL(route.request().url());if(u.searchParams.get('page')==='2'&&!u.searchParams.get('status')){const response=await route.fetch();delayed=true;await delay(1800);try{await route.fulfill({response});}catch{}}else await route.continue();});
  await page.getByRole('button',{name:'刷新',exact:true}).click();
  for(let n=0;n<50&&!delayed;n++)await delay(50);assert.ok(delayed);
  await page.locator('select').first().selectOption('CANCELLED');await page.getByRole('button',{name:'查询',exact:true}).click();
  await page.getByRole('row').filter({hasText:fixtures[kind][1].documentNo}).waitFor();await delay(2000);
  assert.equal(await page.getByRole('row').count(),2);assert.equal(new URL(page.url()).searchParams.get('page'),null);
  await page.unroute(`**/api/v1/${kind}?*`);
  await page.getByRole('button',{name:'重置',exact:true}).click();
  assert.equal(new URL(page.url()).searchParams.get('partnerId'),null);assert.equal(new URL(page.url()).searchParams.get('businessFrom'),null);
 }
 assert.deepEqual(errors,[]);console.log('PASS real browser: historical inactive/removed identity, split-line count, combined dates, page/reset/refresh, detail/edit return context and late response ownership');
}finally{if(browser)await browser.close();vite.kill();}
