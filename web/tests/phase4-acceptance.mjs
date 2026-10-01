// Live acceptance against an already running API and web app; creates real test data.
// Run from the repository root. See phase4-acceptance.md for required environment.
import { chromium } from 'playwright';
import fs from 'node:fs';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
const OUT = process.env.ACCEPTANCE_OUTPUT_DIR || '.task/phase4-acceptance';
fs.mkdirSync(OUT, { recursive: true });
const API_URL = process.env.ACCEPTANCE_API_URL;
const WEB_URL = process.env.ACCEPTANCE_WEB_URL;
const USERNAME = process.env.ACCEPTANCE_USERNAME;
const PASSWORD = process.env.ACCEPTANCE_PASSWORD;
if (!API_URL || !WEB_URL || !USERNAME || !PASSWORD)
    throw Error('Set ACCEPTANCE_API_URL, ACCEPTANCE_WEB_URL, ACCEPTANCE_USERNAME and ACCEPTANCE_PASSWORD; use an isolated test database.');
const TODAY = new Date().toLocaleDateString('en-CA', { timeZone: 'Asia/Shanghai' });
const PERIOD_FROM = new Date(`${TODAY}T00:00:00+08:00`).toISOString();
const PERIOD_TO = new Date(Date.parse(PERIOD_FROM) + 86400000).toISOString();
const report = process.env.RESUME ? JSON.parse(fs.readFileSync(`${OUT}/browser-result.json`, 'utf8')) : { started: new Date().toISOString(), stages: [], consoleErrors: [], artifacts: [] };
delete report.failure;
function save() {
    fs.writeFileSync(`${OUT}/browser-result.json`, JSON.stringify(report, null, 2));
}
function quota() {
    if (!process.env.ACCEPTANCE_QUOTA_CHECK)
        return;
    const q = JSON.parse(execFileSync('python3', [process.env.ACCEPTANCE_QUOTA_CHECK], { encoding: 'utf8' }));
    report.quota = q;
    save();
    if (q.remaining_percent < 15 || q.stop_requested)
        throw Error('QUOTA_STOP');
}
const browser = await chromium.launch({ ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}), headless: true, args: ['--no-sandbox'] });
const context = await browser.newContext({ viewport: { width: 1440, height: 1080 }, locale: 'zh-CN', timezoneId: 'Asia/Shanghai' });
const page = await context.newPage();
page.setDefaultTimeout(12000);
let auth = '';
page.on('request', r => {
    if (r.headers().authorization)
        auth = r.headers().authorization;
});
page.on('pageerror', e => {
    report.consoleErrors.push(e.message);
    save();
});
page.on('dialog', d => d.accept(d.type() === 'prompt' ? '真实浏览器验收取消/冲销原因' : undefined));
async function api(path, method = 'GET', data) {
    const r = await context.request.fetch(API_URL + path, { method, headers: { Authorization: auth }, data });
    const j = await r.json();
    if (j.code !== 200)
        throw Error(`API ${path}: ${j.message}`);
    return j.data;
}
async function goto(path) {
    await page.goto(WEB_URL + path);
    await page.waitForLoadState('networkidle');
}
async function action(path, fn, method = 'POST') {
    const p = page.waitForResponse(r => r.url().includes(path) && r.request().method() === method);
    await fn();
    const r = await p;
    const j = await r.json();
    assert.equal(j.code, 200, `${path}: ${j.message}`);
    await page.waitForTimeout(180);
    return j.data;
}
async function shot(name) {
    await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true });
    report.artifacts.push(`${name}.png`);
}
async function stage(name, fn) {
    if (process.env.RESUME && report.stages.some(s => s.name === name)) {
        if (name === '真实登录与基础夹具') {
            await goto('/login');
            await page.getByLabel('用户名', { exact: true }).fill(USERNAME);
            await page.getByLabel('密码', { exact: true }).fill(PASSWORD);
            await page.getByRole('button', { name: '登 录' }).click();
            await page.waitForURL('**/dashboard');
            await page.waitForLoadState('networkidle');
        }
        ;
        return;
    }
    ;
    quota();
    await fn();
    report.stages.push({ name, passed: true, at: new Date().toISOString() });
    save();
    console.log('PASS ' + name);
}
let { goods, service, partner, openingPartner } = report.fixtures ?? {};
let { purchase, sale, sr, pr } = report.documents ?? {};
let { receipts = [], payments = [], refunds = [] } = report.funds ?? {};
async function purchaseUI(direct = false) {
    await goto('/business/purchases');
    await page.getByRole('button', { name: '新建采购单', exact: true }).click();
    let d = page.getByRole('dialog');
    await d.locator('select').nth(0).selectOption(String(partner.id));
    await d.locator('select').nth(1).selectOption(String(goods.id));
    await d.locator('input').nth(3).fill(direct ? '3' : '10');
    await d.locator('input').nth(4).fill('10.00');
    if (direct)
        await d.getByRole('checkbox').check();
    if (!direct) {
        await d.getByRole('button', { name: '添加明细' }).click();
        await d.locator('select').nth(2).selectOption(String(goods.id));
        await d.locator('input').nth(5).fill('2');
        await d.locator('input').nth(6).fill('0.00');
    }
    ;
    const p = await action('/api/v1/purchases', () => d.getByRole('button', { name: '保存', exact: true }).click());
    await page.getByRole('row').filter({ hasText: p.documentNo }).getByRole('button', { name: '过账', exact: true }).click();
    const posted = await action(`/purchases/${p.id}/post`, () => page.getByRole('button', { name: '确认过账', exact: true }).click());
    return posted;
}
async function saleUI(p = null) {
    if (p) {
        await goto(`/business/purchases?purchaseId=${p.id}`);
        await page.getByRole('button', { name: '关联新建直送销售' }).click();
        await page.waitForLoadState('networkidle');
    }
    else {
        await goto('/business/sales');
        await page.getByRole('button', { name: '新建销售草稿', exact: true }).click();
    }
    ;
    let d = page.getByRole('dialog');
    await d.locator('select').nth(p ? 1 : 0).selectOption(String(partner.id));
    if (p) {
        await d.getByLabel('成交单价').fill('20.00');
    }
    else {
        await d.locator('select').nth(1).selectOption(String(goods.id));
        await d.getByLabel('数量', { exact: true }).nth(0).fill('4');
        await d.getByLabel('成交单价').nth(0).fill('20.00');
        await d.getByRole('button', { name: '增加明细' }).click();
        await d.locator('select').nth(2).selectOption(String(goods.id));
        await d.getByLabel('数量', { exact: true }).nth(1).fill('2');
        await d.getByLabel('成交单价').nth(1).fill('25.00');
        await d.getByRole('button', { name: '增加明细' }).click();
        await d.locator('select').nth(3).selectOption(String(service.id));
        await d.getByLabel('成交单价').nth(2).fill('30.00');
    }
    ;
    const s = await action('/api/v1/sales', () => d.getByRole('button', { name: '保存', exact: true }).click());
    const posted = await action(`/sales/${s.id}/post`, () => page.getByRole('row').filter({ hasText: s.documentNo }).getByRole('button', { name: '过账', exact: true }).click());
    await page.getByRole('button', { name: '关闭', exact: true }).click();
    return posted;
}
async function funds(direction, amount, type = 'SETTLEMENT', pid = partner.id) {
    await goto(type === 'REFUND' ? '/business/refunds' : '/business/settlements');
    await page.getByLabel('资金方向', { exact: true }).selectOption(direction);
    await page.getByLabel('资金往来单位').selectOption(String(pid));
    await page.getByLabel('资金金额').fill(amount);
    await page.getByLabel('备注（选填）').fill('真实浏览器验收资金');
    await page.waitForTimeout(250);
    const operation = type === 'REFUND' ? direction === 'CUSTOMER' ? '向客户退款' : '收到供应商退款' : direction === 'CUSTOMER' ? '客户收款' : '供应商付款';
    await page.getByRole('button', { name: `确认${operation}` }).click();
    return action(`/partner-balances/${type === 'REFUND' ? 'refunds' : 'settlements'}`, () => page.getByRole('dialog').getByRole('button', { name: `确认${operation}` }).click());
}
async function returnUI(kind, source, qty) {
    const saleKind = kind === 'sale';
    if (saleKind) {
        await goto(`/business/sales?saleId=${source.id}`);
        await page.getByRole('dialog').getByRole('button', { name: '办理退货' }).click();
    }
    else {
        await goto('/business/purchases');
        await page.getByRole('row').filter({ hasText: source.documentNo }).getByRole('button', { name: '办理退货' }).click();
    }
    ;
    await page.waitForLoadState('networkidle');
    const d = page.getByRole('dialog');
    await d.locator('input').nth(2).fill(qty);
    const path = `/api/v1/${kind}-returns`;
    const ret = await action(path, () => d.getByRole('button', { name: '保存', exact: true }).click());
    return action(`/${kind}-returns/${ret.id}/post`, () => page.getByRole('row').filter({ hasText: ret.documentNo }).getByRole('button', { name: '过账', exact: true }).click());
}
async function cancelReturn(kind, ret) {
    await goto(`/business/${kind}-returns`);
    await page.getByRole('row').filter({ hasText: ret.documentNo }).getByRole('button', { name: '冲销取消' }).click();
    const d = page.getByRole('dialog');
    await d.locator('textarea').fill('真实浏览器取消已退款退货');
    await action(`/${kind}-returns/${ret.id}/cancel`, () => d.getByRole('button', { name: '确认取消' }).click());
}
async function reverseFunds(entry, pid = partner.id) {
    const path = entry.entryType === 'OPENING' ? '/business/opening-balances' : ['CUSTOMER_REFUND', 'SUPPLIER_REFUND'].includes(entry.entryType) ? '/business/refunds' : '/business/settlements';
    await goto(`${path}?partnerId=${pid}&direction=${entry.direction}`);
    await page.waitForTimeout(250);
    await page.getByRole('row').filter({ hasText: entry.documentNo }).getByRole('button', { name: '查看详情' }).click();
    await page.getByRole('dialog').getByRole('button', { name: '冲销此记录' }).click();
    await page.getByRole('dialog').getByRole('textbox', { name: '冲销原因（必填）' }).fill('真实浏览器验收取消/冲销原因');
    await action(`/entries/${entry.id}/reverse`, () => page.getByRole('dialog').getByRole('button', { name: '确认冲销' }).click());
}
try {
    await stage('真实登录与基础夹具', async () => {
        await goto('/login');
        await page.getByLabel('用户名', { exact: true }).fill(USERNAME);
        await page.getByLabel('密码', { exact: true }).fill(PASSWORD);
        await page.getByRole('button', { name: '登 录' }).click();
        await page.waitForURL('**/dashboard');
        await page.waitForLoadState('networkidle');
        assert.ok(auth);
        const suffix = Date.now();
        partner = await api('/api/v1/partners', 'POST', { code: `UI-${suffix}`, name: 'UI验收双向往来单位', type: 'COMPANY', isCustomer: true, isSupplier: true, contact: '验收原联系人', phone: '13800000000', address: '验收原送货地址' });
        openingPartner = await api('/api/v1/partners', 'POST', { code: `UI-O-${suffix}`, name: 'UI验收期初单位', type: 'PERSON', isCustomer: true, isSupplier: true });
        goods = await api('/api/v1/products', 'POST', { code: `UI-G-${suffix}`, name: 'UI验收实物商品', type: 'GOODS', unit: '台', purchasePrice: '10.00', salePrice: '20.00' });
        service = await api('/api/v1/products', 'POST', { code: `UI-S-${suffix}`, name: 'UI验收安装服务', type: 'SERVICE', unit: '次', salePrice: '30.00' });
        report.fixtures = { partner, openingPartner, goods, service };
        await shot('01-authenticated-menu');
    });
    await stage('业务侧栏五类目录展开搜索与原路由', async () => {
        const nav = page.getByRole('navigation', { name: '主导航' });
        const categories = [
            ['基础资料', ['商品与服务', '往来单位', '仓库', '经营者打印资料']],
            ['采购管理', ['采购入库', '采购退货']],
            ['销售管理', ['销售出库', '销售退货']],
            ['库存管理', ['当前库存', '库存流水', '库存调整']],
            ['往来管理', ['往来余额', '往来明细', '收付款', '退款', '期初录入']],
        ];
        for (const [category, leaves] of categories) {
            const directory = nav.getByRole('button', { name: category, exact: true });
            await directory.click();
            assert.equal(await directory.getAttribute('aria-expanded'), 'true', `${category} should expand`);
            for (const leaf of leaves)
                await nav.getByRole('link', { name: leaf, exact: true }).waitFor();
        }
        await nav.getByRole('link', { name: '销售出库', exact: true }).click();
        await page.waitForURL('**/business/sales');
        assert.ok((await page.locator('main').innerText()).includes('销售出库'));
        await goto('/business/inventory-balances');
        assert.equal(await nav.getByRole('button', { name: '库存管理', exact: true }).getAttribute('aria-expanded'), 'true');
        await page.getByRole('button', { name: '菜单搜索' }).first().click();
        await page.getByRole('combobox', { name: '搜索菜单' }).fill('当前库存');
        const result = page.getByRole('option').filter({ hasText: '当前库存' });
        await result.waitFor();
        assert.equal(await page.getByRole('status').filter({ hasText: /找到 1 个页面/ }).count(), 1);
        await page.getByRole('button', { name: '关闭菜单搜索' }).click();
        await shot('01a-business-menu-groups');
    });
    await stage('UI普通采购销售同商品分价零价服务', async () => {
        purchase = await purchaseUI();
        assert.equal(purchase.totalAmount, '100.00');
        sale = await saleUI();
        assert.equal(sale.totalAmount, '160.00');
        report.documents = { purchase, sale };
        await shot('02-sale-posted');
    });
    await stage('UI分次收付款部分退货退款', async () => {
        receipts.push(await funds('CUSTOMER', '60.00'), await funds('CUSTOMER', '100.00'));
        payments.push(await funds('SUPPLIER', '40.00'), await funds('SUPPLIER', '60.00'));
        sr = await returnUI('sale', sale, '2');
        assert.equal(sr.totalAmount, '40.00');
        refunds.push(await funds('CUSTOMER', '40.00', 'REFUND'));
        pr = await returnUI('purchase', purchase, '2');
        assert.equal(pr.totalAmount, '20.00');
        refunds.push(await funds('SUPPLIER', '20.00', 'REFUND'));
        report.documents = { ...report.documents, sr, pr };
        report.funds = { receipts, payments, refunds };
        await shot('03-refund-records');
    });
    await stage('UI已退款退货取消新欠款原单取消待退款资金冲销', async () => {
        await cancelReturn('sale', sr);
        await cancelReturn('purchase', pr);
        const b = await api(`/api/v1/partner-balances?partnerId=${partner.id}&page=1&pageSize=20`);
        assert.ok(b.records.some(x => x.direction === 'CUSTOMER' && x.amount === '40.00'));
        assert.ok(b.records.some(x => x.direction === 'SUPPLIER' && x.amount === '20.00'));
        await goto('/business/sales');
        await action(`/sales/${sale.id}/cancel`, () => page.getByRole('row').filter({ hasText: sale.documentNo }).getByRole('button', { name: '取消已过账单' }).click());
        await goto('/business/purchases');
        await page.getByRole('row').filter({ hasText: purchase.documentNo }).getByRole('button', { name: '整单取消', exact: true }).click();
        await page.getByRole('dialog').locator('textarea').fill('真实浏览器整单取消');
        await action(`/purchases/${purchase.id}/cancel`, () => page.getByRole('button', { name: '确认取消', exact: true }).click());
        for (const e of [...refunds, ...receipts, ...payments])
            await reverseFunds(e);
        const after = await api(`/api/v1/partner-balances?partnerId=${partner.id}&page=1&pageSize=20`);
        assert.ok(after.records.every(x => x.amount === '0.00'));
        report.finalOrdinaryBalances = after;
        await shot('04-reversals-retained');
    });
    await stage('UI直送分别过账关联导航双向退货', async () => {
        const p = await purchaseUI(true);
        const s = await saleUI(p);
        const rs = await returnUI('sale', s, '1');
        const rp = await returnUI('purchase', p, '1');
        await goto(`/business/sales?saleId=${s.id}`);
        await page.getByRole('dialog').getByRole('button', { name: new RegExp(p.documentNo) }).first().click();
        await page.waitForURL(`**/business/purchases?purchaseId=${p.id}`);
        await page.getByRole('dialog').getByRole('button', { name: '查看供应商往来' }).click();
        await page.waitForURL('**/business/partner-ledger?*');
        report.direct = { p, s, rs, rp };
        await shot('05-direct-ledger-navigation');
    });
    await stage('UI期初分次录入结算冲销', async () => {
        openingPartner = await api('/api/v1/partners', 'POST', { code: `UI-O-${Date.now()}`, name: 'UI验收期初单位', type: 'PERSON', isCustomer: true, isSupplier: true });
        report.fixtures.openingPartner = openingPartner;
        const openings = [];
        for (const amount of ['100.00', '50.00']) {
            await goto('/business/opening-balances');
            await page.getByLabel('期初往来单位', { exact: true }).selectOption(String(openingPartner.id));
            await page.getByLabel('期初金额').fill(amount);
            await page.getByLabel('期初说明', { exact: true }).fill('分次期初验收');
            await page.getByRole('button', { name: '确认录入期初' }).click();
            openings.push(await action('/partner-balances/opening', () => page.getByRole('dialog').getByRole('button', { name: '保存并生效' }).click()));
        }
        ;
        const settlements = [await funds('CUSTOMER', '60.00', 'SETTLEMENT', openingPartner.id), await funds('CUSTOMER', '90.00', 'SETTLEMENT', openingPartner.id)];
        await goto(`/business/opening-balances?partnerId=${openingPartner.id}&direction=CUSTOMER`);
        const rejected = page.waitForResponse(r => r.url().includes(`/entries/${openings[0].id}/reverse`) && r.request().method() === 'POST');
        await page.getByRole('row').filter({ hasText: openings[0].documentNo }).getByRole('button', { name: '查看详情' }).click();
        await page.getByRole('dialog').getByRole('button', { name: '冲销此记录' }).click();
        await page.getByRole('dialog').getByRole('textbox', { name: '冲销原因（必填）' }).fill('金额录错');
        await page.getByRole('dialog').getByRole('button', { name: '确认冲销' }).click();
        assert.equal((await (await rejected).json()).code, 409);
        for (const e of [...settlements, ...openings])
            await reverseFunds(e, openingPartner.id);
        const b = await api(`/api/v1/partner-balances?partnerId=${openingPartner.id}&direction=CUSTOMER`);
        assert.equal(b.records[0].amount, '0.00');
        report.opening = { openings, settlements, b };
        await shot('06-opening-reversals');
    });
    await stage('UI当前库存内部ID预筛选各来源原始冲销详情往来与Phase3旧快照', async () => {
        await goto('/business/inventory-balances');
        await page.getByPlaceholder('编码、名称、型号或规格').fill(goods.code);
        await page.getByLabel('库存范围').selectOption('all');
        await page.getByRole('button', { name: '查询', exact: true }).click();
        await page.getByRole('row').filter({ hasText: goods.code }).getByRole('button', { name: '查看流水' }).click();
        await page.waitForURL(`**/business/inventory-entries?productId=${goods.id}`);
        assert.equal(await page.getByLabel('按商品筛选').inputValue(), String(goods.id));
        for (const source of ['PURCHASE', 'SALE', 'PURCHASE_RETURN', 'SALE_RETURN'])
            for (const type of ['ORIGINAL', 'REVERSAL']) {
                await goto(`/business/inventory-entries?productId=${goods.id}`);
                await page.getByLabel('业务来源').selectOption(source);
                await page.getByLabel('流水类型').selectOption(type);
                await page.getByRole('button', { name: '查询', exact: true }).click();
                await page.waitForTimeout(250);
                const e = await api(`/api/v1/inventory/entries?productId=${goods.id}&sourceType=${source}&entryType=${type}&pageSize=50`);
                assert.ok(e.total > 0);
                await page.getByRole('button', { name: e.records[0].documentNo, exact: true }).first().click();
                await page.getByRole('dialog').waitFor();
                const direction = ['PURCHASE', 'PURCHASE_RETURN'].includes(source) ? '供应商' : '客户';
                await page.getByRole('dialog').getByRole('button', { name: `查看${direction}往来` }).click();
                await page.waitForURL('**/business/partner-ledger?*');
                assert.ok(page.url().includes(`partnerId=${partner.id}`));
                assert.ok((await page.locator('body').innerText()).includes(e.records[0].documentNo));
            }
        ;
        if (process.env.ACCEPTANCE_LEGACY_ADJUSTMENT_ID) {
            const old = await api(`/api/v1/inventory/adjustments/${process.env.ACCEPTANCE_LEGACY_ADJUSTMENT_ID}`);
            await goto(`/business/inventory-entries?productId=${old.items[0].productId}`);
            await page.getByLabel('业务来源').selectOption('ADJUSTMENT');
            await page.getByRole('button', { name: '查询', exact: true }).click();
            await page.getByRole('button', { name: old.documentNo, exact: true }).first().click();
            const text = await page.getByRole('dialog').innerText();
            assert.ok(text.includes(old.items[0].productName));
            assert.ok(text.includes('- / -'));
            report.legacyAdjustment = old;
            await shot('07-phase3-legacy-snapshot');
        }
        ;
        const all = await api(`/api/v1/inventory/entries?productId=${goods.id}&pageSize=500`);
        const balance = await api(`/api/v1/inventory/balances?keyword=${goods.code}&stock=all`);
        const sum = all.records.reduce((a, e) => a + Math.round(Number(e.quantity) * 1000), 0);
        assert.equal(sum, Math.round(Number(balance.records[0].quantity) * 1000));
        report.inventoryInvariant = { sumMilli: sum, balance: balance.records[0], entryCount: all.total };
        assert.equal((await api(`/api/v1/inventory/entries?productId=${service.id}`)).total, 0);
    });
    await stage('真实已保存送货单金额显隐草稿水印快照重印多页A4', async () => {
        await api('/api/v1/print-profile', 'PUT', { name: '验收原经营者', phone: '13600000000', address: '验收原经营地址' });
        const lines = Array.from({ length: 40 }, (_, i) => ({ productId: service.id, productType: 'SERVICE', unit: '次', quantity: '1.000', unitPrice: `${i + 1}.00`, remark: `验收明细${String(i + 1).padStart(2, '0')}` }));
        const draft = await api('/api/v1/sales', 'POST', { partnerId: partner.id, businessDate: TODAY, deliveryContact: '原始签收联系人', deliveryPhone: '13800000000', deliveryAddress: '原始签收地址', remark: '多页签收打印验收', items: lines });
        await goto(`/business/sales/${draft.id}/delivery-note`);
        await page.locator('.delivery-page').nth(2).waitFor();
        assert.equal(await page.locator('.delivery-page').count(), 3);
        assert.equal(await page.locator('.delivery-watermark').count(), 3);
        await page.pdf({ path: `${OUT}/delivery-draft-visible.pdf`, format: 'A4', preferCSSPageSize: true, printBackground: true });
        await page.getByRole('button', { name: '隐藏金额' }).click();
        assert.equal(await page.locator('.delivery-table th').filter({ hasText: '金额' }).count(), 0);
        assert.ok(!(await page.locator('.delivery-note-print').innerText()).includes('合计金额'));
        await page.pdf({ path: `${OUT}/delivery-draft-hidden.pdf`, format: 'A4', preferCSSPageSize: true, printBackground: true });
        const posted = await api(`/api/v1/sales/${draft.id}/post`, 'POST', { version: draft.version });
        const before = await api(`/api/v1/sales/${draft.id}/delivery-note`);
        await api(`/api/v1/partners/${partner.id}`, 'PUT', { ...partner, name: '验收已改客户名称', contact: '验收已改联系人', address: '验收已改地址' });
        await api(`/api/v1/products/${service.id}`, 'PUT', { ...service, name: '验收已改服务名称', model: '后补型号', specification: '后补规格' });
        await api('/api/v1/print-profile', 'PUT', { name: '验收已改经营者', phone: '13700000000', address: '验收已改经营地址' });
        const after = await api(`/api/v1/sales/${draft.id}/delivery-note`);
        assert.deepEqual(after, before);
        await goto(`/business/sales/${draft.id}/delivery-note`);
        await page.locator('.delivery-page').nth(2).waitFor();
        assert.equal(await page.locator('.delivery-watermark').count(), 0);
        assert.ok((await page.locator('.delivery-note-print').innerText()).includes('验收原经营者'));
        await page.pdf({ path: `${OUT}/delivery-posted-snapshot.pdf`, format: 'A4', preferCSSPageSize: true, printBackground: true });
        await shot('08-delivery-last-page');
        report.printDelivery = { id: draft.id, documentNo: draft.documentNo, rows: 40, previewPages: 3, snapshotUnchanged: true, total: posted.totalAmount };
        report.artifacts.push('delivery-draft-visible.pdf', 'delivery-draft-hidden.pdf', 'delivery-posted-snapshot.pdf');
    });
    await stage('往来超过20条跨页完整期间含冲销四页A4末页合计', async () => {
        const entries = [];
        for (let i = 0; i < 40; i++)
            entries.push(await api('/api/v1/partner-balances/opening', 'POST', { requestKey: crypto.randomUUID(), partnerId: openingPartner.id, direction: 'SUPPLIER', amount: '10.00', businessDate: '2026-08-01', description: `多页完整期间验收${i + 1}` }));
        for (let i = 0; i < 3; i++)
            await api(`/api/v1/partner-balances/entries/${entries[i].id}/reverse`, 'POST', { reason: '跨业务日期实际发生期冲销验收' });
        await goto(`/business/partner-ledger?partnerId=${openingPartner.id}&direction=SUPPLIER`);
        await page.getByLabel('生效开始日', { exact: true }).fill(TODAY);
        await page.getByLabel('生效结束日（含当日）').fill(TODAY);
        await page.getByRole('button', { name: '查询', exact: true }).click();
        await page.waitForTimeout(200);
        assert.equal(await page.locator('table').last().locator('tbody tr').count(), 20);
        await page.getByRole('button', { name: '下一页', exact: true }).last().click();
        await page.waitForTimeout(200);
        assert.equal(await page.locator('table').last().locator('tbody tr').count(), 20);
        await shot('09-ledger-second-page');
        await page.getByRole('button', { name: '打印期间对账单' }).click();
        await page.locator('.statement-page').nth(3).waitFor();
        assert.equal(await page.locator('.statement-page').count(), 4);
        assert.equal(await page.locator('.statement-table tbody tr').count(), 43);
        assert.ok((await page.locator('.statement-totals').innerText()).includes('期末余额：370.00'));
        await page.pdf({ path: `${OUT}/statement-43-entries.pdf`, format: 'A4', preferCSSPageSize: true, printBackground: true });
        await shot('10-statement-fourth-page');
        report.printStatement = { entries: 43, previewPages: 4, opening: '0.00', closing: '370.00', reversals: 3 };
        report.artifacts.push('statement-43-entries.pdf');
    });
    await stage('修复后真实PDF页数页码末页签收合计与页面边界', async () => {
        await goto(`/business/sales/${report.printDelivery.id}/delivery-note`);
        await page.locator('.delivery-page').nth(2).waitFor();
        await page.pdf({ path: `${OUT}/delivery-posted-snapshot.pdf`, preferCSSPageSize: true, printBackground: true });
        const product = await api(`/api/v1/products/${service.id}`);
        const draft = await api('/api/v1/sales', 'POST', { partnerId: partner.id, businessDate: TODAY, items: Array.from({ length: 40 }, (_, i) => ({ productId: service.id, productType: 'SERVICE', unit: product.unit, quantity: '1', unitPrice: `${i + 1}.00`, remark: `验收明细${i + 1}` })) });
        await goto(`/business/sales/${draft.id}/delivery-note`);
        await page.locator('.delivery-page').nth(2).waitFor();
        await page.pdf({ path: `${OUT}/delivery-draft-visible.pdf`, preferCSSPageSize: true, printBackground: true });
        await page.getByRole('button', { name: '隐藏金额' }).click();
        await page.pdf({ path: `${OUT}/delivery-draft-hidden.pdf`, preferCSSPageSize: true, printBackground: true });
        await goto(`/business/partner-statements?partnerId=${openingPartner.id}&direction=SUPPLIER&from=${encodeURIComponent(PERIOD_FROM)}&to=${encodeURIComponent(PERIOD_TO)}`);
        await page.locator('.statement-page').nth(3).waitFor();
        await page.pdf({ path: `${OUT}/statement-43-entries.pdf`, preferCSSPageSize: true, printBackground: true });
        execFileSync('python3', [new URL('./phase4-print-pdf.py', import.meta.url).pathname, OUT], { stdio: 'inherit' });
        report.pdfValidation = JSON.parse(fs.readFileSync(`${OUT}/pdf-result.json`, 'utf8'));
    });
    assert.deepEqual(report.consoleErrors, [], 'Unexpected browser page errors');
    report.completed = new Date().toISOString();
    save();
    console.log('ALL_BROWSER_STAGES_PASS');
}
catch (e) {
    report.failure = { message: e.message, stack: e.stack, url: page.url() };
    await shot('failure');
    save();
    console.error(e.message);
    process.exitCode = 1;
}
finally {
    await browser.close();
    save();
}
