// Purchase document dialogs and confirmation recovery, using only intercepted APIs.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4211';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4211', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
const product = { id: 1, code: 'MOCK-G', name: '采购验收商品', type: 'GOODS', unit: '台', purchasePrice: '2.00', model: 'M-1', specification: '标准规格', status: 1 };
const partner = { id: 1, code: 'MOCK-S', name: '采购验收供应商', type: 'COMPANY', isSupplier: true, isCustomer: false, status: 1 };
const line = { id: 1, productId: 1, productCode: product.code, productName: product.name, productType: 'GOODS', unit: '台', quantity: '2', unitPrice: '2.00', amount: '4.00', productModel: product.model, productSpecification: product.specification, remark: '逐行采购备注' };
const makePurchase = (patch = {}) => ({ id: 1, documentNo: 'PI-MOCK-001', partnerId: 1, partnerName: partner.name, businessDate: '2026-10-12', status: 'DRAFT', version: 1, directDelivery: false, directDocuments: [], sourceOrderId: null, createdBy: 1, createdByName: '采购验收员', createTime: '2026-10-12T01:00:00Z', items: [{ ...line }], totalAmount: '4.00', ...patch });
const records = values => ({ records: values, total: values.length, page: 1, pageSize: 10 });
const dicts = { PARTNER_IDENTITY: [['SUPPLIER', '供应商']], BUSINESS_STATUS: [['1', '启用'], ['0', '停用']] };
let detailRecord = makePurchase(), created = null, saveMode = 'ok', postMode = 'ok', createCount = 0, postCount = 0;
const errors = [];
let browser;

async function choose(page, label, code) {
  const group = page.getByRole('group', { name: label, exact: true });
  await group.getByRole('button').first().click();
  const picker = page.getByRole('dialog').last();
  await picker.getByRole('combobox').fill(code);
  await picker.getByRole('row').filter({ hasText: code }).last().click();
}
async function reply(route, data, status = 200) {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify({ code: status, message: status === 200 ? 'OK' : '采购模拟失败', data: status === 200 ? data : null }) });
}
async function visitDetail(page) {
  await page.goto(`${base}/business/purchases?purchaseId=1`);
  const dialog = page.getByRole('dialog');
  try { await dialog.waitFor({ timeout: 8000 }); }
  catch { throw new Error(`采购详情未打开：${page.url()}；${(await page.locator('main').innerText().catch(() => 'no main')).slice(0, 600)}；page errors=${errors.join(' | ')}`); }
  return dialog;
}

