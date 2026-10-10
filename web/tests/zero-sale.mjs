// All API calls are intercepted; this can also inspect a downloaded public production build.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const base = 'http://127.0.0.1:4186';
const args = process.env.ZERO_SALE_ASSET_DIR ? ['preview', '--outDir', process.env.ZERO_SALE_ASSET_DIR] : [];
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', ...args, '--host', '127.0.0.1', '--port', '4186', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
const product = { id: 1, code: 'ZERO-MOUSE', name: '回归鼠标', type: 'GOODS', unit: '个', salePrice: '20.00', status: 1 };
const partner = { id: 1, code: 'ZERO-P', name: '回归客户', type: 'PERSON', isCustomer: true, status: 1 };
const records = values => ({ records: values, total: values.length, page: 1, pageSize: 10 });
let browser, saved;
const writes = [], errors = [];
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch { /* starting */ } await delay(100); }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ timezoneId: 'America/Los_Angeles' });
  await context.addInitScript(() => localStorage.setItem('web-auth', JSON.stringify({ token: 'isolated-zero-fixture', user: null })));
  await context.route('**/api/**', async route => {
    const req = route.request(), path = new URL(req.url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    let data = records([]);
    if (req.method() !== 'GET') {
      const body = req.postDataJSON(); writes.push({ path, body });
      if (path.endsWith('/post')) { saved = { ...saved, status: 'POSTED', version: saved.version + 1 }; }
      else {
        assert.equal(body.items[0].unitPrice, '0');
        saved = { ...body, id: 1, documentNo: 'ZERO-001', status: 'DRAFT', version: 1, totalAmount: '0.00', partnerName: partner.name, createTime: '2026-10-09T16:15:00Z', createdByName: '回归', directDocuments: [], items: body.items.map((i, n) => ({ ...i, id: n + 1, amount: '0.00', productName: product.name, productCode: product.code })) };
      }
      data = saved;
    } else if (path === '/api/auth/me') data = { id: 1, username: 'fixture', roles: [] };
    else if (path === '/api/auth/menus') data = [];
    else if (path === '/api/v1/products') data = records([product]);
    else if (path === '/api/v1/products/1') data = product;
    else if (path === '/api/v1/partners') data = records([partner]);
    else if (path === '/api/v1/partners/1') data = partner;
    else if (path === '/api/v1/sales/1') data = saved;
    else if (path.endsWith('/unread-count')) data = 0;
    await route.fulfill({ json: { code: 200, message: 'OK', data } });
  });
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on('pageerror', e => errors.push(e.message));
  await context.addInitScript(() => {
    const NativeDate = Date;
    class FixedDate extends NativeDate {
      constructor(...args) { super(...(args.length ? args : ['2026-10-09T16:15:00Z'])); }
      static now() { return new NativeDate('2026-10-09T16:15:00Z').getTime(); }
    }
    window.Date = FixedDate;
  });
  await page.goto(`${base}/business/sales/new`);
  await page.locator('main h1').waitFor().catch(async error => {
    console.log(await page.locator('body').innerText());
    console.log(errors);
    throw error;
  });
  for (const [label, name] of [['客户', partner.name], ['第 1 行商品', product.name]]) {
    const group = page.getByRole('group', { name: label, exact: true });
    await group.getByRole('button').first().click(); await group.getByRole('textbox').fill(name);
    await group.locator('button[aria-pressed]').filter({ hasText: name }).last().click();
  }
  if (!process.env.ZERO_SALE_ASSET_DIR) assert.equal(await page.getByLabel('业务日期').inputValue(), '2026-10-10');
  const price = page.getByLabel('成交单价（元）');
  await price.fill('');
  assert.equal(await page.getByRole('button', { name: '保存草稿', exact: true }).isDisabled(), true);
  await price.fill('0');
  assert.equal(await page.getByRole('button', { name: '保存草稿', exact: true }).isEnabled(), true);
  await page.getByRole('button', { name: '保存草稿', exact: true }).click();
  await page.getByRole('status', { name: '操作结果' }).filter({ hasText: '销售草稿保存成功' }).waitFor();
  assert.equal(writes[0].path, '/api/v1/sales');
  assert.equal(saved.totalAmount, '0.00');
  await page.getByRole('button', { name: '保存并过账', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: '确认保存并过账' }).click();
  await page.getByRole('status', { name: '操作结果' }).filter({ hasText: '销售单过账成功' }).waitFor();
  assert.equal(writes.at(-1).path, '/api/v1/sales/1/post');
  assert.equal(saved.status, 'POSTED');
  assert.deepEqual(errors, []);
  console.log(`PASS ${process.env.ZERO_SALE_ASSET_DIR ? 'public production build' : 'local UI with Beijing default date in Los Angeles browser'}: mouse price 20 -> blank (blocked) -> 0, save and post requests; API fully intercepted`);
} finally { await browser?.close(); vite.kill(); }
