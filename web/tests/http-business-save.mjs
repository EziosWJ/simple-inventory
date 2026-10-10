// Isolated HTTP browser regression; API writes are intercepted.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const base = 'http://inventory.test:4198';
const assetArgs = process.env.HTTP_ASSET_DIR ? ['preview', '--outDir', process.env.HTTP_ASSET_DIR] : [];
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', ...assetArgs, '--host', '127.0.0.1', '--port', '4198', '--strictPort'], { cwd: new URL('..', import.meta.url), env: { ...process.env, __VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS: 'inventory.test' }, stdio: 'ignore' });
const product = { id: 1, code: 'HTTP-G', name: '复印机墨盒', type: 'GOODS', unit: '盒', purchasePrice: '200.00', status: 1 };
const partner = { id: 1, code: 'HTTP-P', name: '王孝永', type: 'PERSON', isSupplier: true, isCustomer: true, status: 1 };
const records = values => ({ records: values, total: values.length, page: 1, pageSize: 10 });
const line = { id: 1, productId: 1, productCode: product.code, productName: product.name, productType: 'GOODS', unit: product.unit, quantity: '1', unitPrice: '200.00', amount: '200.00' };
const draft = { id: 1, documentNo: 'HTTP-001', partnerId: 1, partnerName: partner.name, businessDate: '2026-10-08', status: 'DRAFT', version: 1, directDelivery: false, directDocuments: [], createdBy: 1, createdByName: 'HTTP经营者', createTime: '2026-10-08T00:00:00Z', items: [line], totalAmount: '200.00', deliveryContact: '', deliveryPhone: '', deliveryAddress: '' };
const returned = { ...draft, purchaseId: 1, saleId: 1, purchaseNo: draft.documentNo, saleNo: draft.documentNo, items: [{ ...line, purchaseItemId: 1, saleItemId: 1, originalQuantity: '1', returnedQuantity: '0', remainingQuantity: '1', priorReturnAmount: '0.00' }] };
const adjustment = { ...draft, items: [{ ...line, reason: 'OPENING', remark: '' }] };
const dicts = { PRODUCT_TYPE: [['GOODS', '实物'], ['SERVICE', '服务']], PARTNER_TYPE: [['COMPANY', '单位'], ['PERSON', '个人']], PARTNER_IDENTITY: [['CUSTOMER', '客户'], ['SUPPLIER', '供应商']], BUSINESS_STATUS: [['1', '启用'], ['0', '停用']], INVENTORY_ADJUSTMENT_STATUS: [['DRAFT', '草稿'], ['POSTED', '已过账'], ['CANCELLED', '已取消']], INVENTORY_ADJUSTMENT_REASON: [['OPENING', '期初'], ['OTHER', '其他']] };
const writes = [], errors = [];
let browser;
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch('http://127.0.0.1:4198')).ok) break; } catch { /* starting */ } await delay(100); }
  browser = await chromium.launch({ headless: true, args: ['--no-proxy-server', '--host-resolver-rules=MAP inventory.test 127.0.0.1'], ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext();
  await context.addInitScript(() => localStorage.setItem('web-auth', JSON.stringify({ token: 'http-fixture', user: null })));
  await context.route('**/*', async route => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) {
      const url = new URL(route.request().url());
      url.hostname = '127.0.0.1';
      return route.fulfill({ response: await route.fetch({ url: url.toString() }) });
    }
    let data = records([]);
    if (route.request().method() !== 'GET') {
      writes.push({ path, method: route.request().method(), body: route.request().postDataJSON() });
      return route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ code: 400, message: 'HTTP回归已收到保存请求', data: null }) });
    }
    if (path === '/api/auth/me') data = { id: 1, username: 'http-fixture', roles: [] };
    else if (path === '/api/auth/menus') data = [];
    else if (path === '/api/v1/products') data = records([product]);
    else if (path === '/api/v1/products/1') data = product;
    else if (path === '/api/v1/partners') data = records([partner]);
    else if (path === '/api/v1/partners/1') data = partner;
    else if (path.includes('/dict/')) data = (dicts[path.split('/').at(-2)] ?? []).map(([value, label]) => ({ value, label }));
    else if (/^\/api\/v1\/(purchases|sales)\/1$/.test(path)) data = draft;
    else if (/^\/api\/v1\/(purchase-returns|sale-returns)$/.test(path)) data = records([returned]);
    else if (/^\/api\/v1\/(purchase-returns|sale-returns)\/(1|source\/1)$/.test(path)) data = returned;
    else if (path === '/api/v1/inventory/adjustments') data = records([adjustment]);
    else if (path === '/api/v1/inventory/adjustments/1') data = adjustment;
    else if (path === '/api/v1/warehouse') data = { name: 'HTTP仓库', remark: '' };
    else if (path === '/api/v1/print-profile') data = { name: 'HTTP经营者', phone: '', address: '' };
    else if (path.endsWith('/unread-count')) data = 0;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'OK', data }) });
  });
  const page = await context.newPage();
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(`${base}/business/purchases/new`);
  page.setDefaultTimeout(15000);
  assert.deepEqual(await page.evaluate(() => ({ secure: isSecureContext, uuid: typeof crypto.randomUUID, random: typeof crypto.getRandomValues })), { secure: false, uuid: 'undefined', random: 'function' });
  for (const [label, name] of [['供应商', partner.name], ['第 1 行商品', product.name]]) {
    const group = page.getByRole('group', { name: label, exact: true });
    await group.getByRole('button').first().click();
    await group.getByRole('textbox').fill(name);
    await group.locator('button[aria-pressed]').filter({ hasText: name }).last().click();
  }
  const checkSave = async () => {
    const before = writes.length;
    await page.getByRole('button', { name: '保存草稿', exact: true }).click();
    await delay(400);
    assert.equal(writes.length, before + 1, `HTTP保存应发送请求；浏览器错误：${errors.join('; ')}`);
    assert.match(writes.at(-1).body.requestKey, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
    assert.equal(writes.at(-1).body.items[0].quantity, '1');
    assert.equal(writes.at(-1).body.items[0].unitPrice, '200.00');
    await page.getByRole('alert').filter({ hasText: 'HTTP回归已收到保存请求' }).first().waitFor();
    assert.equal(await page.getByLabel('数量').inputValue(), '1');
  };
  await checkSave();
  await checkSave();
  assert.notEqual(writes[0].body.requestKey, writes[1].body.requestKey);
  await page.getByRole('button', { name: '保存并过账', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: '确认保存并过账', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  assert.equal(writes.length, 3);
  assert.ok(writes.every(w => w.path === '/api/v1/purchases'), '保存失败不能继续过账');
  const sale = await context.newPage();
  sale.on('pageerror', e => errors.push(e.message));
  await sale.goto(`${base}/business/sales/new`);
  for (const [label, name] of [['客户', partner.name], ['第 1 行商品', product.name]]) {
    const group = sale.getByRole('group', { name: label, exact: true });
    await group.getByRole('button').first().click();
    await group.getByRole('textbox').fill(name);
    await group.locator('button[aria-pressed]').filter({ hasText: name }).last().click();
  }
  await sale.getByRole('button', { name: '保存草稿', exact: true }).click();
  await sale.getByRole('alert').filter({ hasText: 'HTTP回归已收到保存请求' }).first().waitFor();
  assert.equal(writes.at(-1).path, '/api/v1/sales');
  for (const route of ['settlements', 'refunds', 'opening-balances']) {
    const funds = await context.newPage();
    funds.on('pageerror', e => errors.push(e.message));
    await funds.goto(`${base}/business/${route}`);
    await funds.locator('main h1').waitFor();
    await funds.close();
  }
  for (const kind of ['purchases', 'sales']) {
    const editor = await context.newPage();
    editor.on('pageerror', e => errors.push(e.message));
    await editor.goto(`${base}/business/${kind}/1/edit`);
    await editor.getByLabel('数量').fill('2');
    await editor.getByRole('button', { name: '保存草稿', exact: true }).click();
    await editor.getByRole('alert').filter({ hasText: 'HTTP回归已收到保存请求' }).first().waitFor();
    assert.equal(writes.at(-1).method, 'PUT');
    assert.equal(writes.at(-1).path, `/api/v1/${kind}/1`);
    assert.match(writes.at(-1).body.requestKey, /^[0-9a-f-]{36}$/);
    await editor.close();
    console.log(`PASS HTTP ${kind} draft edit/save`);
  }
  for (const kind of ['products', 'partners', 'inventory-adjustments', 'purchase-returns', 'sale-returns']) {
    const editor = await context.newPage();
    editor.on('pageerror', e => errors.push(e.message));
    await editor.goto(`${base}/business/${kind}`);
    if (kind === 'inventory-adjustments') {
      await editor.getByRole('button', { name: '查看', exact: true }).first().click();
      await editor.getByRole('button', { name: '编辑草稿', exact: true }).click();
    } else await editor.getByRole('button', { name: '编辑', exact: true }).first().click();
    const dialog = editor.getByRole('dialog', { name: /编辑/ });
    const field = dialog.locator(kind === 'inventory-adjustments' ? 'input[placeholder="例如 2 或 -0.125"]' : 'input:not([type="date"]):not([type="checkbox"])').first();
    await field.fill('1');
    await dialog.getByRole('button', { name: /^(保存|保存修改|保存草稿)$/ }).click();
    await dialog.getByRole('alert').filter({ hasText: 'HTTP回归已收到保存请求' }).waitFor();
    assert.equal(writes.at(-1).method, 'PUT');
    assert.equal(writes.at(-1).path, kind === 'inventory-adjustments' ? '/api/v1/inventory/adjustments/1' : `/api/v1/${kind}/1`);
    assert.equal(await field.inputValue(), '1');
    await editor.close();
    console.log(`PASS HTTP ${kind} edit/save`);
  }
  for (const kind of ['warehouse', 'print-profile']) {
    const editor = await context.newPage();
    editor.on('pageerror', e => errors.push(e.message));
    await editor.goto(`${base}/business/${kind}`);
    await editor.locator('main input').first().fill('HTTP修改名称');
    await editor.getByRole('button', { name: /保存/ }).click();
    await editor.getByRole('alert').filter({ hasText: 'HTTP回归已收到保存请求' }).first().waitFor();
    assert.equal(writes.at(-1).path, `/api/v1/${kind}`);
    assert.equal(writes.at(-1).body.name, 'HTTP修改名称');
    await editor.close();
    console.log(`PASS HTTP ${kind} save`);
  }
  assert.deepEqual(errors, []);
  console.log('PASS HTTP purchase save/retry/save-post, sales save and funds pages; no browser exceptions');
} finally { if (browser) await browser.close(); vite.kill(); }
