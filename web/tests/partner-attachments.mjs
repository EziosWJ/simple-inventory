// UI behavior uses intercepted requests; the Go dual-database HTTP contracts verify real files and transactions.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const local = 'http://127.0.0.1:4203';
const insecure = process.env.HTTP_TEST === '1';
const base = insecure ? 'http://inventory.test:4203' : local;
const assetArgs = process.env.HTTP_ASSET_DIR ? ['preview', '--outDir', process.env.HTTP_ASSET_DIR] : [];
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', ...assetArgs, '--host', '127.0.0.1', '--port', '4203', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
const partner = { id: 1, code: 'ATT-P', name: '附件往来单位', isCustomer: true, isSupplier: true, status: 1 };
const menus = ['settlement', 'refund', 'opening-balance'].map((code, i) => ({ id: i + 1, parentId: 0, menuName: code, menuType: 'MENU', path: '', permissionCode: `business:${code}`, children: [] }));
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6iS8AAAAASUVORK5CYII=', 'base64');
let browser;
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(local)).ok) break; } catch {} await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  for (const [routeName, category, entryType, amount, , direction] of [
    ['settlements', 'SETTLEMENT', 'RECEIPT', '100.00', '确认客户收款', 'CUSTOMER'],
    ['settlements', 'SETTLEMENT', 'PAYMENT', '100.00', '确认供应商付款', 'SUPPLIER'],
    ['refunds', 'REFUND', 'CUSTOMER_REFUND', '-100.00', '确认向客户退款', 'CUSTOMER'],
    ['refunds', 'REFUND', 'SUPPLIER_REFUND', '-100.00', '确认收到供应商退款', 'SUPPLIER'],
    ['opening-balances', 'OPENING', 'OPENING', '0.00', '确认录入期初', 'CUSTOMER'],
    ['opening-balances', 'OPENING', 'OPENING', '0.00', '确认录入期初', 'SUPPLIER'],
  ]) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    await context.addInitScript(() => localStorage.setItem('web-auth', JSON.stringify({ token: 'Bearer attachment-ui', user: { id: 1, username: 'attachment-ui', nickname: '经营者', roles: [] } })));
    let entry = { id: 1, partnerId: 1, partnerName: partner.name, direction, entryType, amount: '10.00', balanceBefore: amount, balanceAfter: '90.00', businessDate: '2026-10-03', effectiveAt: '2026-10-03T00:00:00Z', documentNo: `ATT-${category}`, description: '附件业务', operatorId: 1, operatorName: '经营者', reversedById: null, attachmentCount: 0 };
    let files = new Map(), items = [], failedOnce = false, writes = 0, rejectSaveOnce = true, nextFile = 1;
    const pageErrors = [];
    const saveKeys = [];
    if (insecure) await context.route('**/*', async route => {
      const url = new URL(route.request().url());
      if (url.pathname.startsWith('/api/')) return route.fallback();
      url.hostname = '127.0.0.1';
      return route.fulfill({ response: await route.fetch({ url: url.toString() }) });
    });
    await context.route('**/api/**', async route => {
      const req = route.request(), path = new URL(req.url()).pathname, method = req.method();
      if (!path.startsWith('/api/')) return route.fallback();
      let data = null;
      const respond = (value, status = 200, msg = 'OK') => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify({ code: status, message: msg, data: value }) });
      if (path === '/api/auth/me') data = { id: 1, username: 'attachment-ui', nickname: '经营者', roles: [] };
      else if (path === '/api/auth/menus') data = menus;
      else if (path.endsWith('/unread-count')) data = 0;
      else if (path.includes('/dict/')) data = [];
      else if (path === '/api/v1/partners') data = { records: [partner], total: 1 };
      else if (path === '/api/v1/partner-balances') data = { records: [{ partnerId: 1, partnerName: partner.name, direction, amount }] };
      else if (path === '/api/system/file/upload') {
        const body = req.postDataBuffer().toString('latin1');
        assert.ok(body.includes(`partner-attachment:${category}`));
        const name = /filename="([^"]+)"/.exec(body)[1];
        if (name === 'discard.pdf') return respond(null, 400, '此附件需要移除');
        if (name === 'retry.pdf' && !failedOnce) { failedOnce = true; return respond(null, 400, '模拟附件上传失败'); }
        const file = { id: nextFile++, originalName: name, fileSize: 100, mimeType: name.endsWith('.pdf') ? 'application/pdf' : 'image/png', accessUrl: `/api/system/file/${nextFile - 1}/view` };
        files.set(file.id, file); data = file;
      } else if (/^\/api\/system\/file\/\d+\/(view|download)$/.test(path)) {
        assert.equal(req.headers().authorization, 'Bearer attachment-ui');
        const file = files.get(Number(path.split('/')[4]));
        return route.fulfill({ contentType: file?.mimeType ?? 'image/png', body: file?.mimeType === 'application/pdf' ? Buffer.from('%PDF-1.4\n%%EOF') : png });
      } else if (/^\/api\/system\/file\/\d+$/.test(path)) data = files.get(Number(path.split('/').at(-1)));
      else if (path.endsWith('/attachments') && method === 'GET') data = items;
      else if (path.endsWith('/attachments') && method === 'POST') {
        for (const id of req.postDataJSON().fileIds) items.push({ ...files.get(id), id: items.length + 1, fileId: id, entryId: 1, uploadedBy: 1, uploadedName: '经营者', uploadedAt: '2026-10-03T00:00:00Z', removedAt: null });
        entry.attachmentCount = items.filter(i => !i.removedAt).length;
      } else if (/\/attachments\/\d+$/.test(path) && method === 'DELETE') {
        const item = items.find(i => i.id === Number(path.split('/').at(-1))); item.removedAt = '2026-10-03T01:00:00Z'; item.removedBy = 1; item.removedName = '经营者'; entry.attachmentCount--;
      } else if (path === '/api/v1/partner-balances/entries') data = { records: writes ? [entry] : [], total: writes ? 1 : 0 };
      else if (path === '/api/v1/partner-balances/entries/1') data = entry;
      else if (path.endsWith('/reverse')) { entry = { ...entry, reversedById: 2 }; }
      else if (method === 'POST' && /\/(opening|settlements|refunds)$/.test(path)) {
        const body = req.postDataJSON();
        assert.match(body.requestKey, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
        assert.equal(body.direction, direction);
        saveKeys.push(body.requestKey);
        if (rejectSaveOnce) { rejectSaveOnce = false; return respond(null, 500, '模拟业务保存失败'); }
        const ids = req.postDataJSON().attachmentIds; assert.equal(ids.length, 2); writes++;
        items = ids.map((id, i) => ({ ...files.get(id), id: i + 1, fileId: id, entryId: 1, uploadedBy: 1, uploadedName: '经营者', uploadedAt: '2026-10-03T00:00:00Z', removedAt: null }));
        entry.attachmentCount = items.length; data = entry;
      }
      return respond(data);
    });
    const page = await context.newPage(); page.on('pageerror', e => pageErrors.push(e.message));
    await page.goto(`${base}/business/${routeName}?partnerId=1&direction=${direction}`);
    if (insecure) assert.deepEqual(await page.evaluate(() => [isSecureContext, typeof crypto.randomUUID]), [false, 'undefined']);
    await page.getByRole('heading', { level: 1 }).waitFor({ timeout: 8000 }).catch(async e => { throw new Error(`${e.message} URL=${page.url()} errors=${JSON.stringify(pageErrors)} text=${(await page.locator('body').innerText()).slice(0,1800)}`); });
    await page.getByLabel(category === 'OPENING' ? '期初金额' : '资金金额', { exact: true }).fill('10.00');
    if (category === 'OPENING') await page.getByLabel('期初说明').fill('接入前余额');
    const confirm = page.getByRole('button', { name: '核对并登记', exact: true }); await confirm.waitFor();
    await page.getByRole('dialog').locator('summary').click();
    await page.getByLabel('选择往来附件').setInputFiles([{ name: 'receipt.png', mimeType: 'image/png', buffer: png }, { name: 'retry.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF') }, { name: 'discard.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF') }]);
    await page.getByText('模拟附件上传失败', { exact: true }).waitFor();
    assert.ok(await confirm.isDisabled()); assert.equal(writes, 0);
    await page.locator('li').filter({ hasText: 'retry.pdf' }).getByRole('button', { name: '重试', exact: true }).click();
    await page.waitForFunction(() => Array.from(document.querySelectorAll('[role="status"]')).filter(n => n.textContent === '上传成功').length === 2);
    assert.ok(await confirm.isDisabled());
    await page.locator('li').filter({ hasText: 'discard.pdf' }).getByRole('button', { name: '移除', exact: true }).click();
    await confirm.click();
    const saveDialog = page.getByRole('dialog'); await saveDialog.waitFor();
    await saveDialog.getByRole('button', { name: '确认登记并生效', exact: true }).click();
    await page.getByText('模拟业务保存失败', { exact: true }).first().waitFor(); assert.equal(writes, 0);
    assert.equal(await page.getByText('上传成功', { exact: true }).count(), 2);
    await confirm.click(); await saveDialog.waitFor();
    await saveDialog.getByRole('button', { name: '确认登记并生效', exact: true }).click();
    await page.getByRole('status', { name: '操作结果' }).filter({ hasText: '成功' }).waitFor();
    assert.equal(await page.getByRole('dialog').count(), 0, '保存成功返回记录列表');
    await page.getByRole('button', { name: '查看详情', exact: true }).click();
    assert.equal(saveKeys.length, 2);
    assert.equal(saveKeys[0], saveKeys[1], '业务保存失败后的重试保留同一幂等键');
    const detail = page.getByRole('dialog'); await detail.waitFor();
    await detail.getByRole('heading', { name: '附件（2）', exact: true }).waitFor();
    await detail.getByRole('button', { name: '预览', exact: true }).first().click(); await detail.getByRole('img', { name: 'receipt.png' }).waitFor();
    await detail.getByRole('button', { name: '预览', exact: true }).click(); await detail.locator('iframe[title="retry.pdf"]').waitFor();
    const download = page.waitForEvent('download'); await detail.getByRole('button', { name: '下载', exact: true }).first().click(); await download;
    page.once('dialog', d => d.accept()); await detail.getByRole('button', { name: '移除附件', exact: true }).first().click();
    await detail.getByRole('heading', { name: '附件（1）', exact: true }).waitFor();
    await detail.locator('summary').click(); await detail.getByText(/移除：经营者/).waitFor();
    await detail.getByLabel('选择往来附件').setInputFiles({ name: 'later.png', mimeType: 'image/png', buffer: png });
    await detail.getByText('上传成功', { exact: true }).waitFor(); await detail.getByRole('button', { name: '保存补传附件' }).click();
    await detail.getByRole('heading', { name: '附件（2）', exact: true }).waitFor();
    await detail.getByRole('button', { name: '冲销此记录' }).click();
    const reverseDialog = page.getByRole('dialog'); await reverseDialog.getByRole('textbox').fill('凭证纠错');
    await reverseDialog.getByRole('button', { name: '确认冲销' }).click();
    await page.getByRole('button', { name: '查看详情', exact: true }).click(); await detail.getByText('已冲销，附件和历史只读。').waitFor();
    assert.equal(await detail.getByRole('button', { name: '选择附件' }).count(), 0);
    assert.equal(await detail.getByRole('button', { name: '移除附件', exact: true }).count(), 0);
    assert.deepEqual(pageErrors, []);
    console.log(`PASS ${insecure ? 'HTTP' : 'localhost'} ${category}/${direction}: upload/retry, save/retry/reset, binding, preview/download, supplement, reversal`);
    await context.close();
  }
} finally { await browser?.close(); vite.kill('SIGTERM'); }
