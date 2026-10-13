import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const base = "http://127.0.0.1:4215";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4215", "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "ignore" });
const rows = values => ({ records: values, total: values.length, page: 1, pageSize: 10 });
const supplier = "直送供货方", customer = "直送收货客户";
const goods = { id: 1, code: "D-G", name: "激光打印机", type: "GOODS", productType: "GOODS", unit: "台", quantity: "2", purchasePrice: "100.00", salePrice: "150.00", purchaseAmount: "200.00", saleAmount: "300.00", productCode: "D-G", productName: "激光打印机" };
const service = { id: 2, code: "D-S", name: "安装调试", type: "SERVICE", productType: "SERVICE", unit: "次", quantity: "1", purchasePrice: "0.00", salePrice: "35.00", purchaseAmount: "0.00", saleAmount: "35.00", productCode: "D-S", productName: "安装调试" };
const traceDoc = (kind, id, documentNo, items, status = "POSTED") => ({ id, documentNo, kind, status, partnerId: kind === "PURCHASE" ? 8 : 9, partnerName: kind === "PURCHASE" ? supplier : customer, businessDate: "2026-10-12", createdByName: "测试用户", createTime: "2026-10-12T01:00:00Z", postedByName: status === "POSTED" ? "测试用户" : undefined, postedAt: status === "POSTED" ? "2026-10-12T02:00:00Z" : undefined, totalAmount: "300.00", items });
const trace = hasReturn => ({
  purchase: traceDoc("PURCHASE", 31, "P-DIRECT-31", [{ id: 311, productId: 1, productCode: goods.code, productName: goods.name, unit: goods.unit, quantity: goods.quantity, unitPrice: goods.purchasePrice, amount: goods.purchaseAmount }]),
  sales: [traceDoc("SALE", 32, "S-DIRECT-32", [{ id: 321, productId: 1, productCode: goods.code, productName: goods.name, unit: goods.unit, quantity: goods.quantity, unitPrice: goods.salePrice, amount: goods.saleAmount }])],
  saleReturns: hasReturn ? [traceDoc("SALE_RETURN", 33, "SR-DIRECT-33", [{ id: 331, productId: 1, originalItemId: 321, productCode: goods.code, productName: goods.name, unit: goods.unit, quantity: "1", unitPrice: goods.salePrice, amount: "150.00" }])] : [],
  purchaseReturns: [],
});
function makeOrder(id, status = "DRAFT", items = [goods, service]) {
  return { id, documentNo: `DIRECT-${id}`, status, version: 1, supplierId: 8, customerId: 9, supplierName: supplier, customerName: customer, businessDate: "2026-10-12", deliveryContact: "签收人甲", deliveryPhone: "13800000000", deliveryAddress: "测试路 88 号", remark: "已与客户核对交付安排", createdBy: 17, createdByName: "测试用户", createTime: "2026-10-12T01:00:00Z", purchaseTotal: "200.00", saleTotal: "335.00", purchaseId: null, saleId: null, postedAt: null, postedByName: "", cancelledAt: null, cancelledByName: "", cancelReason: null, activeSaleReturns: [], activePurchaseReturns: [], items: items.map((line, index) => ({ ...line, id: index + 1 })) };
}
const response = (data, status = 200) => ({ status, contentType: "application/json", body: JSON.stringify({ code: status, message: status === 200 ? "OK" : "mock failure", data }) });
let direct = makeOrder(41), writes = [], mode = "normal", postGate, browser;
const errors = [];
const returnRecord = { id: 71, documentNo: "SR-71", saleId: 32, saleNo: "S-DIRECT-32", partnerId: 9, partnerName: customer, businessDate: "2026-10-12", status: "POSTED", version: 1, createdBy: 17, createdByName: "测试用户", createTime: "2026-10-12T01:00:00Z", totalAmount: "150.00", items: [{ id: 711, saleItemId: 321, productId: 1, productCode: goods.code, productName: goods.name, unit: goods.unit, originalQuantity: "2", returnedQuantity: "1", remainingQuantity: "1", quantity: "1", unitPrice: "150.00", amount: "150.00", priorReturnAmount: "0.00" }], directTrace: trace(false) };
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch { /* starting */ } await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(() => localStorage.setItem("web-auth", JSON.stringify({ token: "direct-modal-mock", user: { id: 17, username: "mock", nickname: "Mock", roles: [] } })));
  await context.route("**/api/**", async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname;
    if (!path.startsWith("/api/")) return route.continue();
    if (req.method() !== "GET") {
      writes.push({ path, body: req.postDataJSON() });
      if (/\/api\/v1\/direct-orders\/\d+\/post$/.test(path)) {
        if (mode === "hold") { await new Promise(resolve => { postGate = resolve; }); }
        if (mode === "lost-posted") { direct = { ...direct, status: "POSTED", version: direct.version + 1, purchaseId: 501, saleId: 601, postedAt: "2026-10-12T02:00:00Z", postedByName: "测试用户" }; return route.abort("failed"); }
        if (mode === "lost-version") { direct = { ...direct, version: direct.version + 1 }; return route.abort("failed"); }
        direct = { ...direct, status: "POSTED", version: direct.version + 1, purchaseId: 501, saleId: 601, postedAt: "2026-10-12T02:00:00Z", postedByName: "测试用户" };
        mode = "normal";
        return route.fulfill(response(direct));
      }
      if (/\/api\/v1\/direct-orders\/\d+\/cancel$/.test(path)) { direct = { ...direct, status: "CANCELLED", version: direct.version + 1, cancelReason: req.postDataJSON().reason, cancelledAt: "2026-10-12T03:00:00Z", cancelledByName: "测试用户" }; return route.fulfill(response(direct)); }
      return route.fulfill(response(direct));
    }
    let data = rows([]);
    if (path === "/api/auth/me") data = { id: 17, username: "mock", nickname: "Mock", roles: [] };
    else if (path === "/api/auth/menus") data = [];
    else if (/\/api\/v1\/direct-orders\/\d+$/.test(path)) data = direct;
    else if (path === "/api/v1/sale-returns") data = rows([]);
    else if (path === "/api/v1/sale-returns/71") data = returnRecord;
    return route.fulfill(response(data));
  });
  const page = await context.newPage(); page.setDefaultTimeout(10000); page.on("pageerror", error => errors.push(error.message));
  const openDirect = async (id = direct.id) => { await page.goto(`${base}/business/direct-orders/${id}`); await page.getByRole("heading", { name: `直送详情 · DIRECT-${id}` }).waitFor(); };

  await openDirect();
  const overview = page.getByRole("region", { name: "单据概览" });
  await overview.getByText("¥200.00", { exact: true }).waitFor(); await overview.getByText("¥335.00", { exact: true }).waitFor();
  await page.getByRole("table").getByText("激光打印机").waitFor(); await page.getByText("服务项目不计采购金额，不产生采购入库或库存流水").waitFor();
  await page.getByText("草稿尚未生效，库存与双方往来余额均未变化。", { exact: true }).waitFor();
  assert.equal(await page.locator("details").filter({ hasText: "送货资料" }).getAttribute("open"), null, "delivery details start folded");
  assert.equal(await page.locator("details").filter({ hasText: "操作记录" }).getAttribute("open"), null, "operation records start folded");
  await page.screenshot({ path: "/tmp/issue90-direct-detail.png", fullPage: true });
  await page.getByRole("button", { name: "确认直送", exact: true }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByText("确认依据：客户已实际收到本次直送货物。").waitFor();
  await dialog.getByText(/服务不计采购金额/).waitFor(); await dialog.getByText(/库存净变化为零，但保留采购增加、销售减少的成对库存流水/).waitFor();
  await page.screenshot({ path: "/tmp/issue90-direct-confirm.png", fullPage: true });
  const beforeCancel = writes.length;
  await dialog.getByRole("button", { name: "返回核对", exact: true }).click();
  assert.equal(writes.length, beforeCancel, "canceling confirmation must not write");

  // Busy guard: a repeated submit while the first request is held sends one request.
  mode = "hold";
  await page.getByRole("button", { name: "确认直送", exact: true }).click(); dialog = page.getByRole("dialog");
  const postButton = dialog.getByRole("button", { name: "确认并同时过账", exact: true });
  await postButton.dblclick();
  await page.getByRole("status").filter({ hasText: "正在按同一事务确认直送单" }).waitFor();
  await delay(150);
  assert.equal(writes.filter(item => item.path.endsWith("/post")).length, 1);
  assert.ok(postGate, "the held post request should be waiting before release");
  postGate(); mode = "normal";
  await page.getByRole("status", { name: "操作结果" }).filter({ hasText: "直送已确认" }).waitFor();
  assert.equal(direct.status, "POSTED");
  assert.match(await page.getByRole("link", { name: "查看采购单" }).getAttribute("href"), /purchaseId=501/);
  assert.match(await page.getByRole("link", { name: "查看销售单" }).getAttribute("href"), /saleId=601/);
  assert.match(await page.getByRole("link", { name: "打印送货单" }).getAttribute("href"), /delivery-note/);
  assert.equal(await page.getByRole("link", { name: "编辑草稿" }).count(), 0);

  // With an effective return, cancellation is blocked and both return routes remain visible.
  direct = { ...direct, activeSaleReturns: [711] };
  await page.reload();
  await page.getByText(/存在生效退货，不能整笔取消/).waitFor();
  assert.equal(await page.getByRole("button", { name: "整笔取消直送", exact: true }).isDisabled(), true);
  assert.equal(await page.getByRole("link", { name: /客户退回店内：销售退货/ }).count(), 1);
  assert.equal(await page.getByRole("link", { name: /后续退供应商：采购退货/ }).count(), 1);

  // Unknown outcome posting is reconciled on the existing document ID after reload.
  direct = makeOrder(42); mode = "lost-posted";
  await openDirect(42); await page.getByRole("button", { name: "确认直送", exact: true }).click(); dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "确认并同时过账", exact: true }).click();
  await page.getByText("直送操作结果待核实，保留原版本，不另建单据。").waitFor();
  await page.reload(); await page.getByRole("button", { name: "核实直送操作结果", exact: true }).click();
  await page.getByRole("status", { name: "操作结果" }).filter({ hasText: "已核实直送操作结果" }).waitFor();
  assert.equal(direct.id, 42); assert.equal(direct.status, "POSTED");

  // A version change while the outcome is unknown stops retry using the old version.
  direct = makeOrder(43); mode = "lost-version";
  await openDirect(43); await page.getByRole("button", { name: "确认直送", exact: true }).click(); dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "确认并同时过账", exact: true }).click();
  await page.getByText("直送操作结果待核实，保留原版本，不另建单据。").waitFor();
  await page.reload(); await page.getByRole("button", { name: "核实直送操作结果", exact: true }).click();
  await page.getByRole("alert").filter({ hasText: "单据状态或版本已变化" }).waitFor();
  assert.equal(direct.status, "DRAFT"); assert.equal(direct.version, 2);
  assert.equal(await page.getByRole("button", { name: "按原版本重试" }).count(), 0);

  // A long confirmation remains usable in a short, narrow viewport.
  const longItems = Array.from({ length: 18 }, (_, index) => ({ ...goods, id: index + 1, productCode: `D-G-${index + 1}`, productName: `打印机配件长明细 ${index + 1}`, purchaseAmount: "200.00", saleAmount: "300.00" }));
  direct = { ...makeOrder(44, "DRAFT", longItems), purchaseTotal: "3600.00", saleTotal: "5400.00" };
  await page.setViewportSize({ width: 390, height: 640 });
  await openDirect(44);
  await page.getByRole("button", { name: "确认直送", exact: true }).click(); dialog = page.getByRole("dialog");
  await dialog.getByText("即将确认的直送明细").waitFor();
  await dialog.getByText("确认依据：客户已实际收到本次直送货物。").waitFor();
  await page.screenshot({ path: "/tmp/issue90-direct-confirm-narrow.png", fullPage: true });
  await dialog.getByRole("button", { name: "返回核对", exact: true }).click();

  // Existing trace keeps risk and required step controls visible while the full document tables fold.
  await page.goto(`${base}/business/sale-returns?saleReturnId=71`);
  await page.getByRole("alert").filter({ hasText: "直送采购退货过账前，须先办理并过账关联销售退货" }).waitFor();
  await page.getByRole("button", { name: /第一步：销售退货/ }).waitFor();
  const traceDisclosure = page.locator("details").filter({ hasText: "关联单据完整明细" });
  await traceDisclosure.waitFor();
  assert.equal(await traceDisclosure.getAttribute("open"), null, "full trace details start folded");
  await page.screenshot({ path: "/tmp/issue90-direct-trace-risk.png", fullPage: true });
  await page.getByRole("button", { name: /第一步：销售退货/ }).click();
  assert.match(page.url(), /\/business\/sale-returns\?saleId=32/);
  await page.goto(`${base}/business/sale-returns?saleReturnId=71`);
  returnRecord.directTrace = trace(true);
  await page.reload();
  await page.getByRole("button", { name: /第二步：采购退货/ }).waitFor();
  await page.getByText("关联单据完整明细").click();
  await page.getByRole("button", { name: "原销售 · S-DIRECT-32" }).waitFor();
  assert.deepEqual(errors, []);
  console.log("PASS isolated mock browser: direct detail identity/status/two totals/items/delivery/operation sections; service excluded from purchase effect; cancel preview writes nothing; double-click busy protection; same transaction success and navigation/print links; active return blocks cancellation; lost-response verification posted and changed-version stop; trace risk and required order remain visible while linked rows are folded");
} finally { await browser?.close(); vite.kill("SIGTERM"); }
