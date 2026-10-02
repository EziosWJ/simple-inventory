// Isolated Compose deployment only. Private fixture contains test credentials.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import crypto from 'node:crypto';
import { chromium } from 'playwright';
const fixturePath=process.env.PHASE5_PRODUCTION_FIXTURE;
if(!fixturePath)throw Error('Set PHASE5_PRODUCTION_FIXTURE to an isolated mode-600 fixture');
assert.equal(fs.statSync(fixturePath).mode & 0o077,0);
const fixture=JSON.parse(fs.readFileSync(fixturePath,'utf8'));
assert.equal(fixture.isolated,true);
const base=process.env.PHASE5_PRODUCTION_URL??'https://localhost:18443',mode=process.env.PHASE5_PRODUCTION_MODE??'seed';
const legacy=process.env.PHASE5_PRODUCTION_LEGACY==='1',entry=legacy?'/business/purchases':'/business/purchases/new';
const browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH}:{})});
async function select(page,label,code){const g=page.getByRole('group',{name:label,exact:true});await g.getByRole('button').first().click();await g.getByRole('textbox').fill(code);await g.locator('button[aria-pressed]').filter({hasText:code}).last().click();}
try{
 const contexts=[],pages=[],tokens=[];
 for(const operator of fixture.operators){
  const context=await browser.newContext({ignoreHTTPSErrors:true,viewport:{width:1440,height:1000},timezoneId:'Asia/Shanghai'}),page=await context.newPage();page.on('dialog',d=>d.accept());
  await page.goto(`${base}${entry}`);await page.waitForURL('**/login');await page.getByLabel('用户名',{exact:true}).fill(operator.username);await page.getByLabel('密码',{exact:true}).fill(operator.password);await page.locator('button[type=submit]').click();await page.waitForURL(`**${entry}`);const heading=page.getByRole('heading',{name:legacy?'采购入库':'新建采购单',exact:true});await heading.waitFor();await page.reload();await heading.waitFor();
  const login=await context.request.post(`${base}/api/auth/login`,{data:{username:operator.username,password:operator.password}});tokens.push((await login.json()).data.tokenValue);contexts.push(context);pages.push(page);
 }
 const call=async(path,body,method=body===undefined?'GET':'POST',actor=0)=>{const response=await contexts[actor].request.fetch(`${base}/api${path}`,{method,headers:{Authorization:tokens[actor]},data:body});const value=await response.json();assert.equal(response.status(),200,`${path}: ${value.message}`);assert.equal(value.code,200);return value.data;};
 assert.equal((await contexts[0].request.get(`${base}/ready`)).status(),200);
 assert.equal((await contexts[0].request.get(`${base}/swagger/index.html`)).status(),404);
 if(mode==='seed'){
  const prefix=`DEPLOY-${Date.now()}`;
  fixture.partner=await call('/v1/partners',{code:prefix,name:'部署恢复往来',type:'COMPANY',isSupplier:true,isCustomer:true});fixture.product=await call('/v1/products',{code:prefix,name:'部署恢复商品',model:'测试型号',type:'GOODS',unit:'台',purchasePrice:'2.00',salePrice:'3.00'});
  const [first,second]=pages;
  await select(first,'供应商',prefix);await select(first,'第 1 行商品',prefix);await first.getByLabel('数量').fill('2');await first.getByRole('button',{name:'保存草稿',exact:true}).click();await first.waitForURL(/\/purchases\/\d+\/edit(?:\?.*)?$/);
  fixture.purchaseId=Number(first.url().match(/purchases\/(\d+)/)[1]);
  await second.goto(`${base}/business/purchases/${fixture.purchaseId}/edit`);await second.getByRole('button',{name:'保存并过账',exact:true}).click();await second.getByRole('dialog').getByRole('button',{name:'确认保存并过账'}).click();await second.getByText(/库存与应付已生效/).waitFor();
  await first.goto(`${base}/dashboard`);await first.getByRole('link',{name:'新建销售单',exact:true}).click();await select(first,'客户',prefix);await select(first,'第 1 行商品',prefix);await first.getByRole('button',{name:'保存并过账',exact:true}).click();await first.getByRole('dialog').getByRole('button',{name:'确认保存并过账'}).click();await first.getByText(/库存与应收已生效/).waitFor();fixture.saleId=Number(first.url().match(/sales\/(\d+)/)[1]);
  const bytes=Buffer.from('Phase 5 persistent file / 采购销售恢复验证\n');fixture.fileHash=crypto.createHash('sha256').update(bytes).digest('hex');
  const response=await contexts[0].request.post(`${base}/api/system/file/upload`,{headers:{Authorization:tokens[0]},multipart:{file:{name:'phase5-recovery.txt',mimeType:'text/plain',buffer:bytes},businessModule:'phase5-acceptance'}});assert.equal(response.status(),200);fixture.file=(await response.json()).data;
  fixture.purchase=await call(`/v1/purchases/${fixture.purchaseId}`);fixture.sale=await call(`/v1/sales/${fixture.saleId}`);fs.writeFileSync(fixturePath,JSON.stringify(fixture,null,2),{mode:0o600});
 }
 assert.deepEqual(await call(`/v1/purchases/${fixture.purchaseId}`,undefined,'GET',1),fixture.purchase);
 assert.deepEqual(await call(`/v1/sales/${fixture.saleId}`,undefined,'GET',1),fixture.sale);
 const download=await contexts[1].request.get(`${base}/api/system/file/${fixture.file.id}/download`,{headers:{Authorization:tokens[1]}});assert.equal(download.status(),200);assert.equal(crypto.createHash('sha256').update(await download.body()).digest('hex'),fixture.fileHash);
 const stock=(await call(`/v1/inventory/balances?productId=${fixture.product.id}&stock=all`)).records.find(r=>r.productId===fixture.product.id);assert.equal(stock.quantity,mode==='after-write'?'0':'1');
 const balances=(await call(`/v1/partner-balances?partnerId=${fixture.partner.id}`)).records;assert.equal(balances.find(r=>r.direction==='CUSTOMER').amount,mode==='after-write'?'6.00':'3.00');assert.equal(balances.find(r=>r.direction==='SUPPLIER').amount,'4.00');
 await pages[1].goto(`${base}/business/sales?saleId=${fixture.saleId}`);await pages[1].getByRole('dialog').getByText(fixture.sale.documentNo,{exact:false}).first().waitFor();await pages[1].reload();await pages[1].getByRole('dialog').getByText(fixture.sale.documentNo,{exact:false}).first().waitFor();
 await pages[0].goto(`${base}/business/sales/${fixture.saleId}/delivery-note`);await pages[0].locator('.delivery-page').waitFor();await pages[0].pdf({path:process.env.PHASE5_PRODUCTION_PDF??'/tmp/phase5-production-delivery.pdf',format:'A4',printBackground:true,preferCSSPageSize:true});
 if(mode==='write'){
  const draft=await call('/v1/sales',{partnerId:fixture.partner.id,businessDate:'2026-10-02',items:[{productId:fixture.product.id,productType:'GOODS',unit:'台',quantity:'1',unitPrice:'3.00'}]});const posted=await call(`/v1/sales/${draft.id}/post`,{version:draft.version});assert.equal(posted.status,'POSTED');assert.equal((await call('/v1/inventory/balances?stock=all')).records.find(r=>r.productId===fixture.product.id).quantity,'0');assert.equal((await call(`/v1/partner-balances?partnerId=${fixture.partner.id}&direction=CUSTOMER`)).records[0].amount,'6.00');console.log('PASS restored deployment: representative sale write updates stock and receivable');
 }
 console.log(`PASS production HTTPS ${mode}: embedded frontend/deep refresh, actual independent logins/shared business, readiness/Swagger off, exact documents/snapshots/balances, persistent file checksum and real PDF`);
}finally{await browser.close();}
