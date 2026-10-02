// Run only against an isolated, migrated API. Uses real public HTTP and Chromium.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const api = process.env.PHASE5_API_URL ?? "http://127.0.0.1:18109";
const base = "http://127.0.0.1:4190";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4190", "--strictPort"], { cwd: new URL("..", import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: "ignore" });
let token, browser;
const prefix = `D5-${Date.now()}`;
async function request(path, body, method = body === undefined ? "GET" : "POST", auth = token) {
  const response = await fetch(`${api}/api${path}`, { method, headers: { "Content-Type": "application/json", Authorization: auth ?? "" }, body: body === undefined ? undefined : JSON.stringify(body) });
  const envelope = await response.json(); assert.equal(response.status, 200, `${path}: ${envelope.message}`); return envelope.data;
}
async function select(page, label, code) {
  const group = page.getByRole("group", { name: label, exact: true });
  await group.getByRole("button").first().click();
  await group.getByRole("textbox", { name: `搜索${label}` }).fill(code);
  await group.locator('button[aria-pressed]').filter({ hasText: code }).last().click();
}
try {
  for (let n = 0; n < 60; n++) { try { if ((await fetch(base)).ok) break; } catch { /* starting */ } await delay(250); }
  token = (await request("/auth/login", { username: "admin", password: process.env.PHASE5_UI_PASSWORD ?? "admin123" })).tokenValue;
  const product = await request("/v1/products", { code: prefix, name: "可靠录单商品", type: "GOODS", unit: "台", purchasePrice: "2.50" });
  const partner = await request("/v1/partners", { code: prefix, name: "可靠录单供应商", type: "COMPANY", isSupplier: true });
  await request("/system/user", { username: prefix, nickname: "第二位操作人", deptId: 1, status: 1 });
  const otherToken = (await request("/auth/login", { username: prefix, password: "admin123" })).tokenValue;
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ timezoneId: "Asia/Shanghai", viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(value => localStorage.setItem("web-auth", JSON.stringify({ token: value, user: null })), token);
  const page = await context.newPage(); const errors = []; page.on("pageerror", e => errors.push(e.message));
  await page.goto(`${base}/business/purchases/new`);
  await select(page, "供应商", partner.code); await select(page, "第 1 行商品", product.code);
  await page.getByLabel("数量").fill("1.5");
  await page.getByRole("button", { name: "添加明细" }).click(); await select(page, "第 2 行商品", product.code);
  await page.getByLabel("数量").nth(1).fill("0.001");
  await page.getByLabel("成交单价（元）").nth(1).fill("5.00");
  await page.getByText("合计：¥3.76 · 2 行", { exact: true }).waitFor();
  await page.getByRole("button", { name: "返回采购列表" }).click();
  const leavePrompt = page.getByRole("alertdialog");
  assert.ok(await leavePrompt.evaluate(node => { const rect = node.getBoundingClientRect(); return rect.top >= 0 && rect.bottom <= innerHeight; }), "leave confirmation must be inside the viewport before any automatic scrolling");
  await leavePrompt.getByRole("button", { name: "继续录单" }).waitFor();
  await page.waitForFunction(() => document.activeElement?.textContent === "继续录单");
  assert.ok(await leavePrompt.getByRole("button", { name: "继续录单" }).evaluate(node => document.activeElement === node), "leave confirmation defaults to continuing the edit");
  await leavePrompt.getByRole("button", { name: "继续录单" }).click();
  assert.equal(await page.getByLabel("数量").first().inputValue(), "1.5");
  let created, createCalls = 0;
  await page.route("**/api/v1/purchases", async route => {
    if (route.request().method() !== "POST") return route.continue();
    createCalls++; const response = await route.fetch(); created = (await response.json()).data;
    await route.abort("failed"); // committed, but the client loses the response
  });
  await page.getByRole("button", { name: "保存草稿", exact: true }).click();
  await page.waitForURL(/\/business\/purchases\/\d+\/edit(?:\?.*)?$/);
  assert.equal(createCalls, 1); assert.equal(created.totalAmount, "3.76");
  assert.equal(created.items.length, 2); assert.equal(created.items[0].productId, created.items[1].productId);
  await page.getByText(/原保存版本 1/).waitFor();
  assert.equal(await page.evaluate(() => Object.keys(localStorage).filter(k => k.startsWith("simple-inventory:purchase-save:")).length), 0);
  await page.unroute("**/api/v1/purchases");
  await page.reload(); await page.getByText("合计：¥3.76 · 2 行", { exact: true }).waitFor();
  assert.equal(await page.getByLabel("数量").first().inputValue(), "1.5");

  // Another actual account edits the same original version.
  const original = await request(`/v1/purchases/${created.id}`);
  await page.getByLabel("数量").first().fill("7");
  const editBody = { partnerId: partner.id, businessDate: original.businessDate, directDelivery: original.directDelivery, items: original.items.map(i => ({ productId: i.productId, productType: i.productType, unit: i.unit, quantity: i.quantity, unitPrice: i.unitPrice })), version: original.version, requestKey: `${prefix}-other` };
  await request(`/v1/purchases/${created.id}`, editBody, "PUT", otherToken);
  await page.getByRole("button", { name: "保存草稿", exact: true }).click();
  await page.getByRole("button", { name: "重新加载最新单据" }).waitFor();
  assert.equal(await page.getByLabel("数量").first().inputValue(), "7");
  let confirmations = 0, beforeUnloadWarnings = 0;
  page.on("dialog", async d => { confirmations++; if (d.type() === "beforeunload") beforeUnloadWarnings++; await d.accept(); });
  await page.getByRole("button", { name: "重新加载最新单据" }).click();
  await page.getByText("版本 2 · 草稿，未改变库存与应付", { exact: true }).waitFor();
  assert.equal(confirmations, 1); assert.equal(await page.getByLabel("数量").first().inputValue(), "1.5");

  // Lost edit response remains provably successful after a subsequent posting.
  await page.getByLabel("数量").first().fill("2");
  await page.route(`**/api/v1/purchases/${created.id}`, async route => {
    if (route.request().method() !== "PUT") return route.continue();
    const response = await route.fetch(); const saved = (await response.json()).data;
    await request(`/v1/purchases/${saved.id}/post`, { version: saved.version }, "POST", otherToken);
    await route.abort("failed");
  });
  await page.getByRole("button", { name: "保存草稿", exact: true }).click();
  await page.getByText(/原保存版本 3 · 当前版本 4/).waitFor();
  await page.getByRole("status", { name: "操作结果" }).filter({ hasText: "已过账" }).waitFor();
  assert.equal(await page.getByRole("button", { name: "保存草稿", exact: true }).isDisabled(), true);
  await page.unroute(`**/api/v1/purchases/${created.id}`);
  await page.getByRole("button", { name: "返回采购列表" }).click();
  await page.waitForURL("**/business/purchases");

  // Request never arrives: refresh has only an account-scoped minimal key.
  await page.getByRole("button", { name: "新建采购单", exact: true }).click();
  await select(page, "供应商", partner.code); await select(page, "第 1 行商品", product.code);
  let lateBody;
  await page.route("**/api/v1/purchases", async route => { if (route.request().method() === "POST") { lateBody = route.request().postDataJSON(); await route.abort("failed"); } else await route.continue(); });
  await page.getByRole("button", { name: "保存草稿", exact: true }).click();
  await page.getByText(/保存结果尚未确认/).waitFor();
  const recovery = await page.evaluate(() => JSON.parse(localStorage.getItem(Object.keys(localStorage).find(k => k.startsWith("simple-inventory:purchase-save:")))));
  assert.deepEqual(Object.keys(recovery).sort(), ["id", "key", "operation", "version"]);
  await page.unroute("**/api/v1/purchases");
  await page.reload();
  await page.getByRole("button", { name: "核实并解除等待" }).click();
  await page.getByRole("status", { name: "操作结果" }).getByText("已核实原保存未提交，可以继续修改并保存。").waitFor();
  const late = await fetch(`${api}/api/v1/purchases`, { method: "POST", headers: { Authorization: token, "Content-Type": "application/json" }, body: JSON.stringify(lateBody) });
  assert.equal(late.status, 409);
  await page.goto(`${base}/business/purchases/${created.id}/edit`);
  await page.getByText(/单据已生效或取消，不能编辑/).waitFor();
  assert.equal(await page.getByRole("button", { name: "保存草稿", exact: true }).isDisabled(), true);
  assert.deepEqual(errors, []);
  assert.ok(beforeUnloadWarnings > 0, "refreshing an unconfirmed save must show the native leave warning");
  console.log("PASS real browser: full route/refresh, exact totals, duplicate product lines, leave guard, lost create/edit responses, two-account version conflict with retained input, later post, minimal recovery, missing-request fence and terminal edit refusal");
} finally { if (browser) await browser.close(); vite.kill(); }
