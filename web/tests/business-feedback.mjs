import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

// Requires an isolated API: this suite creates and posts real business data.
const api = process.env.FEEDBACK_API_URL;
if (!api) throw new Error("Set FEEDBACK_API_URL to an isolated test API");
const base = "http://127.0.0.1:4197", prefix = `FB-${Date.now()}`;
const output = process.env.FEEDBACK_OUTPUT_DIR ?? "/tmp/business-feedback-artifacts";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4197", "--strictPort"], {
  cwd: new URL("..", import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: "ignore",
});
let token, browser, page;
async function call(path, body, method = body === undefined ? "GET" : "POST") {
  const response = await fetch(`${api}/api${path}`, { method, headers: { Authorization: token ?? "", "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
  const json = await response.json();
  assert.equal(response.status, 200, `${path}: ${json.message}`);
  return json.data;
}
try {
  for (let i = 0; i < 60; i++) { try { if ((await fetch(base)).ok) break; } catch { /* Vite starting */ } await delay(250); }
  token = (await call("/auth/login", { username: "admin", password: process.env.FEEDBACK_UI_PASSWORD ?? "admin123" })).tokenValue;
  const partner = await call("/v1/partners", { code: prefix, name: "反馈验收往来单位", type: "COMPANY", isSupplier: true, isCustomer: true });
  const product = await call("/v1/products", { code: prefix, name: "反馈验收商品", type: "GOODS", unit: "台", purchasePrice: "10.00", salePrice: "20.00" });
  const service = await call("/v1/products", { code: `${prefix}-S`, name: "反馈验收服务", type: "SERVICE", unit: "次", salePrice: "0.00" });
  const input = { partnerId: partner.id, businessDate: "2026-10-02", items: [{ productId: product.id, productType: "GOODS", unit: "台", quantity: "20", unitPrice: "10.00" }] };
  let purchase = await call("/v1/purchases", input);
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: "zh-CN", timezoneId: "Asia/Shanghai" });
  await context.addInitScript(t => localStorage.setItem("web-auth", JSON.stringify({ token: t, user: null })), token);
  page = await context.newPage(); const errors = [];
  page.setDefaultTimeout(12000);
  page.on("pageerror", e => errors.push(e.message));
  page.on("dialog", d => d.accept(d.type() === "prompt" ? "反馈验收取消" : undefined));
  const toast = () => page.getByRole("region", { name: "消息通知" });
  const result = () => page.locator('section[aria-label="操作结果"]');
  const row = no => page.getByRole("row").filter({ hasText: no });
  async function feedback(title, type = "success", description) {
    await toast().getByText(title, { exact: true }).waitFor();
    await result().filter({ hasText: title }).waitFor();
    assert.equal(await toast().getByText(title, { exact: true }).count(), 1, "one toast per operation");
    const card = result().filter({ hasText: title });
    assert.equal(await card.getAttribute("role"), type === "error" ? "alert" : "status");
    assert.match(await card.getAttribute("class"), new RegExp(`border-${type}`));
    if (description) assert.match(await card.innerText(), description);
  }
  async function action(path, trigger, method = "POST") {
    const pending = page.waitForResponse(r => r.url().includes(path) && r.request().method() === method);
    const [response] = await Promise.all([pending, trigger()]);
    const json = await response.json();
    assert.equal(response.status(), 200, `${path}: ${json.message}`);
    return json.data;
  }
  async function confirmPost() {
    await page.getByRole("button", { name: "保存并过账", exact: true }).click();
    await page.getByRole("dialog").getByRole("button", { name: "确认保存并过账", exact: true }).click();
  }
  await mkdir(output, { recursive: true });
  await page.goto(`${base}/business/purchases/${purchase.id}/edit`);
  await page.getByLabel("数量").waitFor();
  assert.equal(await toast().getByRole("status").count(), 0, "ordinary loading does not announce success");
  purchase = await action(`/purchases/${purchase.id}`, () => page.getByRole("button", { name: "保存草稿", exact: true }).click(), "PUT");
  await feedback("采购草稿保存成功", "success", /尚未过账/);
  await toast().getByText("采购草稿保存成功", { exact: true }).waitFor({ state: "detached", timeout: 4500 });
  await result().waitFor(); // Persistent result survives toast expiry.
  await confirmPost();
  await feedback("采购单过账成功", "success", /库存已增加；应付款已增加/);
  assert.equal(await toast().getByRole("status").count(), 1, "save-and-post has only the final success toast");
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: `${output}/purchase-posted.png`, fullPage: true });
  await toast().getByText("采购单过账成功", { exact: true }).waitFor({ state: "detached", timeout: 7500 });
  await result().getByRole("link", { name: "查看供应商往来" }).waitFor();
  await page.reload();
  await page.getByLabel("数量").waitFor();
  assert.equal(await toast().getByRole("status").count(), 0, "refresh does not repeat an operation notification");
  purchase = await call(`/v1/purchases/${purchase.id}`);

  const pureService = await call("/v1/sales", { ...input, items: [{ productId: service.id, productType: "SERVICE", unit: "次", quantity: "1", unitPrice: "0.00" }] });
  await page.goto(`${base}/business/sales/${pureService.id}/edit`);
  await page.getByLabel("数量").waitFor(); await confirmPost();
  await feedback("销售单过账成功", "success", /库存无变化；零金额，往来余额无变化/);
  assert.equal(await result().getByRole("link", { name: "查看库存流水" }).count(), 0);
  await toast().getByRole("button", { name: "关闭通知" }).click();
  assert.equal(await toast().getByRole("status").count(), 0);
  await result().waitFor();

  const sale = await call("/v1/sales", { ...input, items: [{ ...input.items[0], quantity: "3", unitPrice: "20.00" }] });
  await call(`/v1/sales/${sale.id}/post`, { version: sale.version });
  // Both returns exercise the actual edit/post/cancel page handlers.
  for (const [kind, origin, itemKey, label] of [["purchase", purchase, "purchaseItemId", "采购"], ["sale", sale, "saleItemId", "销售"]]) {
    let returned = await call(`/v1/${kind}-returns`, { [`${kind}Id`]: origin.id, businessDate: "2026-10-02", items: [{ [itemKey]: origin.items[0].id, quantity: "1" }] });
    await page.goto(`${base}/business/${kind}-returns`);
    await row(returned.documentNo).getByRole("button", { name: "编辑", exact: true }).click();
    const dialog = page.getByRole("dialog");
    returned = await action(`/${kind}-returns/${returned.id}`, () => dialog.getByRole("button", { name: "保存", exact: true }).click(), "PUT");
    await feedback(`${label}退货草稿保存成功`, "success", /尚未过账/);
    await action(`/${kind}-returns/${returned.id}/post`, () => row(returned.documentNo).getByRole("button", { name: "过账", exact: true }).click());
    await feedback(`${label}退货过账成功`);
    await row(returned.documentNo).getByRole("button", { name: "冲销取消", exact: true }).click();
    await dialog.locator("textarea").fill("反馈验收冲销");
    await action(`/${kind}-returns/${returned.id}/cancel`, () => dialog.getByRole("button", { name: "确认取消", exact: true }).click());
    await feedback(`${label}退货冲销取消成功`, "success", /已取消/);
  }

  await page.goto(`${base}/business/inventory-adjustments`);
  await page.getByRole("button", { name: "新建调整单" }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByPlaceholder("按编码、名称、型号或规格搜索商品").fill(product.code);
  await dialog.getByRole("button", { name: "搜索", exact: true }).click();
  await dialog.getByLabel("选择启用实物商品").selectOption(String(product.id));
  await dialog.getByLabel("调整数量（正数增加，负数减少）").fill("2");
  await dialog.getByLabel(/调整原因/).selectOption("SURPLUS");
  const adjustment = await action("/inventory/adjustments", () => dialog.getByRole("button", { name: "保存草稿", exact: true }).click());
  await feedback("库存调整草稿保存成功");
  await row(adjustment.documentNo).getByRole("button", { name: "过账", exact: true }).click();
  await dialog.getByRole("checkbox").check();
  await action(`/adjustments/${adjustment.id}/post`, () => dialog.getByRole("button", { name: "确认并过账" }).click());
  await feedback("库存调整单过账成功");
  // Success opens a detail dialog; the toast remains above it and can be closed.
  await toast().getByRole("button", { name: "关闭通知" }).click();
  await dialog.getByRole("button", { name: "取消并冲销" }).click();
  await dialog.locator("textarea").fill("反馈验收取消调整");
  await action(`/adjustments/${adjustment.id}/cancel`, () => dialog.getByRole("button", { name: "确认冲销并取消" }).click());
  await feedback("库存调整单冲销取消成功");
  await dialog.getByRole("button", { name: "关闭", exact: true }).click();
  await result().waitFor();

  // Period opening, receipts/payments, and reversing in both balance directions.
  for (const direction of ["CUSTOMER", "SUPPLIER"]) {
    await page.goto(`${base}/business/opening-balances?partnerId=${partner.id}&direction=${direction}`);
    await page.getByLabel("期初金额").fill("50");
    await page.getByLabel("期初说明").fill("反馈验收期初");
    await page.getByRole("button", { name: "确认录入期初", exact: true }).click();
    const opening = await action("/partner-balances/opening", () => page.getByRole("dialog").getByRole("button", { name: "保存并生效" }).click());
    await feedback(direction === "CUSTOMER" ? "期初应收录入成功" : "期初应付录入成功", "success", /方向余额/);
    await page.goto(`${base}/business/settlements?partnerId=${partner.id}&direction=${direction}`);
    await page.getByLabel("资金金额").fill("1");
    const operation = direction === "CUSTOMER" ? "客户收款" : "供应商付款";
    await page.getByRole("button", { name: `确认${operation}`, exact: true }).click();
    const settlement = await action("/partner-balances/settlements", () => page.getByRole("dialog").getByRole("button", { name: `确认${operation}`, exact: true }).click());
    await feedback(`${operation}成功`, "success", /方向余额/);
    await row(settlement.documentNo).getByRole("button", { name: "查看详情" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "冲销此记录" }).click();
    await page.getByRole("dialog").getByLabel("冲销原因（必填）").fill("反馈验收冲销往来");
    await action(`/entries/${settlement.id}/reverse`, () => page.getByRole("dialog").getByRole("button", { name: "确认冲销" }).click());
    await feedback("往来记录冲销成功", "success", /原记录和冲销记录均保留/);
    assert.equal((await call(`/v1/partner-balances/entries/${opening.id}`)).direction, direction);
  }
  // Create a negative balance through actual return business, then refund it.
  for (const [direction, kind, price, operation] of [["CUSTOMER", "sale", "200.00", "向客户退款"], ["SUPPLIER", "purchase", "300.00", "收到供应商退款"]]) {
    const funded = await call(`/v1/${kind}s`, { ...input, items: [{ ...input.items[0], quantity: "1", unitPrice: price }] });
    await call(`/v1/${kind}s/${funded.id}/post`, { version: funded.version });
    const balances = await call(`/v1/partner-balances?partnerId=${partner.id}&direction=${direction}&page=1&pageSize=1`);
    await call("/v1/partner-balances/settlements", { requestKey: `${prefix}-${direction}-settle`, partnerId: partner.id, direction, amount: balances.records[0].amount, businessDate: "2026-10-02", paymentMethod: "CASH", transactionNo: "", remark: "" });
    const returned = await call(`/v1/${kind}-returns`, { [`${kind}Id`]: funded.id, businessDate: "2026-10-02", items: [{ [`${kind}ItemId`]: funded.items[0].id, quantity: "1" }] });
    await call(`/v1/${kind}-returns/${returned.id}/post`, { version: returned.version });
    await page.goto(`${base}/business/refunds?partnerId=${partner.id}&direction=${direction}`);
    await page.getByLabel("资金金额").fill("1");
    await page.getByRole("button", { name: `确认${operation}`, exact: true }).click();
    await action("/partner-balances/refunds", () => page.getByRole("dialog").getByRole("button", { name: `确认${operation}`, exact: true }).click());
    await feedback(`${operation}成功`, "success", /方向余额/);
  }
  // Operation confirmed, refresh fails: the persistent success result remains truthful.
  await page.goto(`${base}/business/settlements?partnerId=${partner.id}&direction=CUSTOMER`);
  await call("/v1/partner-balances/opening", { requestKey: `${prefix}-extra-opening`, partnerId: partner.id, direction: "CUSTOMER", amount: "300.00", businessDate: "2026-10-02", description: "追加欠款" });
  await page.reload();
  await page.getByLabel("资金金额").fill("1");
  await page.getByRole("button", { name: "确认客户收款", exact: true }).click();
  await page.route("**/api/v1/partner-balances?*", route => route.abort());
  await action("/partner-balances/settlements", () => page.getByRole("dialog").getByRole("button", { name: "确认客户收款", exact: true }).click());
  await feedback("客户收款成功");
  await page.unroute("**/api/v1/partner-balances?*");

  // Failed return cancellation preserves reason and displays a clickable toast above the dialog.
  const failReturn = await call("/v1/purchase-returns", { purchaseId: purchase.id, businessDate: "2026-10-02", items: [{ purchaseItemId: purchase.items[0].id, quantity: "1" }] });
  await page.goto(`${base}/business/purchase-returns`);
  await row(failReturn.documentNo).getByRole("button", { name: "取消", exact: true }).click();
  dialog = page.getByRole("dialog"); await dialog.locator("textarea").fill("保留此原因");
  await page.route(`**/api/v1/purchase-returns/${failReturn.id}/cancel`, route => route.fulfill({ status: 409, contentType: "application/json", body: JSON.stringify({ code: 409, message: "单据版本已变化", data: null }) }));
  await dialog.getByRole("button", { name: "确认取消", exact: true }).click();
  await feedback("取消失败", "error", /单据版本已变化/);
  assert.equal(await dialog.locator("textarea").inputValue(), "保留此原因");
  await toast().getByRole("button", { name: "关闭通知" }).click();
  await page.screenshot({ path: `${output}/return-cancel-failed.png`, fullPage: true });
  await page.setViewportSize({ width: 640, height: 800 });
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  const card = result().filter({ hasText: "取消失败" });
  assert.ok((await card.boundingBox()).width <= 640, "long document feedback fits the viewport");
  assert.deepEqual(errors, []);
  console.log(`PASS business feedback: real saves/posts/returns/inventory/opening/settlements/refunds/reversals; expiry, dismissal, dialog stacking, persistent results, refresh failure. Screenshots: ${output}`);
} catch (error) {
  if (page) await page.screenshot({ path: `${output}/failure.png`, fullPage: true });
  throw error;
} finally {
  if (browser) await browser.close();
  vite.kill("SIGTERM");
}
