// #67–#73: public HTTP fixtures and real Chromium interactions. Isolated API only.
import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const api = process.env.PHASE5_API_URL ?? 'http://127.0.0.1:18176';
const base = 'http://127.0.0.1:4198';
const prefix = `FIX-${Date.now()}`;
const screenshots = process.env.SALES_SCREENSHOT_DIR ?? '/tmp/sales-query-fixes';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4198', '--strictPort'], {
  cwd: new URL('..', import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: 'ignore',
});
let token, browser;
async function call(path, body, method = body === undefined ? 'GET' : 'POST') {
  const response = await fetch(`${api}/api${path}`, {
    method, headers: { Authorization: token ?? '', 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const result = await response.json();
  assert.equal(response.status, 200, `${path}: ${result.message}`);
  return result.data;
}
async function eventually(check, message) {
  for (let n = 0; n < 100; n++) { if (await check()) return; await delay(50); }
  assert.ok(await check(), message);
}
try {
  await mkdir(screenshots, { recursive: true });
  for (let n = 0; n < 60; n++) { try { if ((await fetch(base)).ok) break; } catch {} await delay(250); }
  token = (await call('/auth/login', { username: 'admin', password: process.env.PHASE5_UI_PASSWORD ?? 'admin123' })).tokenValue;
  const customer = await call('/v1/partners', { code: `${prefix}-C`, name: '销售整改验收客户（长名称及历史交易查询）', type: 'COMPANY', isCustomer: true });
  const goods = await call('/v1/products', { code: `${prefix}-A`, name: '同名硒鼓', model: '规格 A', specification: '小容量', type: 'GOODS', unit: '支' });
  const twin = await call('/v1/products', { code: `${prefix}-B`, name: '同名硒鼓', model: '规格 B', specification: '大容量', type: 'GOODS', unit: '盒' });
  const service = await call('/v1/products', { code: `${prefix}-S`, name: '安装服务', type: 'SERVICE', unit: '次' });
  const line = (product, quantity, unitPrice) => ({ productId: product.id, productType: product.type, unit: product.unit, quantity, unitPrice });
  for (const product of [goods, twin]) {
    const adjustment = await call('/v1/inventory/adjustments', { items: [{ productId: product.id, productType: 'GOODS', unit: product.unit, quantity: '100', reason: 'OPENING' }] });
    await call(`/v1/inventory/adjustments/${adjustment.id}/post`, { version: 1 });
  }
  async function sale(items, date = '2026-07-15', status = 'POSTED') {
    let document = await call('/v1/sales', { partnerId: customer.id, businessDate: date, items });
    if (status === 'POSTED') document = await call(`/v1/sales/${document.id}/post`, { version: 1 });
    return document;
  }
  const bulk = [];
  for (let n = 0; n < 12; n++) bulk.push(await sale([line(twin, '1', String(10 + n % 3))]));
  const mixed = await sale([line(goods, '0.1', '10'), line(goods, '0.2', '20'), line(twin, '0.125', '0'), line(service, '1', '30')], '2026-09-15', 'DRAFT');
  const pure = await sale([line(service, '1', '0')], '2026-09-16', 'DRAFT');
  const zero = await sale([line(goods, '0.5', '0')], '2026-09-17', 'DRAFT');
  await call(`/v1/products/${goods.id}`, { name: '改名硒鼓', brand: `Brand%_X-${prefix}`, model: '新型号', type: 'GOODS', unit: '支' }, 'PUT');

  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(value => localStorage.setItem('web-auth', JSON.stringify({ token: value, user: null })), token);
  const page = await context.newPage(); page.setDefaultTimeout(10000);
  const errors = []; page.on('pageerror', error => errors.push(error.message));
  let listRequests = 0, detailRequests = 0;
  page.on('request', request => {
    if (request.method() !== 'GET') return;
    const path = new URL(request.url()).pathname;
    if (path === '/api/v1/sales' || path === '/api/v1/sales/items') listRequests++;
    if (/\/api\/v1\/sales\/\d+$/.test(path)) detailRequests++;
  });
  const group = label => page.getByRole('group', { name: label, exact: true });
  const query = page.getByRole('button', { name: '查询', exact: true });
  const reset = page.getByRole('button', { name: '重置', exact: true });
  const refresh = page.getByRole('button', { name: '刷新', exact: true });
  const row = number => page.getByRole('row').filter({ hasText: number });
  const summary = page.getByRole('region', { name: '销售明细全结果合计' });
  const from = page.getByLabel('业务日期起'), to = page.getByLabel('业务日期止');
  async function quick(label, code, wait = true) {
    await group(label).getByRole('combobox').fill(code);
    await page.getByRole('option').filter({ hasText: code }).click();
    if (wait) await page.waitForURL(url => url.searchParams.get(label === '客户筛选' ? 'partnerId' : 'productId') === String(label === '客户筛选' ? customer.id : goods.id));
  }
  async function ready() {
    await eventually(async () => await page.locator('tbody tr').count() > 0, 'expected loaded rows');
    await delay(100);
  }
  await page.goto(`${base}/business/sales?partnerId=${customer.id}`);
  await row(zero.documentNo).waitFor();
  // Inline controls are on one line, editable, independently focusable.
  for (const label of ['客户筛选', '商品或服务筛选']) {
    const input = group(label).getByRole('combobox');
    const browse = group(label).getByRole('button', { name: `浏览选择${label}` });
    const inputBox = await input.boundingBox(), browseBox = await browse.boundingBox();
    assert.equal(inputBox.y, browseBox.y);
    assert.ok(browseBox.x >= inputBox.x && browseBox.x + browseBox.width <= inputBox.x + inputBox.width + 1);
    await browse.focus(); assert.ok(await browse.evaluate(element => element === document.activeElement));
  }
  assert.equal(await group('客户筛选').getByRole('combobox').getAttribute('placeholder'), '搜索名称、编码、联系人或电话');
  assert.equal(await group('商品或服务筛选').getByRole('combobox').getAttribute('placeholder'), '搜索名称、编码、品牌或型号');

  // A selected customer query remains slow while dates are typed and applied.
  await page.route('**/api/v1/sales?*', async route => { const response = await route.fetch(); await delay(600); await route.fulfill({ response }); });
  await quick('客户筛选', customer.code, false);
  await from.fill('2026-07-01'); await to.fill('2026-09-30'); await query.click();
  await page.waitForURL(url => url.searchParams.get('businessFrom') === '2026-07-01');
  await row(zero.documentNo).waitFor();
  assert.equal(await from.inputValue(), '2026-07-01'); assert.equal(await to.inputValue(), '2026-09-30');
  await page.unroute('**/api/v1/sales?*');
  // Unsubmitted inputs survive paging, view changes, sorting and details.
  await from.fill('2026-06-02'); await page.getByLabel('销售单号').fill('未提交的单号');
  await page.getByRole('button', { name: '下一页' }).click(); await row(bulk[0].documentNo).waitFor();
  assert.equal(await from.inputValue(), '2026-06-02'); assert.equal(await page.getByLabel('销售单号').inputValue(), '未提交的单号');
  await page.getByRole('button', { name: '按明细', exact: true }).click(); await ready();
  await page.getByRole('button', { name: '按成交单价排序' }).click(); await ready();
  assert.equal(await from.inputValue(), '2026-06-02');
  const source = page.locator('tbody a').first(); const sourceNo = await source.innerText();
  const before = listRequests, beforeDetail = detailRequests;
  await source.click(); await page.getByRole('dialog').getByText(sourceNo, { exact: false }).first().waitFor();
  await eventually(async () => detailRequests === beforeDetail + 1, 'one detail fetch');
  await page.getByRole('dialog').getByRole('button', { name: '关闭详情弹窗' }).click(); await delay(250);
  assert.equal(listRequests, before, 'details must not refetch the list');
  assert.equal(await from.inputValue(), '2026-06-02');
  assert.equal(await page.getByLabel('销售单号').inputValue(), '未提交的单号');
  await source.click(); await page.getByRole('dialog').waitFor();
  await page.goBack(); await page.getByRole('dialog').waitFor({ state: 'hidden' });
  await page.goForward(); await page.getByRole('dialog').waitFor();
  await page.getByRole('dialog').getByRole('button', { name: '关闭详情弹窗' }).click();
  assert.equal(listRequests, before);

  // Invalid dates are reported in either view and every automatic apply path.
  await from.fill('2026-10-31'); await to.fill('2026-10-01');
  const beforeInvalid = listRequests;
  await query.click(); await page.getByRole('alert').filter({ hasText: '业务日期起始不能晚于结束日期' }).waitFor();
  await page.getByRole('button', { name: '已过账', exact: true }).click();
  await quick('客户筛选', customer.code, false); await delay(250);
  assert.equal(listRequests, beforeInvalid, 'invalid dates must never query');
  await page.getByRole('button', { name: '按单据', exact: true }).click(); await ready();
  await query.click(); assert.equal(await page.getByRole('alert').filter({ hasText: '业务日期起始不能晚于结束日期' }).count(), 1);
  await reset.click(); await ready();

  // Browse the full picker, including a stopped historical archive; clear and type.
  await call(`/v1/products/${twin.id}/status`, { status: 0 }, 'PUT');
  await group('商品或服务筛选').getByRole('button', { name: '浏览选择商品或服务筛选' }).click();
  const picker = page.getByRole('dialog', { name: '选择商品或服务筛选' });
  await picker.getByRole('combobox').fill(twin.code);
  await picker.getByRole('row').filter({ hasText: twin.code }).getByRole('button').click();
  await page.waitForURL(url => url.searchParams.get('productId') === String(twin.id)); await ready();
  assert.equal(await group('商品或服务筛选').getByRole('combobox').inputValue(), twin.name);
  await group('商品或服务筛选').getByRole('button', { name: '清除商品或服务筛选' }).click();
  await page.waitForURL(url => !url.searchParams.has('productId')); await ready();
  await group('商品或服务筛选').getByRole('combobox').fill(`brand%_x-${prefix.toLowerCase()}`);
  await group('商品或服务筛选').getByRole('combobox').press('Enter');
  await page.waitForURL(url => url.searchParams.get('productKeyword') === `brand%_x-${prefix.toLowerCase()}`);
  await eventually(async () => await row(mixed.documentNo).locator('[data-matched="true"]').count() === 2, 'current brand marks both saved A lines');
  assert.equal(await row(mixed.documentNo).locator('[data-matched="true"]').count(), 2);
  await row(mixed.documentNo).getByRole('button', { name: '展开明细' }).click();
  const expanded = page.getByRole('list', { name: `${mixed.documentNo} 全部明细` });
  assert.equal(await expanded.getByRole('listitem').count(), 4);
  assert.equal(await expanded.locator('[data-matched="true"]').count(), 2);
  assert.ok((await expanded.innerText()).includes('规格 A'), 'saved specification remains visible after rename');
  await page.screenshot({ path: `${screenshots}/documents.png`, fullPage: true });
  await page.getByRole('button', { name: '按明细', exact: true }).click(); await ready();
  for (const visibleRow of await page.locator('tbody tr').all()) assert.ok((await visibleRow.innerText()).includes(goods.code));
  let delayedDetail = false;
  await page.route(`**/api/v1/sales/${zero.id}`, async route => { const response = await route.fetch(); delayedDetail = true; await delay(900); try { await route.fulfill({ response }); } catch {} });
  await row(zero.documentNo).getByRole('link').click();
  await eventually(async () => delayedDetail, 'detail response held');
  await page.getByRole('dialog').getByRole('button', { name: '关闭详情弹窗' }).click();
  await row(mixed.documentNo).getByRole('link').first().click();
  await page.getByRole('dialog').getByText('规格 A', { exact: false }).first().waitFor();
  await delay(1000);
  assert.ok((await page.getByRole('dialog').innerText()).includes(mixed.documentNo));
  assert.ok(!(await page.getByRole('dialog').innerText()).includes(zero.documentNo));
  await page.getByRole('dialog').getByRole('button', { name: '关闭详情弹窗' }).click();
  await page.unroute(`**/api/v1/sales/${zero.id}`);
  // Detail error has its own retry, and does not invalidate item totals.
  await page.route(`**/api/v1/sales/${mixed.id}`, route => route.abort('failed'));
  const beforeDetailFailure = listRequests;
  await row(mixed.documentNo).getByRole('link').first().click();
  await page.getByRole('dialog').getByRole('alert').waitFor();
  await page.unroute(`**/api/v1/sales/${mixed.id}`);
  await page.getByRole('button', { name: '重试读取来源单' }).click();
  await page.getByRole('dialog').getByText('规格 A', { exact: false }).first().waitFor();
  await page.getByRole('dialog').getByRole('button', { name: '关闭详情弹窗' }).click();
  assert.equal(listRequests, beforeDetailFailure);
  // Loading hides numbers; failure means unavailable rather than zero sales.
  let delayed = false;
  await page.route('**/api/v1/sales/items?*', async route => { delayed = true; await delay(700); await route.abort('failed'); });
  await refresh.click(); await summary.getByText('合计加载中…').waitFor();
  assert.ok(!(await summary.innerText()).includes('¥'));
  await summary.getByText('合计不可用，请重试查询。').waitFor(); assert.ok(delayed);
  assert.ok(!(await summary.innerText()).includes('¥0.00'));
  await page.unroute('**/api/v1/sales/items?*');
  await page.getByRole('button', { name: '重试查询' }).click(); await ready();

  // Exact posting confirmation, same names with distinct identities, service and zero price.
  await call(`/v1/products/${twin.id}/status`, { status: 1 }, 'PUT');
  await reset.click(); await ready(); await quick('客户筛选', customer.code);
  await page.getByRole('button', { name: '按单据', exact: true }).click(); await row(mixed.documentNo).first().waitFor();
  const confirmations = [];
  page.on('dialog', async dialog => { confirmations.push(dialog.message()); await dialog.accept(); });
  await row(mixed.documentNo).getByRole('button', { name: '过账', exact: true }).click();
  await eventually(async () => (await call(`/v1/sales/${mixed.id}`)).status === 'POSTED', 'mixed sale posted');
  assert.ok(confirmations[0].includes('扣减 0.3 支')); assert.ok(!confirmations[0].includes('0.300000'));
  assert.ok(confirmations[0].includes(twin.code) && confirmations[0].includes('扣减 0.125 盒'));
  assert.ok(confirmations[0].includes(goods.code) && confirmations[0].includes('规格 A'));
  assert.ok(!confirmations[0].includes(service.code), 'service has no stock deduction');
  await row(pure.documentNo).getByRole('button', { name: '过账', exact: true }).click();
  await eventually(async () => (await call(`/v1/sales/${pure.id}`)).status === 'POSTED', 'pure service posted');
  assert.ok(confirmations[1].includes('无实物库存变化')); assert.ok(confirmations[1].includes('零金额，客户应收无变化'));
  await row(zero.documentNo).getByRole('button', { name: '过账', exact: true }).click();
  await eventually(async () => (await call(`/v1/sales/${zero.id}`)).status === 'POSTED', 'zero sale posted');
  assert.ok(confirmations[2].includes('扣减 0.5 支')); assert.ok(confirmations[2].includes('零金额'));
  const balances = await call(`/v1/inventory/balances?pageSize=50&keyword=${encodeURIComponent(prefix)}`);
  assert.equal(balances.records.find(record => record.productId === goods.id).quantity, '99.2');
  assert.equal(balances.records.find(record => record.productId === twin.id).quantity, '87.875');

  // Genuine result shrink corrects the query page as well as URL and pagination.
  await from.fill('2026-07-01'); await to.fill('2026-07-31'); await page.getByLabel('销售状态').selectOption('POSTED'); await query.click(); await ready();
  await page.getByRole('button', { name: '下一页' }).click(); await row(bulk[0].documentNo).waitFor();
  for (const document of bulk.slice(10)) await call(`/v1/sales/${document.id}/cancel`, { version: document.version, reason: '验收页码校正' });
  const refreshed = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/sales' && response.request().method() === 'GET');
  await refresh.click();
  const refreshedData = (await (await refreshed).json()).data;
  assert.equal(refreshedData.total, 10, `shrink total, URL=${page.url()}`);
  assert.equal(refreshedData.page, 2);
  await page.waitForURL(url => !url.searchParams.has('page')); await ready();
  assert.equal(await page.locator('tbody tr').count(), 10); await page.getByText('1 / 1', { exact: true }).waitFor();
  await page.getByRole('button', { name: '按明细', exact: true }).click(); await ready();
  // Simulate persisted old page, then shrink to zero and refresh it.
  await page.goto(`${base}/business/sales?view=item&partnerId=${customer.id}&status=POSTED&businessFrom=2026-07-01&businessTo=2026-07-31&ipage=2`);
  await page.waitForURL(url => !url.searchParams.has('ipage')); await ready();
  for (const document of bulk.slice(0, 10)) await call(`/v1/sales/${document.id}/cancel`, { version: document.version, reason: '验收空结果' });
  await refresh.click(); await page.getByText('没有匹配的销售明细').waitFor();
  assert.ok((await summary.innerText()).includes('¥0.00'));
  assert.equal(new URL(page.url()).searchParams.get('ipage'), null);
  // Layout semantics for posted service, zero-price goods and cancelled originals.
  await from.fill('2026-09-01'); await to.fill('2026-09-30'); await page.getByLabel('销售状态').selectOption(''); await query.click(); await ready();
  assert.equal(await summary.locator('[data-testid="sale-summary-card"]').count(), 3);
  const customerCell = page.locator('tbody tr').first().locator('td').nth(1);
  assert.ok((await customerCell.boundingBox()).width >= 190, 'customer name has readable space');
  assert.ok(!(await page.locator('tbody').innerText()).includes('-¥0.00'));
  for (const cell of await page.locator('tbody td:last-child').all()) {
    const tag = cell.locator('span').first();
    assert.equal(await tag.evaluate(element => getComputedStyle(element).whiteSpace), 'nowrap');
  }
  const cards = summary.locator('[data-testid="sale-summary-card"]');
  const cardBoxes = await Promise.all((await cards.all()).map(card => card.boundingBox()));
  assert.ok(cardBoxes.every(Boolean));
  assert.ok(cardBoxes.every(box => Math.abs(box.y - cardBoxes[0].y) < 2), 'summary cards share one desktop row');
  assert.ok(cardBoxes[2].x + cardBoxes[2].width > cardBoxes[0].x + cardBoxes[0].width * 2.8, 'summary cards use the available row width');
  await page.screenshot({ path: `${screenshots}/items-compact.png`, fullPage: true });
  await summary.getByRole('button', { name: /查看数量合计/ }).click();
  const quantityDialog = page.getByRole('dialog', { name: '数量合计' });
  const quantityTable = quantityDialog.getByRole('table', { name: '数量合计明细' });
  await quantityTable.waitFor();
  assert.ok((await quantityTable.innerText()).includes('净数量'));
  assert.ok((await quantityTable.innerText()).includes('0.8 支'));
  await page.screenshot({ path: `${screenshots}/items-quantities.png`, fullPage: true });
  await quantityDialog.getByRole('button', { name: '关闭数量合计' }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  const narrowBoxes = await Promise.all((await cards.all()).map(card => card.boundingBox()));
  assert.ok(narrowBoxes[1].y > narrowBoxes[0].y && narrowBoxes[2].y > narrowBoxes[1].y, 'summary cards stack on narrow screens');
  await summary.getByRole('button', { name: /查看数量合计/ }).click();
  await quantityDialog.waitFor();
  assert.ok((await quantityDialog.boundingBox()).width <= 390, 'quantity modal fits a narrow viewport');
  const narrowGroups = quantityDialog.getByRole('list', { name: '数量合计明细' });
  assert.ok(await narrowGroups.getByRole('listitem').count() > 0, 'mobile quantity details fit the modal without a wide table');
  await page.screenshot({ path: `${screenshots}/items-quantities-mobile.png`, fullPage: true });
  await quantityDialog.getByRole('button', { name: '关闭数量合计' }).click();
  assert.deepEqual(errors, []);
  console.log('PASS #67–#73 browser: draft/applied filter isolation, immediate dates, request counts, history, inline browser/clear/keyboard, server matching, independent detail retry, loading/failure totals, exact posting and actual inventory, page correction, compact layout and screenshots');
} finally {
  if (browser) await browser.close();
  vite.kill();
}
