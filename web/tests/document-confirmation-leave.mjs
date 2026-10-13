// The existing leave guard must remain above the document confirmation.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const base = 'http://127.0.0.1:4216';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4216', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
const product = { id: 1, code: 'LEAVE-G', name: '离开保护商品', type: 'GOODS', unit: '台', status: 1 };
const partner = { id: 1, code: 'LEAVE-P', name: '离开保护单位', type: 'COMPANY', isSupplier: true, isCustomer: true, status: 1 };
const document = { id: 1, documentNo: 'LEAVE-001', partnerId: 1, partnerName: partner.name, businessDate: '2026-10-13', status: 'DRAFT', version: 1, directDelivery: false, directDocuments: [], createdBy: 1, createdByName: 'fixture', createTime: '2026-10-13T00:00:00Z', totalAmount: '10.00', deliveryContact: '', deliveryPhone: '', deliveryAddress: '', items: [{ id: 1, productId: 1, productCode: product.code, productName: product.name, productType: 'GOODS', unit: product.unit, quantity: '1', unitPrice: '10.00', amount: '10.00' }] };
const errors = [], writes = [];
let browser;
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch { /* starting */ } await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1024, height: 600 } });
  await context.addInitScript(() => localStorage.setItem('web-auth', JSON.stringify({ token: 'leave-fixture', user: null })));
  await context.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (route.request().method() !== 'GET') { writes.push(path); return route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ code: 400, message: 'unexpected write' }) }); }
    let data = { records: [], total: 0, page: 1, pageSize: 10 };
    if (path === '/api/auth/me') data = { id: 1, username: 'fixture', roles: [] };
    else if (path === '/api/auth/menus' || path.includes('/dict/')) data = [];
    else if (path.endsWith('/unread-count')) data = 0;
    else if (/\/v1\/(purchases|sales)\/1$/.test(path)) data = document;
    else if (/\/v1\/(purchases|sales)$/.test(path)) data = { records: [document], total: 1, page: 1, pageSize: 10 };
    else if (path === '/api/v1/products/1') data = product;
    else if (path === '/api/v1/partners/1') data = partner;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'OK', data }) });
  });
  const page = await context.newPage(); page.on('pageerror', error => errors.push(error.message));
  for (const kind of ['purchases', 'sales']) {
    await page.goto(`${base}/business/${kind}`);
    await page.getByRole('button', { name: '编辑', exact: true }).click();
    await page.locator('main textarea').first().fill('未保存的整单说明');
    await page.getByRole('button', { name: '保存并过账', exact: true }).click();
    const confirmation = page.getByRole('dialog'); await confirmation.waitFor();
    await page.goBack();
    const warning = page.getByRole('alertdialog'); await warning.waitFor();
    const stay = warning.getByRole('button', { name: '继续录单', exact: true });
    assert.ok(await stay.evaluate(node => { const r = node.getBoundingClientRect(); return node.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)); }), 'leave decision is visually above the underlying confirmation');
    await stay.focus(); await page.keyboard.press('Tab');
    assert.ok(await warning.evaluate(node => node.contains(document.activeElement)), 'only top-level leave decision traps focus');
    await page.keyboard.press('Escape'); await warning.waitFor({ state: 'hidden' });
    assert.ok(await confirmation.isVisible(), 'Escape dismisses only the leave decision');
    assert.equal(writes.length, 0);
    await confirmation.getByRole('button', { name: '返回修改', exact: true }).click();
    await confirmation.waitFor({ state: 'hidden' });
    assert.equal(await page.locator('main textarea').first().inputValue(), '未保存的整单说明');
    await page.goBack(); await warning.waitFor();
    await warning.getByRole('button', { name: '确认离开', exact: true }).click();
    await page.waitForURL(url => url.pathname === `/business/${kind}`);
    console.log(`PASS ${kind} posting preview leave protection: top dialog, focus, Escape, preserved input and zero writes`);
  }
  assert.deepEqual(writes, []); assert.deepEqual(errors, []);
} finally { await browser?.close(); vite.kill('SIGTERM'); }
