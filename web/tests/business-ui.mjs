// Business-only regression. Every API request is intercepted; no real data writes.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const base = 'http://127.0.0.1:4199';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4199', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
const product = { id: 1, code: 'UI-G', name: '回归商品', type: 'GOODS', unit: '台', purchasePrice: '2.00', status: 1 };
const partner = { id: 1, code: 'UI-P', name: '回归往来单位', type: 'COMPANY', isSupplier: true, isCustomer: true, status: 1 };
const line = { id: 1, productId: 1, productCode: product.code, productName: product.name, productType: 'GOODS', unit: '台', quantity: '1', unitPrice: '2.00', amount: '2.00' };
const draft = { id: 1, documentNo: 'UI-001', partnerId: 1, partnerName: partner.name, businessDate: '2026-10-02', status: 'DRAFT', version: 1, directDelivery: false, directDocuments: [], createdBy: 1, createdByName: '回归', createTime: '2026-10-02T00:00:00Z', items: [line], totalAmount: '2.00', deliveryContact: '', deliveryPhone: '', deliveryAddress: '' };
const returnLine = { ...line, purchaseItemId: 1, saleItemId: 1, originalQuantity: '1', returnedQuantity: '0', remainingQuantity: '1', priorReturnAmount: '0.00' };
const returnDraft = { ...draft, purchaseId: 1, saleId: 1, purchaseNo: draft.documentNo, saleNo: draft.documentNo, items: [returnLine] };
const adjustment = { ...draft, items: [{ ...line, reason: 'OPENING', remark: '' }] };
const records = values => ({ records: values, total: values.length, page: 1, pageSize: 10 });
const dicts = {
  PRODUCT_TYPE: [['GOODS', '实物'], ['SERVICE', '服务']], PARTNER_TYPE: [['COMPANY', '单位'], ['PERSON', '个人']],
  PARTNER_IDENTITY: [['CUSTOMER', '客户'], ['SUPPLIER', '供应商']], BUSINESS_STATUS: [['1', '启用'], ['0', '停用']],
  INVENTORY_ADJUSTMENT_STATUS: [['DRAFT', '草稿'], ['POSTED', '已过账'], ['CANCELLED', '已取消']],
  INVENTORY_ADJUSTMENT_REASON: [['OPENING', '期初'], ['SURPLUS', '盘盈'], ['SHORTAGE', '盘亏'], ['DAMAGE', '报损'], ['OTHER', '其他']],
};
const routes = ['dashboard', 'business/products', 'business/partners', 'business/warehouse', 'business/inventory-adjustments', 'business/inventory-balances', 'business/inventory-entries', 'business/print-profile', 'business/purchases', 'business/purchases/new', 'business/purchases/1/edit', 'business/purchase-returns', 'business/sale-returns', 'business/sales', 'business/sales/new', 'business/sales/1/edit', 'business/sales/1/delivery-note', 'business/partner-balances', 'business/settlements', 'business/refunds', 'business/opening-balances', 'business/partner-ledger', 'business/partner-statements?partnerId=1&direction=CUSTOMER&from=2026-10-01&to=2026-10-03'];
let browser, writeMode = 'fail', releaseWrite, searchMode = 'normal', balanceAmount = '100.00';
const errors = [], writes = [];
async function visit(page, route) {
  await page.goto(`${base}/${route}${route.includes('?') ? '&' : '?'}returnTo=${encodeURIComponent('/business/purchases?status=DRAFT')}`);
  await page.locator('main h1').first().waitFor();
}
async function goSource(page) { await page.getByRole('link', { name: '返回来源页面', exact: true }).first().click(); }
async function directReturn(page) { await goSource(page); await page.waitForURL(`${base}/business/purchases?status=DRAFT`); assert.equal(await page.getByRole('alertdialog').count(), 0); }
async function prompt(page, stay = '继续编辑') {
  const dialog = page.getByRole('alertdialog'); await dialog.waitFor();
  // Check before clicks: Playwright's automatic scrolling must not conceal this bug.
  assert.ok(await dialog.evaluate(n => { const r = n.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight && r.left >= 0 && r.right <= innerWidth; }), 'leave prompt must already be in viewport');
  const button = dialog.getByRole('button', { name: stay, exact: true }); await button.waitFor();
  await page.waitForFunction(stay => document.activeElement?.textContent === stay, stay, { timeout: 1500 }).catch(async () => { throw new Error(`Unexpected focus: ${JSON.stringify(await page.evaluate(() => ({ tag: document.activeElement?.tagName, label: document.activeElement?.getAttribute('aria-label') })))}`); });
  assert.ok(await button.evaluate(n => document.activeElement === n), 'default focus must preserve input'); return dialog;
}
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch { /* Vite starting */ } await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, timezoneId: 'Asia/Shanghai' });
  await context.addInitScript(() => { if (location.origin === 'http://127.0.0.1:4199') localStorage.setItem('web-auth', JSON.stringify({ token: 'business-ui-fixture', user: null })); });
  context.on('page', page => page.on('pageerror', e => errors.push(e.message)));
  await context.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    let data = records([]);
    if (route.request().method() !== 'GET') {
      writes.push(path);
      if (writeMode === 'hold') await new Promise(resolve => { releaseWrite = resolve; });
      if (writeMode === 'fail') return route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ code: 400, message: '回归保存失败', data: null }) });
      data = path.includes('print-profile') ? route.request().postDataJSON() : path.includes('products') ? product : path.includes('partners') ? partner : draft;
    } else if (path === '/api/auth/me') data = { id: 1, username: 'ui-regression', nickname: '回归', roles: [] };
    else if (path === '/api/auth/menus') data = [];
    else if (path.includes('/dict/')) data = (dicts[path.split('/').at(-2)] ?? []).map(([value, label], sortOrder) => ({ value, label, sortOrder }));
    else if (path.endsWith('/unread-count')) data = 0;
    else if (path === '/api/v1/products') {
      if (searchMode === 'fail') return route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ code: 500, message: '回归搜索失败' }) });
      if (searchMode === 'empty') data = records([]);
      else if (searchMode === 'many') { const page = Number(new URL(route.request().url()).searchParams.get('page') || 1); data = { ...records([{ ...product, id: page === 1 ? 1 : 11 }]), total: 24, page }; }
      else data = records([product, { ...product, id: 2, code: 'UI-G2' }]);
    }
    else if (path === '/api/v1/products/1') data = product;
    else if (path === '/api/v1/partners') data = records([partner, { ...partner, id: 2, code: 'UI-P2' }]);
    else if (path === '/api/v1/partners/1') data = partner;
    else if (/^\/api\/v1\/(purchases|sales)$/.test(path)) data = records([draft]);
    else if (/^\/api\/v1\/(purchases|sales)\/1$/.test(path)) data = draft;
    else if (path === '/api/v1/inventory/adjustments') data = records([adjustment]);
    else if (path === '/api/v1/inventory/adjustments/1') data = adjustment;
    else if (/^\/api\/v1\/(purchase-returns|sale-returns)$/.test(path)) data = records([returnDraft]);
    else if (/^\/api\/v1\/(purchase-returns|sale-returns)\/1$/.test(path)) data = returnDraft;
    else if (/\/(purchase-returns|sale-returns)\/source\//.test(path)) data = returnDraft;
    else if (path === '/api/v1/warehouse') data = { name: '回归仓库', remark: '' };
    else if (path === '/api/v1/print-profile') data = { name: '回归店铺', phone: '', address: '' };
    else if (path === '/api/v1/partner-balances') data = records([{ partnerId: 1, partnerCode: partner.code, partnerName: partner.name, direction: 'CUSTOMER', amount: balanceAmount }]);
    else if (path === '/api/v1/partner-balances/statement') data = { partnerId: 1, partnerName: partner.name, direction: 'CUSTOMER', from: '2026-10-01T00:00:00Z', to: '2026-10-03T00:00:00Z', records: [], openingAmount: '0.00', increaseAmount: '0.00', decreaseAmount: '0.00', netChange: '0.00', closingAmount: '0.00' };
    else if (path.endsWith('/delivery-note')) data = { ...draft, posted: false, ownerName: '回归店铺', ownerPhone: '', ownerAddress: '', totalQuantity: '1' };
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'OK', data }) });
  });
  let selectorCount = 0;
  for (const width of [1440, 1024]) {
    const page = await context.newPage(); await page.setViewportSize({ width, height: 1000 });
    for (const route of routes) {
      await visit(page, route);
      await page.locator('main').evaluate(async () => { await document.fonts.ready; });
      const controls = await page.locator('main select, main input:not([type="checkbox"]), main [role="group"] > div:first-child > button:first-child').evaluateAll(ns => ns.map(n => ({ tag: n.tagName, height: n.getBoundingClientRect().height, className: n.className })));
      for (const c of controls) if (!c.className.includes('h-control-sm')) {
        if (c.tag === 'BUTTON') assert.ok(c.height >= 36 && Math.abs((c.height - 36) % 20) < 1, `${width} ${route}: auto-height trigger ${c.height}`);
        else assert.ok(Math.abs(c.height - 36) < 1, `${width} ${route} ${c.tag}: height ${c.height}`);
      }
      for (const label of await page.locator('main [role="group"]').evaluateAll(ns => ns.map(n => n.getAttribute('aria-label')))) {
        const group = page.getByRole('group', { name: label, exact: true }); await group.getByRole('button').first().click();
        await group.locator('button[aria-pressed]').first().waitFor();
        const g = await group.evaluate(n => {
          const panel = n.querySelector('input').parentElement, box = panel.getBoundingClientRect(), next = n.querySelector('button[aria-label="下一页"]');
          return { overflow: panel.scrollWidth - panel.clientWidth, rightOverflow: next.getBoundingClientRect().right - box.right, summaryHeight: next.parentElement.parentElement.parentElement.firstElementChild.getBoundingClientRect().height };
        });
        assert.ok(g.overflow <= 1 && g.rightOverflow <= 1 && g.summaryHeight <= 24, `${width} ${route} ${label}: ${JSON.stringify(g)}`);
        await group.getByRole('button', { name: '收起搜索', exact: true }).click(); selectorCount++;
      }
      console.log('ROUTE', width, route);
    } await page.close();
  }
  const page = await context.newPage();
  // Selector zero/error/retry/multiple pages in 240px and 270px actual containers.
  for (const width of [240, 270]) {
    await visit(page, 'business/purchases');
    const group = page.getByRole('group', { name: '筛选商品', exact: true });
    await group.evaluate((n, width) => { n.style.width = `${width}px`; }, width);
    searchMode = 'empty'; await group.getByRole('button').first().click();
    await group.getByText('没有匹配资料', { exact: true }).waitFor();
    assert.ok(await group.getByRole('button', { name: '下一页', exact: true }).isDisabled());
    searchMode = 'fail'; await group.getByRole('textbox').fill('error');
    await group.getByRole('alert').filter({ hasText: '回归搜索失败' }).waitFor();
    assert.ok(await group.getByRole('button', { name: '下一页', exact: true }).isDisabled());
    searchMode = 'many'; await group.getByRole('button', { name: '重试搜索', exact: true }).click();
    await group.locator('button[aria-pressed]').first().waitFor();
    await group.getByRole('button', { name: '下一页', exact: true }).click();
    await group.getByText('11-20', { exact: true }).waitFor();
    assert.ok(await group.evaluate(n => n.scrollWidth <= n.clientWidth), 'compact pagination must stay within narrow container');
    await group.getByRole('button', { name: '上一页', exact: true }).click();
    await group.getByText('1-10', { exact: true }).waitFor();
    searchMode = 'normal';
  }
  for (const width of [1440, 1024]) for (const kind of ['purchases', 'sales']) for (const state of ['new', '1/edit']) {
    await page.setViewportSize({ width, height: 1000 });
    await visit(page, `business/${kind}/${state}`);
    const note = page.locator('main textarea').first(); await note.fill('未保存录单备注');
    await goSource(page); await (await prompt(page, '继续录单')).getByRole('button', { name: '继续录单', exact: true }).click();
    assert.equal(await note.inputValue(), '未保存录单备注');
    await page.getByRole('button', { name: /返回采购列表|返回销售列表|返回来源页面/ }).click();
    await prompt(page, '继续录单'); await page.getByRole('alertdialog').getByRole('button', { name: '确认离开', exact: true }).click();
    await page.waitForURL(`${base}/business/purchases?status=DRAFT`);
    console.log('TRADE_GUARD', width, kind, state);
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  for (const name of ['warehouse', 'print-profile', 'settlements', 'refunds', 'opening-balances']) {
    const route = `business/${name}`, stay = ['warehouse', 'print-profile'].includes(name) ? '继续编辑' : '继续填写';
    const ready = async () => { await visit(page, route); if (['warehouse', 'print-profile'].includes(name)) await page.waitForFunction(() => document.querySelector('main input')?.value.startsWith('回归')); };
    await ready();
    const input = page.locator('main input:not([type="date"]):not([type="checkbox"])').first(), initial = await input.inputValue();
    await directReturn(page); await ready(); await input.fill('13.25'); await input.fill(initial); await directReturn(page);
    await ready(); await input.fill('13.25'); await goSource(page);
    await (await prompt(page, stay)).getByRole('button', { name: stay, exact: true }).click(); assert.equal(await input.inputValue(), '13.25');
    await goSource(page); await prompt(page, stay); await page.getByRole('alertdialog').getByRole('button', { name: '放弃并离开', exact: true }).click();
    await page.waitForURL(`${base}/business/purchases?status=DRAFT`); console.log('PAGE_GUARD', name);
  }
  // Browser Back while the posting confirmation is open: leave prompt stays on top,
  // and Escape cancels only the leave decision, without discarding the posting preview.
  for (const kind of ['purchases', 'sales']) {
    await visit(page, `business/${kind}`);
    await page.getByRole('button', { name: '编辑', exact: true }).first().click();
    await page.locator('main textarea').first().fill('未保存过账备注');
    await page.getByRole('button', { name: '保存并过账', exact: true }).click();
    const posting = page.getByRole('dialog'); await posting.waitFor();
    const count = writes.length;
    await page.goBack();
    const warning = await prompt(page, '继续录单');
    assert.ok(await warning.getByRole('button', { name: '继续录单', exact: true }).evaluate(n => { const r = n.getBoundingClientRect(); return n.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)); }), 'leave decision must cover the posting preview');
    await page.keyboard.press('Escape'); await warning.waitFor({ state: 'hidden' });
    assert.equal(await posting.count(), 1); assert.equal(writes.length, count);
    await posting.getByRole('button', { name: '取消', exact: true }).click();
    await page.goBack(); await prompt(page, '继续录单');
    await page.getByRole('alertdialog').getByRole('button', { name: '确认离开', exact: true }).click();
    await page.waitForURL(url => url.pathname === `/business/${kind}`);
    console.log('POSTING_PREVIEW_LEAVE', kind);
  }
  const modals = [
    { route: 'products', create: '新建档案', field: 'input:not([type="date"]):not([type="checkbox"])' },
    { route: 'partners', create: '新建档案', field: 'input:not([type="date"]):not([type="checkbox"])' },
    { route: 'inventory-adjustments', create: '新建调整单', field: 'input[placeholder="例如 2 或 -0.125"]' },
    { route: 'purchase-returns', source: 'purchaseId', field: 'input:not([type="date"])' },
    { route: 'sale-returns', source: 'saleId', field: 'input:not([type="date"])' },
  ];
  for (const spec of modals) for (const editing of [false, true]) {
    const openEditor = async () => {
    await visit(page, `business/${spec.route}${!editing && spec.source ? `?${spec.source}=1` : ''}`);
    if (!editing && spec.create) await page.getByRole('button', { name: spec.create, exact: true }).click();
    if (editing && spec.route === 'inventory-adjustments') { await page.getByRole('button', { name: '查看', exact: true }).first().click(); await page.getByRole('button', { name: '编辑草稿', exact: true }).click(); }
    else if (editing) await page.getByRole('button', { name: '编辑', exact: true }).first().click();
    };
    await openEditor();
    const editor = page.getByRole('dialog', { name: /^(新建|编辑)/ }); await editor.waitFor();
    const field = editor.locator(spec.field).first(), initial = await field.inputValue(); await field.fill('777');
    for (const close of ['x', 'escape', 'overlay', 'cancel']) {
      if (close === 'x') await editor.getByRole('button', { name: '关闭表单弹窗', exact: true }).click();
      if (close === 'escape') await page.keyboard.press('Escape');
      if (close === 'overlay') await page.mouse.click(5, 5);
      if (close === 'cancel') await editor.getByRole('button', { name: '取消', exact: true }).click();
      console.log('CLOSE_PATH', spec.route, editing, close);
      const warning = await prompt(page);
      if (close === 'escape') await page.keyboard.press('Escape'); else await warning.getByRole('button', { name: '继续编辑', exact: true }).click();
      assert.equal(await field.inputValue(), '777'); assert.equal(await editor.count(), 1);
    }
    if (editing) {
      writeMode = 'fail';
      await editor.getByRole('button', { name: /^(保存|保存修改|保存草稿)$/ }).click();
      await editor.getByRole('alert').filter({ hasText: '回归保存失败' }).waitFor();
      assert.equal(await field.inputValue(), '777');
      writeMode = 'hold'; releaseWrite = undefined;
      await editor.getByRole('button', { name: /^(保存|保存修改|保存草稿)$/ }).click();
      for (let i = 0; !releaseWrite && i < 300; i++) await delay(10);
      assert.ok(releaseWrite, 'save request must reach fixture handler');
      assert.ok(await field.isDisabled(), 'editor inputs disabled during save');
      await page.keyboard.press('Escape'); assert.equal(await editor.count(), 1);
      writeMode = 'fail'; releaseWrite();
      await page.waitForFunction(() => !document.querySelector('[role="dialog"] fieldset:disabled'));
    }
    await field.fill(initial);
    if (spec.route === 'products' || spec.route === 'partners') { const optional = editor.getByLabel('备注', { exact: true }); await optional.fill('temporary'); await optional.fill(''); }
    await editor.getByRole('button', { name: '关闭表单弹窗', exact: true }).click(); await editor.waitFor({ state: 'hidden' });
    assert.equal(await page.getByRole('alertdialog').count(), 0);
    await openEditor(); await field.fill('discard');
    await editor.getByRole('button', { name: '关闭表单弹窗', exact: true }).click();
    await prompt(page); await page.getByRole('alertdialog').getByRole('button', { name: '放弃并关闭', exact: true }).click();
    await editor.waitFor({ state: 'hidden' });
    console.log('MODAL_GUARD', spec.route, editing ? 'edit' : 'new');
  }
  for (const name of ['settlements', 'refunds', 'opening-balances']) {
    balanceAmount = name === 'refunds' ? '-100.00' : '100.00';
    await visit(page, `business/${name}?partnerId=1`);
    const amount = page.getByLabel(name === 'opening-balances' ? '期初金额' : '资金金额', { exact: true });
    await amount.fill('13.25');
    if (name === 'opening-balances') await page.getByLabel('期初说明', { exact: true }).fill('历史欠款');
    const title = name === 'opening-balances' ? '确认录入期初' : name === 'refunds' ? '确认向客户退款' : '确认客户收款';
    const save = async () => {
      await page.getByRole('button', { name: title, exact: true }).click();
      await page.getByRole('dialog', { name: title, exact: true }).getByRole('button', { name: name === 'opening-balances' ? '保存并生效' : title, exact: true }).click();
    };
    writeMode = 'fail'; await save(); await page.getByRole('status').filter({ hasText: '回归保存失败' }).waitFor();
    await goSource(page); await (await prompt(page, '继续填写')).getByRole('button', { name: '继续填写', exact: true }).click();
    assert.equal(await amount.inputValue(), '13.25');
    writeMode = 'success'; await save();
    await page.waitForFunction(label => document.querySelector(`input[aria-label="${label}"]`)?.value === '', name === 'opening-balances' ? '期初金额' : '资金金额');
    await directReturn(page); console.log('FINANCE_SAVE_BASELINE', name);
  }
  writeMode = 'fail';
  await visit(page, 'business/warehouse'); await page.waitForFunction(() => document.querySelector('main input')?.value.startsWith('回归'));
  const warehouseName = page.locator('main input').first(); await warehouseName.fill('修改仓库');
  await page.getByRole('button', { name: /保存/ }).click(); await page.getByRole('alert').filter({ hasText: '回归保存失败' }).waitFor();
  await goSource(page); await (await prompt(page)).getByRole('button', { name: '继续编辑', exact: true }).click(); assert.equal(await warehouseName.inputValue(), '修改仓库');
  writeMode = 'hold'; await page.getByRole('button', { name: /保存/ }).click(); await page.waitForFunction(() => document.querySelector('main input')?.disabled);
  await goSource(page); const busyPrompt = page.getByRole('alertdialog'); await busyPrompt.waitFor(); assert.ok(await busyPrompt.getByRole('button', { name: '处理中...', exact: true }).isDisabled());
  for (let i = 0; !releaseWrite && i < 300; i++) await delay(10);
      assert.ok(releaseWrite, 'save request must reach fixture handler');
  writeMode = 'success'; releaseWrite(); await busyPrompt.getByRole('button', { name: '继续编辑', exact: true }).click(); await directReturn(page);
  await page.close(); assert.deepEqual(errors, []);
  console.log(`PASS business UI: ${routes.length} route states at 1440/1024, ${selectorCount} selectors, 5 page guards, 10 new/edit modal guards, all close paths, failed/busy saves and success baseline (${writes.length} fixture writes)`);
} finally { if (releaseWrite) releaseWrite(); await browser?.close(); vite.kill('SIGTERM'); }