try {
  for (let i = 0; i < 100; i++) { try { if ((await fetch(base)).ok) break; } catch { /* Vite starting */ } await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, timezoneId: 'Asia/Shanghai' });
  await context.addInitScript(() => { if (location.origin === 'http://127.0.0.1:4211') localStorage.setItem('web-auth', JSON.stringify({ token: 'purchase-modal-fixture', user: null })); });
  context.on('page', page => page.on('pageerror', error => errors.push(error.message)));
  await context.route('**/api/**', async route => {
    const url = new URL(route.request().url()), path = url.pathname, method = route.request().method();
    if (!path.startsWith('/api/')) return route.continue();
    let data = records([]);
    if (path === '/api/auth/me') data = { id: 1, username: 'purchase-ui', nickname: '采购验收员', roles: [] };
    else if (path === '/api/auth/menus') data = [];
    else if (path.includes('/dict/')) data = (dicts[path.split('/').at(-2)] ?? []).map(([value, label], sortOrder) => ({ value, label, sortOrder }));
    else if (path.endsWith('/unread-count')) data = 0;
    else if (path === '/api/v1/partners' && method === 'GET') data = records([partner]);
    else if (path === '/api/v1/partners/1') data = partner;
    else if (path === '/api/v1/products' && method === 'GET') data = records([product]);
    else if (path === '/api/v1/products/1') data = product;
    else if (path === '/api/v1/purchases' && method === 'GET') data = records([detailRecord]);
    else if (path === '/api/v1/purchases' && method === 'POST') {
      createCount++;
      if (saveMode === 'fail') return reply(route, null, 400);
      const input = route.request().postDataJSON();
      created = makePurchase({ id: 10 + createCount, documentNo: `PI-MOCK-${String(10 + createCount).padStart(3, '0')}`, partnerId: input.partnerId, businessDate: input.businessDate, directDelivery: input.directDelivery ?? false, remark: input.remark, items: input.items.map((item, index) => ({ ...line, ...item, id: index + 1, productCode: product.code, productName: product.name, productModel: product.model, productSpecification: product.specification, amount: `${(Number(item.quantity) * Number(item.unitPrice)).toFixed(2)}` })), totalAmount: input.items.reduce((sum, item) => sum + Number(item.quantity) * Number(item.unitPrice), 0).toFixed(2) });
      return reply(route, created);
    }
    else if (/^\/api\/v1\/purchases\/\d+$/.test(path) && method === 'GET') data = created && Number(path.split('/').at(-1)) === created.id ? created : detailRecord;
    else if (/^\/api\/v1\/purchases\/\d+\/post$/.test(path) && method === 'POST') {
      postCount++;
      const target = created && Number(path.split('/').at(-2)) === created.id ? created : detailRecord;
      if (postMode === 'unknown') return route.abort('failed');
      if (postMode === 'fail') return reply(route, null, 409);
      target.status = 'POSTED'; target.version++;
      return reply(route, target);
    }
    await reply(route, data);
  });
  const page = await context.newPage();

  for (const status of ['DRAFT', 'POSTED', 'CANCELLED']) {
    detailRecord = makePurchase({ status, totalAmount: status === 'CANCELLED' ? '0.00' : '4.00', postedAt: status !== 'DRAFT' ? '2026-10-12T02:00:00Z' : null, postedByName: status !== 'DRAFT' ? '过账人' : undefined, cancelledAt: status === 'CANCELLED' ? '2026-10-12T03:00:00Z' : null, cancelledByName: status === 'CANCELLED' ? '取消人' : undefined, cancelReason: status === 'CANCELLED' ? '采购取消原因' : undefined });
    const detail = await visitDetail(page);
    await detail.getByRole('region', { name: '单据概览' }).getByText(status === 'CANCELLED' ? '¥0.00' : '¥4.00', { exact: true }).waitFor();
    await detail.getByRole('region', { name: '商品明细' }).getByText('采购验收商品', { exact: true }).waitFor();
    await detail.getByText('逐行采购备注', { exact: false }).waitFor();
    assert.equal(await detail.locator('details[open]').count(), 0, `${status}: auxiliary sections start folded`);
    if (status === 'CANCELLED') { await detail.getByText('采购取消原因', { exact: false }).waitFor(); await detail.getByText(/原过账影响已通过冲销抵消/).waitFor(); }
    assert.ok(await detail.locator('footer').count());
    await detail.getByRole('button', { name: '关闭详情弹窗', exact: true }).click();
  }

  detailRecord = makePurchase({ status: 'POSTED', postedAt: '2026-10-12T02:00:00Z', directDelivery: true, directDocuments: [], totalAmount: '0.00', items: [{ ...line, amount: '0.00', unitPrice: '0.00' }] });
  let detail = await visitDetail(page);
  await detail.getByText('旧版直送：采购与关联销售分别过账', { exact: false }).waitFor();
  await detail.locator('summary').filter({ hasText: '关联单据与直送追溯' }).click();
  await detail.getByRole('button', { name: '关联新建直送销售', exact: true }).waitFor();
  assert.match(await detail.innerText(), /零金额，供应商方向往来余额无变化/);
  await detail.getByRole('button', { name: '关闭详情弹窗', exact: true }).click();
  detailRecord = makePurchase({ sourceOrderId: 55, directDelivery: true, totalAmount: '4.00' });
  detail = await visitDetail(page);
  await detail.getByText('新直送方案生成的采购单：请从来源直送单统一确认或取消。', { exact: true }).waitFor();
  await detail.getByRole('button', { name: '查看来源直送单', exact: true }).waitFor();
  assert.equal(await detail.getByRole('button', { name: '关联新建直送销售', exact: true }).count(), 0);
  if (process.env.PURCHASE_DETAIL_SCREENSHOT) await detail.screenshot({ path: process.env.PURCHASE_DETAIL_SCREENSHOT });

  detailRecord = makePurchase({ status: 'POSTED', items: Array.from({ length: 24 }, (_, index) => ({ ...line, id: index + 1, productName: `长采购明细商品 ${index + 1}` })) });
  await page.setViewportSize({ width: 1024, height: 600 });
  detail = await visitDetail(page);
  const body = detail.locator('div.overflow-y-auto');
  assert.ok(await body.evaluate(node => node.scrollHeight > node.clientHeight), 'long detail scrolls in its own body');
  const footerBefore = await detail.locator('footer').boundingBox();
  assert.ok(footerBefore.y >= 0 && footerBefore.y + footerBefore.height <= 600, 'fixed detail footer remains inside a short viewport');
  await body.evaluate(node => { node.scrollTop = node.scrollHeight; });
  assert.equal(Math.round((await detail.locator('footer').boundingBox()).y), Math.round(footerBefore.y));
  await page.setViewportSize({ width: 390, height: 844 });
  await detail.evaluate(() => window.dispatchEvent(new Event('resize')));
  const geometry = await detail.evaluate(node => { const rect = node.getBoundingClientRect(); const table = [...node.querySelectorAll('div.overflow-x-auto')].find(el => el.querySelector('table')); return { within: rect.left >= 0 && rect.right <= innerWidth && rect.top >= 0 && rect.bottom <= innerHeight, tableOverflow: table.scrollWidth - table.clientWidth }; });
  assert.ok(geometry.within, 'narrow detail stays inside viewport');
  assert.ok(geometry.tableOverflow > 0, 'only the item table scrolls horizontally');
  await detail.getByRole('button', { name: '关闭详情弹窗', exact: true }).click();

  // List posting can be abandoned without writes; definite failures stay visible in the dialog.
  detailRecord = makePurchase();
  await page.goto(`${base}/business/purchases`);
  await page.getByRole('button', { name: '过账', exact: true }).click();
  let confirmation = page.getByRole('dialog'); await confirmation.waitFor();
  await confirmation.getByRole('region', { name: '商品明细' }).getByText('采购验收商品', { exact: true }).waitFor();
  const beforeCancel = { createCount, postCount };
  await confirmation.getByRole('button', { name: '取消', exact: true }).click();
  assert.deepEqual({ createCount, postCount }, beforeCancel, 'cancel confirmation sends no writes');
  await page.getByRole('button', { name: '过账', exact: true }).click(); confirmation = page.getByRole('dialog');
  postMode = 'fail'; await confirmation.getByRole('button', { name: '确认过账', exact: true }).click();
  await confirmation.getByRole('alert').filter({ hasText: '过账失败' }).waitFor();
  assert.equal(await confirmation.count(), 1, 'posting error stays visible with the confirmation content');
  assert.match(await confirmation.innerText(), /已核实.*当前为草稿/);
  assert.equal(await confirmation.getByRole('button', { name: '请刷新后重新核对', exact: true }).isDisabled(), true, 'a stale list confirmation cannot blindly retry');
  await confirmation.getByRole('button', { name: '关闭确认', exact: true }).click();
  postMode = 'ok'; await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByRole('button', { name: '过账', exact: true }).click(); confirmation = page.getByRole('dialog');
  await confirmation.getByRole('button', { name: '确认过账', exact: true }).click();

  // Save failure never posts, and an uncertain post can only retry the saved ID.
  await page.goto(`${base}/business/purchases/new`);
  await choose(page, '供应商', partner.code); await choose(page, '第 1 行商品', product.code);
  await page.getByRole('button', { name: '保存并过账', exact: true }).click();
  confirmation = page.getByRole('dialog');
  assert.equal(await confirmation.getByRole('row').count(), 2, 'one line item is shown in a table');
  await confirmation.getByRole('button', { name: '返回修改', exact: true }).click();
  assert.equal(postCount, beforeCancel.postCount + 2, 'returning to edit does not post');
  saveMode = 'fail';
  await page.getByRole('button', { name: '保存并过账', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认保存并过账', exact: true }).click();
  await page.locator('main p[role="alert"]').filter({ hasText: '采购模拟失败' }).waitFor();
  assert.equal(postCount, beforeCancel.postCount + 2, 'save rejection never calls posting');
  saveMode = 'ok'; postMode = 'unknown';
  await page.getByRole('button', { name: '保存并过账', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认保存并过账', exact: true }).click();
  await page.getByRole('button', { name: '重新核对并重试过账', exact: true }).waitFor();
  const savedID = created.id, savedNo = created.documentNo;
  assert.equal(created.status, 'DRAFT');
  await page.getByRole('button', { name: '核实过账状态', exact: true }).click();
  await page.getByRole('button', { name: '重新核对并重试过账', exact: true }).click();
  confirmation = page.getByRole('dialog');
  await confirmation.getByRole('button', { name: '确认重试过账', exact: true }).waitFor();
  assert.match(await confirmation.innerText(), new RegExp(savedNo));
  assert.match(await confirmation.innerText(), /增加 1 台/);
  await page.setViewportSize({ width: 1440, height: 900 });
  if (process.env.PURCHASE_CONFIRM_SCREENSHOT) await confirmation.screenshot({ path: process.env.PURCHASE_CONFIRM_SCREENSHOT });
  postMode = 'ok'; await confirmation.getByRole('button', { name: '确认重试过账', exact: true }).click();
  assert.equal(created.id, savedID, 'safe retry retains the saved purchase ID');
  assert.equal(created.status, 'POSTED');
  assert.equal(createCount, 2, 'safe retry does not create another purchase');
  assert.deepEqual(errors, []);
  console.log('PASS purchase dialogs: status/details, long and narrow layout, legacy/new direct delivery, zero amount, cancel without writes, save failure, same-ID safe retry');
} finally {
  if (browser) await browser.close();
  vite.kill();
}
