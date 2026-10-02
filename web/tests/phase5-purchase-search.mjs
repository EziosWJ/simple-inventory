// Isolated SQLite or PostgreSQL API required; setup and assertions use public APIs.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const api = process.env.PHASE5_API_URL ?? "http://127.0.0.1:18109";
const base = "http://127.0.0.1:4189";
const prefix = `S5-${Date.now()}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4189", "--strictPort"], {
  cwd: new URL("..", import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: "ignore",
});
let token;
async function request(path, body, method = body === undefined ? "GET" : "POST") {
  const response = await fetch(`${api}/api${path}`, {
    method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: token } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const envelope = await response.json();
  assert.equal(response.status, 200, `${method} ${path}: ${envelope.message}`);
  return envelope.data;
}
const group = (dialog, label) => dialog.getByRole("group", { name: label, exact: true });
async function search(selector, label, keyword) {
  if (await selector.getByRole("textbox", { name: `搜索${label}`, exact: true }).count() === 0)
    await selector.getByRole("button").first().click();
  await selector.getByRole("textbox", { name: `搜索${label}`, exact: true }).fill(keyword);
}
async function choose(selector, label, code) {
  await search(selector, label, code);
  await selector.getByRole("button").filter({ hasText: code }).last().click();
}
let browser;
try {
  for (let n = 0; n < 60; n++) {
    try { if ((await fetch(base)).ok) break; } catch { /* starting */ }
    await delay(250);
  }
  token = (await request("/auth/login", { username: "admin", password: process.env.PHASE5_UI_PASSWORD ?? "admin123" })).tokenValue;
  const goods = await request("/v1/products", { code: `${prefix}-old`, name: "同名商品", type: "GOODS", unit: "台", brand: "BrandX", model: `${prefix}-AbC%_!\\型号`, specification: "特有规格", purchasePrice: "2.50" });
  const twin = await request("/v1/products", { code: `${prefix}-twin`, name: "同名商品", type: "GOODS", unit: "盒", model: `${prefix}-AbC-anything` });
  const supplier = await request("/v1/partners", { code: `${prefix}-old`, name: "同名供应商", type: "COMPANY", isSupplier: true, isCustomer: true, contact: `${prefix}-AbC%_!\\`, phone: "13800123456" });
  const wrongProduct = await request("/v1/products", { code: `${prefix}-service`, name: "同名商品", type: "SERVICE", unit: "次", model: goods.model });
  const wrongPartner = await request("/v1/partners", { code: `${prefix}-customer`, name: "同名供应商", type: "PERSON", isSupplier: false, isCustomer: true, contact: supplier.contact });
  const inactive = await request("/v1/products", { code: `${prefix}-inactive`, name: "停用商品", type: "GOODS", unit: "台", model: goods.model });
  const inactivePartner = await request("/v1/partners", { code: `${prefix}-inactive`, name: "停用供应商", type: "COMPANY", isSupplier: true, contact: supplier.contact });
  await request(`/v1/products/${inactive.id}/status`, { status: 0 }, "PUT");
  await request(`/v1/partners/${inactivePartner.id}/status`, { status: 0 }, "PUT");
  // Create 501 newer choices: the target cannot occur in a first-500 catalog.
  for (let start = 0; start < 501; start += 10) await Promise.all(Array.from({ length: Math.min(10, 501 - start) }, async (_, n) => {
    const code = `${prefix}-fill-${start + n}`;
    await request("/v1/products", { code, name: "填充商品", type: "GOODS", unit: "台" });
    await request("/v1/partners", { code, name: "填充供应商", type: "COMPANY", isSupplier: true });
  }));
  console.log("Created >500 products/suppliers through public HTTP");
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ timezoneId: "Asia/Shanghai", viewport: { width: 1400, height: 1000 } });
  await context.addInitScript(value => localStorage.setItem("web-auth", JSON.stringify({ token: value, user: null })), token);
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", e => errors.push(e.message));
  await page.goto(`${base}/business/purchases`);
  await page.getByRole("button", { name: "新建采购单", exact: true }).click();
  const dialog = page.getByRole("main");
  const productSelector = group(dialog, "第 1 行商品"), partnerSelector = group(dialog, "供应商");
  await search(productSelector, "第 1 行商品", prefix);
  await productSelector.getByText("条", { exact: false }).filter({ hasText: "503" }).waitFor();
  await productSelector.getByRole("button", { name: "下一页", exact: true }).click();
  await productSelector.getByText("11-20", { exact: true }).waitFor();
  await search(productSelector, "第 1 行商品", `${prefix}-abc%_!\\型号`);
  await productSelector.getByRole("button").filter({ hasText: goods.code }).last().waitFor();
  assert.equal(await productSelector.getByRole("button").filter({ hasText: wrongProduct.code }).count(), 0);
  assert.equal(await productSelector.getByRole("button").filter({ hasText: inactive.code }).count(), 0);
  await choose(productSelector, "第 1 行商品", goods.code);
  await search(productSelector, "第 1 行商品", twin.code);
  await productSelector.getByRole("button").filter({ hasText: twin.code }).waitFor();
  assert.match(await productSelector.getByRole("button").first().innerText(), /同名商品/);
  await productSelector.getByRole("button", { name: "收起搜索" }).click();
  await search(partnerSelector, "供应商", `${prefix}-aBc%_!\\`);
  await partnerSelector.getByRole("button").filter({ hasText: supplier.code }).last().waitFor();
  assert.equal(await partnerSelector.getByRole("button").filter({ hasText: wrongPartner.code }).count(), 0);
  assert.equal(await partnerSelector.getByRole("button").filter({ hasText: inactivePartner.code }).count(), 0);
  await choose(partnerSelector, "供应商", supplier.code);
  await search(partnerSelector, "供应商", "001234");
  await partnerSelector.getByRole("button").filter({ hasText: supplier.code }).last().waitFor();
  await partnerSelector.getByRole("button", { name: "收起搜索" }).click();
  await search(partnerSelector, "供应商", prefix);
  await partnerSelector.getByText("条", { exact: false }).filter({ hasText: "502" }).waitFor();
  await partnerSelector.getByRole("button", { name: "下一页", exact: true }).click();
  await partnerSelector.getByText("11-20", { exact: true }).waitFor();
  await partnerSelector.getByRole("button", { name: "收起搜索" }).click();

  // Delayed results from an obsolete query cannot replace the new results.
  let releaseOld;
  const oldDone = new Promise(resolve => { releaseOld = resolve; });
  await page.route("**/api/v1/products?**", async route => {
    if (new URL(route.request().url()).searchParams.get("keyword") === twin.code) {
      const response = await route.fetch(); await delay(1200); await route.fulfill({ response }); releaseOld();
    } else await route.continue();
  });
  const oldRequest = page.waitForRequest(r => r.url().includes("/api/v1/products?") && new URL(r.url()).searchParams.get("keyword") === twin.code);
  await search(productSelector, "第 1 行商品", twin.code);
  await oldRequest;
  await search(productSelector, "第 1 行商品", goods.code);
  await productSelector.getByRole("button").filter({ hasText: goods.code }).last().waitFor();
  await oldDone; await delay(100);
  assert.equal(await productSelector.getByRole("button").filter({ hasText: twin.code }).count(), 0);
  await page.unroute("**/api/v1/products?**");
  await search(productSelector, "第 1 行商品", "no-such-product");
  await productSelector.getByText("没有匹配资料").waitFor();
  let fail = true;
  await page.route("**/api/v1/products?**", async route => {
    if (fail) { fail = false; await route.abort("failed"); } else await route.continue();
  });
  await search(productSelector, "第 1 行商品", goods.code);
  await productSelector.getByRole("button", { name: "重试搜索" }).click();
  await productSelector.getByRole("button").filter({ hasText: goods.code }).last().waitFor();
  await page.unroute("**/api/v1/products?**");
  await productSelector.getByRole("textbox").press("Enter");
  assert.equal(await dialog.isVisible(), true);
  await productSelector.getByRole("textbox").press("Escape");
  assert.equal(await dialog.isVisible(), true);
  assert.equal(await productSelector.getByRole("textbox").count(), 0);
  assert.equal(await productSelector.getByRole("button").first().evaluate(node => document.activeElement === node), true);
  const createdResponse = page.waitForResponse(r => r.url().endsWith("/api/v1/purchases") && r.request().method() === "POST");
  await dialog.getByRole("button", { name: "保存草稿", exact: true }).click();
  const created = (await (await createdResponse).json()).data;
  await page.waitForURL(`**/business/purchases/${created.id}/edit*`);
  assert.equal(created.partnerId, supplier.id); assert.equal(created.items[0].productId, goods.id);
  assert.equal(created.items[0].unitPrice, "2.50");
  await page.getByRole("button", { name: "返回采购列表" }).click();
  const row = page.getByRole("row").filter({ hasText: created.documentNo });
  await row.getByRole("button", { name: "编辑", exact: true }).click();
  await productSelector.getByRole("button").filter({ hasText: goods.code }).waitFor();
  const editResponse = page.waitForResponse(r => r.url().endsWith(`/api/v1/purchases/${created.id}`) && r.request().method() === "PUT");
  await dialog.getByRole("button", { name: "保存草稿", exact: true }).click();
  await editResponse;
  await page.getByRole("button", { name: "返回采购列表" }).click();
  await row.getByRole("button", { name: "过账", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认过账", exact: true }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  assert.equal((await request(`/v1/purchases/${created.id}`)).status, "POSTED");

  // Both selections remain visible if their master data are disabled later.
  const historical = await request("/v1/purchases", { partnerId: supplier.id, businessDate: "2026-10-02", directDelivery: true, items: [{ productId: goods.id, productType: "GOODS", unit: "台", quantity: "1", unitPrice: "2.50" }] });
  await request(`/v1/products/${goods.id}/status`, { status: 0 }, "PUT");
  await request(`/v1/partners/${supplier.id}/status`, { status: 0 }, "PUT");
  await page.reload();
  await page.getByRole("row").filter({ hasText: historical.documentNo }).getByRole("button", { name: "编辑", exact: true }).click();
  await productSelector.getByText("已停用：保留草稿原选择，请更换启用商品后保存。").waitFor();
  await partnerSelector.getByText("已停用：保留草稿原选择，请更换启用往来单位后保存。").waitFor();
  assert.equal(await dialog.getByRole("checkbox").isChecked(), true);
  assert.equal((await request(`/v1/purchases/${historical.id}`)).items[0].productId, goods.id);
  await page.screenshot({ path: "/tmp/phase5-search-inactive.png", fullPage: true });
  assert.deepEqual(errors, []);
  console.log("PASS real browser: paging, >500, same names, literals/case, role/type/status, retained ID, stale responses, empty/error/retry, keyboard, save/edit/post/direct draft");
} finally {
  if (browser) await browser.close();
  vite.kill();
}
