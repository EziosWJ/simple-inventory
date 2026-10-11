// Run only against an isolated API: this creates business data through public HTTP.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const api = process.env.PARTNER_UX_API_URL ?? 'http://127.0.0.1:18164';
const base = 'http://127.0.0.1:4164', prefix = `UX-${Date.now()}`;
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4164', '--strictPort'], { cwd: new URL('..', import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: 'ignore' });
let browser, token;
async function call(path, body, method = body === undefined ? 'GET' : 'POST') {
  const r = await fetch(`${api}/api${path}`, { method, headers: { Authorization: token ?? '', 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
  const j = await r.json(); assert.equal(r.status, 200, `${path}: ${j.message}`); return j.data;
}
async function select(page, label, code) {
  const group = page.getByRole('group', { name: label, exact: true });
  await group.getByRole('button').first().click(); await group.getByRole('textbox').fill(code);
  await group.locator('button[aria-pressed]').filter({ hasText: code }).last().click();
}
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch {} await delay(100); }
  token = (await call('/auth/login', { username: 'admin', password: process.env.PARTNER_UX_PASSWORD ?? 'admin123' })).tokenValue;
  const baseline = await call('/v1/partner-balances/overview');
  const partners = [];
  async function partner(name) { return call('/v1/partners', { code: `${prefix}-${name}`, name: `${prefix}-${name}`, type: 'COMPANY', isCustomer: true, isSupplier: true }); }
  async function opening(p, direction, amount) { await call('/v1/partner-balances/opening', { requestKey: `${prefix}-${p.id}-${direction}`, partnerId: p.id, direction, amount, businessDate: '2026-10-01', description: '往来体验验收' }); }
  for (let i = 1; i <= 25; i++) { const p = await partner(`客户${String(i).padStart(2,'0')}`); partners.push(p); await opening(p, 'CUSTOMER', i === 25 ? '1000.00' : `${i}.00`); }
  const dual = await partner('双向单位'); await opening(dual,'CUSTOMER','200.00'); await opening(dual,'SUPPLIER','70.00');
  const refundPartner = await partner('退款单位');
  const product = await call('/v1/products',{code:prefix,name:'退款验收实物',type:'GOODS',unit:'台'});
  const line = price => ({productId:product.id,productType:'GOODS',unit:'台',quantity:'1',unitPrice:price});
  const purchase = await call('/v1/purchases',{partnerId:refundPartner.id,businessDate:'2026-10-01',items:[line('50.00')]});
  await call(`/v1/purchases/${purchase.id}/post`,{version:1});
  const sale = await call('/v1/sales',{partnerId:refundPartner.id,businessDate:'2026-10-01',items:[line('80.00')]});
  await call(`/v1/sales/${sale.id}/post`,{version:1});
  const funds = (p,direction,amount,key,kind='settlements') => call(`/v1/partner-balances/${kind}`,{requestKey:`${prefix}-${key}`,partnerId:p.id,direction,amount,businessDate:'2026-10-01',paymentMethod:'CASH',transactionNo:'',remark:'真实UI验收'});
  await funds(refundPartner,'CUSTOMER','80.00','receipt'); await funds(refundPartner,'SUPPLIER','50.00','payment');
  const savedSale = await call(`/v1/sales/${sale.id}`),savedPurchase = await call(`/v1/purchases/${purchase.id}`);
  const sr = await call('/v1/sale-returns',{saleId:sale.id,businessDate:'2026-10-02',items:[{saleItemId:savedSale.items[0].id,quantity:'1'}]});await call(`/v1/sale-returns/${sr.id}/post`,{version:1});
  const pr = await call('/v1/purchase-returns',{purchaseId:purchase.id,businessDate:'2026-10-02',items:[{purchaseItemId:savedPurchase.items[0].id,quantity:'1'}]});await call(`/v1/purchase-returns/${pr.id}/post`,{version:1});
  const noRecord = await partner('无记录');
  browser = await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? {executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH} : {})});
  const context = await browser.newContext({viewport:{width:1440,height:1000},timezoneId:'Asia/Shanghai'});
  await context.addInitScript(t=>localStorage.setItem('web-auth',JSON.stringify({token:t,user:null})),token);
  const page = await context.newPage(), errors=[]; page.on('pageerror',e=>errors.push(e.message));
  page.setDefaultTimeout(10000);
  await page.goto(`${base}/business/partner-balances`);
  await page.getByRole('region',{name:'欠款金额排名'}).getByRole('button').first().waitFor();
  const overview = page.getByRole('region',{name:'全店当前未结余额'});
  const all = await call('/v1/partner-balances/overview');
  for (const summary of all.summaries) {
    const label = {RECEIVABLE:'客户应收',PAYABLE:'供应商应付',CUSTOMER_REFUND:'应退客户款',SUPPLIER_REFUND:'应收供应商退款'}[summary.category];
    assert.match(await overview.getByRole('button',{name:`查看${label}`,exact:true}).innerText(), new RegExp(`${summary.partnerCount} 个单位`));
  }
  const summary = name => all.summaries.find(s=>s.category===name);
  const original = name => baseline.summaries.find(s=>s.category===name);
  assert.equal(Number(summary('RECEIVABLE').amount)-Number(original('RECEIVABLE').amount),1500);
  assert.equal(summary('RECEIVABLE').partnerCount-original('RECEIVABLE').partnerCount,26);
  assert.match(await page.getByRole('row').nth(1).innerText(),/1,000.00/);
  assert.equal(await page.getByRole('region',{name:'欠款金额排名'}).getByRole('button').count(),10);
  await page.getByLabel('排名方向').selectOption('SUPPLIER');
  const chartButton=page.getByRole('region',{name:'欠款金额排名'}).getByRole('button').filter({hasText:dual.name});await chartButton.waitFor();await chartButton.click();
  assert.equal(new URL(page.url()).searchParams.get('direction'),'SUPPLIER');
  await page.getByRole('link',{name:'返回来源页面'}).last().click();await overview.getByRole('button',{name:'查看客户应收',exact:true}).waitFor();
  const globalText=await overview.innerText();
  await select(page,'筛选往来单位',dual.code);
  await page.waitForFunction(()=>document.querySelectorAll('table tbody tr').length===1);await page.getByRole('row').filter({hasText:dual.name}).waitFor();assert.equal(await page.getByRole('row').count(),2);
  assert.equal(await overview.innerText(),globalText);
  const filteredURL=page.url();
  await page.getByRole('row').filter({hasText:dual.name}).getByRole('button',{name:'查看明细'}).click();
  assert.equal(new URL(page.url()).searchParams.get('direction'),'CUSTOMER');
  await page.getByRole('link',{name:'返回来源页面'}).last().click();assert.equal(page.url(),filteredURL);
  // Partial collection and latest-balance validation, then full collection.
  await page.getByRole('row').filter({hasText:dual.name}).getByRole('button',{name:'收款',exact:true}).click();
  const form=page.getByRole('region',{name:'办理收付款'});
  await page.getByRole('button',{name:'填入全额'}).click();assert.equal(await page.getByLabel('资金金额',{exact:true}).inputValue(),'200.00');
  await page.getByLabel('资金金额',{exact:true}).fill('200.001');await page.getByText('请输入大于零的金额，最多两位小数。',{exact:true}).waitFor();
  await page.getByLabel('资金金额',{exact:true}).fill('201');await page.getByText(/金额超过当前可办理金额 200.00/).waitFor();
  await page.getByLabel('资金金额',{exact:true}).fill('50.00');
  await funds(dual,'CUSTOMER','180.00','concurrent');
  await form.getByRole('button',{name:'确认客户收款',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'确认客户收款',exact:true}).click();
  await form.getByText(/收付款金额超过当前欠款/).waitFor();assert.equal(await page.getByLabel('资金金额',{exact:true}).inputValue(),'50.00');
  await page.getByRole('button',{name:'填入全额'}).click();assert.equal(await page.getByLabel('资金金额',{exact:true}).inputValue(),'20.00');
  await page.getByLabel('资金金额',{exact:true}).fill('5.00');await form.getByRole('button',{name:'确认客户收款',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'确认客户收款',exact:true}).click();
  const result=page.getByRole('region',{name:'款项办理结果'});await result.getByText(/15.00 元/).waitFor();
  await result.getByRole('button',{name:'继续登记'}).click();await page.getByRole('button',{name:'填入全额'}).click();
  await form.getByRole('button',{name:'确认客户收款',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'确认客户收款',exact:true}).click();
  await result.getByText(/0.00 元/).waitFor();await result.getByRole('button',{name:'返回往来余额'}).click();assert.equal(page.url(),filteredURL);
  await page.getByText('当前筛选下没有往来款项',{exact:true}).waitFor();
  // Supplier task and both refund directions use positive display amounts.
  for (const [label,category,p,button,amount] of [['供应商应付','PAYABLE',dual,'付款','70.00'],['应退客户款','CUSTOMER_REFUND',refundPartner,'向客户退款','80.00'],['应收供应商退款','SUPPLIER_REFUND',refundPartner,'收供应商退款','50.00']]) {
    await page.getByRole('button',{name:'清除筛选',exact:true}).click();await overview.getByRole('button',{name:`查看${label}`,exact:true}).click();
    await select(page,'筛选往来单位',p.code);const row=page.getByRole('row').filter({hasText:p.name});await row.waitFor();assert.match(await row.innerText(),new RegExp(amount));assert.ok(!(await row.innerText()).includes(`-${amount}`));
    await row.getByRole('button',{name:button,exact:true}).click();await page.getByRole('button',{name:'填入全额'}).click();assert.equal(await page.getByLabel('资金金额',{exact:true}).inputValue(),amount);
    const operation=category==='PAYABLE'?'供应商付款':category==='CUSTOMER_REFUND'?'向客户退款':'收到供应商退款';
    await page.getByRole('button',{name:`确认${operation}`,exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:`确认${operation}`,exact:true}).click();
    await result.getByText(/0.00 元/).waitFor();await result.getByRole('button',{name:'返回往来余额'}).click();assert.equal(new URL(page.url()).searchParams.get('category'),category);await page.getByText('当前筛选下没有往来款项',{exact:true}).waitFor();
  }
  await page.getByRole('button',{name:'清除筛选',exact:true}).click();await page.getByLabel('筛选款项分类').selectOption('ALL');await select(page,'筛选往来单位',noRecord.code);
  await page.waitForFunction(()=>document.querySelectorAll('table tbody tr').length===2);await page.getByRole('row').filter({hasText:noRecord.name}).first().waitFor();assert.equal(await page.getByRole('row').count(),3);assert.equal(await page.getByText('尚无记录',{exact:true}).count(),2);
  // Loading/failure states use a delayed real response and one explicit failure.
  await page.route('**/api/v1/partner-balances/overview',async route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({code:503,message:'概览暂时不可用',data:null})}));
  await page.getByRole('button',{name:'刷新',exact:true}).click();await page.getByText('概览暂时不可用',{exact:true}).waitFor();assert.equal(await overview.getByRole('button',{name:'查看客户应收',exact:true}).count(),0);
  await page.unroute('**/api/v1/partner-balances/overview');await page.getByRole('button',{name:'重试概览'}).click();await overview.getByRole('button',{name:'查看客户应收',exact:true}).waitFor();
  await page.route('**/api/v1/partner-balances?*',route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({code:503,message:'列表暂时不可用',data:null})}));await page.getByRole('button',{name:'刷新',exact:true}).click();await page.getByText('列表暂时不可用',{exact:true}).waitFor();
  await page.unroute('**/api/v1/partner-balances?*');await page.getByRole('button',{name:'重试列表'}).click();await page.getByRole('row').filter({hasText:noRecord.name}).first().waitFor();
  await page.goto(`${base}/business/partner-balances?category=RECEIVABLE&page=2`);await page.getByRole('row').nth(1).waitFor();const pagedURL=page.url();await page.getByRole('row').nth(1).getByRole('button',{name:'查看明细'}).click();await page.getByRole('link',{name:'返回来源页面'}).last().click();assert.equal(page.url(),pagedURL);
  // Visual review evidence and overflow checks at the main desktop widths.
  await page.goto(`${base}/business/partner-balances?category=RECEIVABLE`);await page.getByRole('row').nth(1).waitFor();
  for (const width of [1440,1024]) {await page.setViewportSize({width,height:1000});await page.screenshot({path:`/tmp/partner-balance-overview-${width}.png`,fullPage:true});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));}
  await page.route('**/api/v1/partner-balances?*',async route=>{await delay(1200);await route.continue();});
  await page.goto(`${base}/business/settlements?partnerId=${partners[0].id}&direction=CUSTOMER`);
  await page.getByText('读取中…',{exact:true}).waitFor();assert.equal(await page.getByText('当前没有正欠款时不能办理收付款。',{exact:true}).count(),0);
  await page.getByRole('button',{name:'填入全额'}).click();await page.getByLabel('资金金额',{exact:true}).fill('');
  await page.unroute('**/api/v1/partner-balances?*');
  await page.route('**/api/v1/partner-balances?*',route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({code:503,message:'余额暂时不可用',data:null})}));
  await page.reload();await page.getByText('余额暂时不可用',{exact:true}).waitFor();assert.ok(await page.getByRole('button',{name:'填入全额'}).isDisabled());
  await page.unroute('**/api/v1/partner-balances?*');await page.getByRole('button',{name:'重试余额'}).click();await page.getByRole('button',{name:'填入全额'}).click();
  assert.deepEqual(errors,[]);
  console.log('PASS real UI: global summaries/top 10, filters/navigation/page retention, partial/full/payment/refunds, balance conflict/input retention, zero removal, no-record, errors/retries and desktop layout');
} finally { if (browser) await browser.close(); vite.kill(); }
