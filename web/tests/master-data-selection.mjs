// Core browser acceptance against an isolated API; setup uses public HTTP.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const api = process.env.SELECTION_API_URL ?? "http://127.0.0.1:18165";
const base = "http://127.0.0.1:4195", prefix = `PICK-${Date.now()}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4195", "--strictPort"], { cwd: new URL("..", import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: "ignore" });
let token, browser;
async function request(path, body, method = body === undefined ? "GET" : "POST") {
  const response = await fetch(`${api}/api${path}`, { method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: token } : {}) }, body: body === undefined ? undefined : JSON.stringify(body) });
  const result = await response.json(); assert.equal(response.status, 200, `${path}: ${result.message}`); assert.equal(result.code, 200, `${path}: ${result.message}`); return result.data;
}
try {
  for (let n = 0; n < 60; n++) { try { if ((await fetch(base)).ok) break; } catch {} await delay(250); }
  token = (await request("/auth/login", { username: "admin", password: process.env.SELECTION_PASSWORD ?? "admin123" })).tokenValue;
  const supplier = await request("/v1/partners", { code: `${prefix}-partner`, name: "选择验收单位", type: "COMPANY", isSupplier: true, isCustomer: true, contact: "原联系人", phone: "13800123456" });
  const second = await request("/v1/partners", { code: `${prefix}-second`, name: "换选单位", type: "COMPANY", isCustomer: true, contact: "新联系人" });
  const goods = await request("/v1/products", { code: `${prefix}-old`, name: "选择验收粉盒", type: "GOODS", unit: "支", model: "TN3028", purchasePrice: "2.50", salePrice: "5.00" });
  const twin = await request("/v1/products", { code: `${prefix}-twin`, name: "换选粉盒", type: "GOODS", unit: "盒", purchasePrice: "9.00" });
  for (let n = 0; n < 12; n++) await request("/v1/products", { code: `${prefix}-fill-${n}`, name: "分页验收商品", type: "GOODS", unit: "支" });
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(value => localStorage.setItem("web-auth", JSON.stringify({ token: value, user: null })), token);
  const page = await context.newPage(), errors = []; page.setDefaultTimeout(10000); page.on("pageerror", e => errors.push(e.message));
  const group = label => page.getByRole("group", { name: label, exact: true });
  async function choose(label, code) {
    const picker = group(label);
    await picker.getByRole("combobox").fill(code);
    await page.getByRole("option").filter({ has: page.getByText(code, { exact: true }) }).click();
    for (let attempt = 0; attempt < 100; attempt++) {
      if (await page.getByRole("button", { name: "取消换选", exact: true }).isVisible()) return;
      if (await picker.getByRole("combobox").inputValue() === "" && await picker.locator(`span[title*="${code}"]`).isVisible()) return;
      await delay(25);
    }
    assert.fail(`selection feedback missing for ${label}: ${code}`);
  }
  await page.goto(`${base}/business/purchases`); await page.getByRole("button", { name: "新建采购单", exact: true }).click();
  const product = group("第 1 行商品"), input = product.getByRole("combobox");
  await input.fill(prefix); await product.getByRole("option").first().waitFor(); assert.equal(await product.getByRole("option").count(), 6);
  await input.press("Enter"); assert.equal(await product.getByText(/指定第 1 行商品/).count(), 0);
  await product.getByRole("button", { name: "浏览选择", exact: true }).click();
  const modal = page.getByRole("dialog", { name: "选择第 1 行商品" }); await modal.getByRole("grid").getByRole("button").first().waitFor();
  await page.screenshot({ path: "/tmp/issue65-picker-modal.png", fullPage: true });
  await modal.getByRole("button", { name: "下一页", exact: true }).click(); await modal.getByText("11-14", { exact: true }).waitFor();
  await modal.getByRole("combobox").press("Escape"); await modal.waitFor({ state: "hidden" }); assert.equal(await product.getByRole("option").count(), 0);
  await input.fill("选择验收 TN3028"); await product.getByRole("option").filter({ hasText: goods.code }).waitFor(); await input.press("ArrowDown"); await input.press("Enter");
  await page.waitForFunction(() => document.activeElement?.id === "quantity-0"); assert.equal(await page.locator("#price-0").inputValue(), "2.50");
  await choose("供应商", supplier.code); await group("供应商").getByText("指定供应商：选择验收单位", { exact: true }).waitFor(); await page.locator("#quantity-0").fill("3"); await page.locator("#price-0").fill("7.00");
  await choose("第 1 行商品", goods.code); assert.equal(await page.locator("#price-0").inputValue(), "7.00");
  await choose("第 1 行商品", twin.code); await page.getByRole("button", { name: "取消换选", exact: true }).click(); assert.equal(await page.locator("#price-0").inputValue(), "7.00"); await product.getByText("指定第 1 行商品：选择验收粉盒", { exact: true }).waitFor();
  await choose("第 1 行商品", twin.code); await page.getByRole("button", { name: "保留成交价", exact: true }).click(); await product.locator(`span[title*="${twin.code}"]`).waitFor(); assert.equal(await page.locator("#quantity-0").inputValue(), "3"); assert.equal(await page.locator("#price-0").inputValue(), "7.00");
  const saved = page.waitForResponse(r => r.request().method() === "POST" && new URL(r.url()).pathname === "/api/v1/purchases"); await page.getByRole("button", { name: "保存草稿", exact: true }).click(); const draft = (await (await saved).json()).data;
  const persisted = await request(`/v1/purchases/${draft.id}`); assert.equal(persisted.items[0].productId, twin.id); assert.equal(persisted.items[0].unitPrice, "7.00"); assert.equal(Number(persisted.items[0].quantity), 3);
  await page.goto(`${base}/business/purchases`); const filter = group("筛选商品"); await filter.getByRole("combobox").fill("换选 粉盒"); await filter.getByRole("combobox").press("Enter"); await page.waitForURL(/productKeyword=/); assert.match(decodeURIComponent(page.url()), /productKeyword=换选\+粉盒/);
  await page.goto(`${base}/business/sales`); await page.getByRole("button", { name: "新建销售草稿", exact: true }).click(); await choose("客户", supplier.code); await page.waitForFunction(() => document.querySelector("#sale-contact")?.value === "原联系人"); await page.locator("#sale-contact").fill("");
  await choose("客户", second.code); await page.getByRole("button", { name: "保留送货资料", exact: true }).click(); await group("客户").locator(`span[title*="${second.code}"]`).waitFor(); assert.equal(await page.locator("#sale-contact").inputValue(), "");
  await choose("客户", supplier.code); await page.getByRole("button", { name: "取消换选", exact: true }).click(); await group("客户").getByText("指定客户：换选单位", { exact: true }).waitFor();
  const recent = await page.evaluate(() => Object.keys(localStorage).filter(k => k.startsWith("simple-inventory:recent:"))); assert.ok(recent.length >= 3, "recent preferences saved per account and scenario");
  assert.deepEqual(errors, []); await page.screenshot({ path: process.env.SELECTION_SCREENSHOT ?? "/tmp/issue65-picker.png", fullPage: true }); console.log("PASS: quick search, modal pagination/Escape, keyboard/focus, price/unit/cancel protection, keyword URL, delivery protection, recents and persisted draft");
} finally { await browser?.close(); vite.kill("SIGTERM"); }
