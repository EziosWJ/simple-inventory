// The real inventory list renders fixture API responses; no production writes.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4197';
const args = process.env.ENTRY_REASON_ASSET_DIR ? ['preview', '--outDir', process.env.ENTRY_REASON_ASSET_DIR] : [];
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', ...args, '--host', '127.0.0.1', '--port', '4197', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
const labels = [
  ['PURCHASE', '采购入库'], ['PURCHASE_CANCEL', '采购入库取消'],
  ['SALE', '销售出库'], ['SALE_CANCEL', '销售出库取消'],
  ['PURCHASE_RETURN', '采购退货'], ['PURCHASE_RETURN_CANCEL', '采购退货取消'],
  ['SALE_RETURN', '销售退货'], ['SALE_RETURN_CANCEL', '销售退货取消'],
  ['OTHER', '自定义其他调整'],
];
const records = labels.map(([reason], i) => ({ id: i + 1, productId: 1, sourceType: reason === 'OTHER' ? 'ADJUSTMENT' : reason.replace('_CANCEL', ''), entryType: reason.endsWith('_CANCEL') ? 'REVERSAL' : 'ORIGINAL', reason, productCode: 'TEST', productName: '原因回归商品', unit: '个', quantity: '1', balanceBefore: '0', balanceAfter: '1', documentNo: `TEST-${i}`, operatorId: 1, operatorName: '测试', occurredAt: '2026-10-10T00:00:00Z' }));
let browser;
try {
  for (let i = 0; i < 80; i++) {
    try { if ((await fetch(base)).ok) break; } catch { /* starting */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  await context.addInitScript(() => localStorage.setItem('web-auth', JSON.stringify({ token: 'isolated-entry-reasons', user: null })));
  await context.route('**/api/**', route => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    assert.equal(route.request().method(), 'GET');
    let data = { records: [], total: 0, page: 1, pageSize: 10 };
    if (path === '/api/auth/me') data = { id: 1, username: 'regression', roles: [] };
    else if (path === '/api/auth/menus') data = [];
    else if (path.endsWith('/unread-count')) data = 0;
    else if (path.includes('/dict/')) data = [['OPENING', '期初录入'], ['SURPLUS', '盘盈'], ['SHORTAGE', '盘亏'], ['DAMAGE', '报损'], ['OTHER', '自定义其他调整']].map(([value, label], sortOrder) => ({ value, label, sortOrder }));
    else if (path === '/api/v1/inventory/entries') data = { records, total: records.length, page: 1, pageSize: 10 };
    return route.fulfill({ json: { code: 200, message: 'ok', data } });
  });
  const page = await context.newPage();
  page.on('pageerror', error => console.log('PAGE_ERROR', error.message));
  await page.goto(base + '/business/inventory-entries');
  await page.getByRole('cell', { name: 'TEST-8', exact: true }).waitFor({ timeout: 5000 }).catch(async error => { throw new Error(error.message + '\n' + await page.locator('body').innerText()); });
  await page.getByRole('cell', { name: '自定义其他调整', exact: true }).waitFor();
  const rows = page.locator('tbody tr');
  for (let i = 0; i < labels.length; i++) {
    const cells = await rows.nth(i).locator('td').allTextContents();
    assert.equal(cells[6], labels[i][1], labels[i][0]);
  }
  assert(!((await page.locator('main').innerText()).includes('字典项缺失')));
  console.log('PASS inventory reasons: purchase/sale/returns/cancellations and configured adjustment labels; API GET fixtures only');
} finally {
  await browser?.close();
  vite.kill();
}
