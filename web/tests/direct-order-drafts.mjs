import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const api = process.env.DIRECT_ORDER_API_URL ?? "http://127.0.0.1:18112";
const base = "http://127.0.0.1:4196", prefix = `DD-${Date.now()}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4196", "--strictPort"], { cwd: new URL("..", import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: "ignore" });
let token, browser, page;
async function call(path, body, method = body === undefined ? "GET" : "POST") {
  const res = await fetch(`${api}/api${path}`, { method, headers: { Authorization: token ?? "", "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
  const json = await res.json(); assert.equal(res.status, 200, `${path}: ${json.message}`); return json.data;
}
async function select(page, label, code) {
  const group = page.getByRole("group", { name: label, exact: true });
  await group.getByRole("combobox").fill(code);
  const option = page.getByRole("option").filter({ has: page.getByText(code, { exact: true }) });
  const name = await option.locator("strong").innerText();
  await option.click();
  await page.waitForFunction(({ label, name }) => [...document.querySelectorAll('[role="group"]')].find(g => g.getAttribute("aria-label") === label)?.querySelector('[role="combobox"]')?.value === name, { label, name });
}
try {
  for (let i = 0; i < 60; i++) { try { const res = await fetch(base); if (res.ok) break; } catch {} await delay(500); }
  token = (await call("/auth/login", { username: process.env.ACCEPTANCE_USERNAME ?? "admin", password: process.env.ACCEPTANCE_PASSWORD ?? "admin123" })).tokenValue;
  const partner = await call("/v1/partners", { code: prefix, name: "直送客户供应商", type: "COMPANY", isCustomer: true, isSupplier: true, contact: "收货人", phone: "123", address: "测试地址" });
  const goods = await call("/v1/products", { code: `${prefix}-G`, name: "直送商品", type: "GOODS", unit: "台", purchasePrice: "10.02", salePrice: "20.03" });
  const service = await call("/v1/products", { code: `${prefix}-S`, name: "安装服务", type: "SERVICE", unit: "次", salePrice: "3.00" });
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(t => localStorage.setItem("web-auth", JSON.stringify({ token: t, user: null })), token);
  page = await context.newPage(); const errors = []; page.on("pageerror", e => errors.push(e.message));
  await page.goto(`${base}/business/direct-orders/new`);
  await select(page, "供应商", partner.code); await select(page, "客户", partner.code);
  await select(page, "第 1 行商品", goods.code);
  assert.equal(await page.locator("#direct-contact").inputValue(), "收货人");
  await page.locator("#direct-contact").fill("");
  await page.locator("#direct-qty-0").fill("1.25");
  await page.getByRole("button", { name: "添加服务", exact: true }).click();
  await select(page, "第 2 行商品", service.code);
  await page.locator("#direct-qty-1").fill("1.5");
  await page.getByText("采购合计 ¥12.53 / 销售合计 ¥29.54", { exact: true }).waitFor();
  await page.getByRole("button", { name: "保存草稿", exact: true }).click();
  await page.waitForURL(/\/direct-orders\/\d+\/edit$/);
  const id = Number(new URL(page.url()).pathname.split("/").at(-2));
  const saved = await call(`/v1/direct-orders/${id}`);
  assert.equal(saved.status, "DRAFT"); assert.equal(saved.items.length, 2); assert.equal(saved.deliveryContact, "");
  assert.equal(saved.purchaseTotal, "12.53"); assert.equal(saved.saleTotal, "29.54");
  await page.getByRole("button", { name: "返回列表", exact: true }).click();
  await page.waitForURL(`${base}/business/direct-orders`);
  for (const label of ["供应商筛选", "客户筛选"]) {
    const filter = page.getByRole("group", { name: label, exact: true });
    await filter.getByRole("button", { name: `浏览选择${label}`, exact: true }).waitFor({ state: "visible" });
    assert.equal(await filter.getByRole("button", { name: `浏览选择${label}`, exact: true }).count(), 1);
    assert.equal(await filter.getByRole("button", { name: "浏览选择", exact: true }).count(), 0);
  }
  await page.getByRole("row").filter({ hasText: saved.documentNo }).getByRole("button", { name: "详情", exact: true }).click();
  await page.getByRole("heading", { name: `直送详情 · ${saved.documentNo}` }).waitFor();
  assert.equal(await page.getByRole("button", { name: "保存草稿", exact: true }).count(), 0);
  await page.getByRole("link", { name: "编辑草稿", exact: true }).click();
  await page.locator("#direct-qty-0").fill("2");
  await page.getByRole("button", { name: "返回列表", exact: true }).click();
  await page.getByRole("alertdialog").waitFor();
  await page.getByRole("alertdialog").getByRole("button", { name: "继续编辑" }).click();
  assert.equal(await page.locator("#direct-qty-0").inputValue(), "2");
  // Another operator changes the saved version before this editor submits.
  const original = await call(`/v1/direct-orders/${id}`);
  const editBody = { ...original, requestKey: `${prefix}-peer`, version: original.version, items: original.items.map(l => ({ ...l, id: undefined })) };
  await call(`/v1/direct-orders/${id}`, editBody, "PUT");
  await page.getByRole("button", { name: "保存草稿", exact: true }).click();
  await page.getByRole("alert").filter({ hasText: "版本已变化" }).waitFor();
  assert.equal(await page.locator("#direct-qty-0").inputValue(), "2");
  // Drop only the response after the save commits; restore by the original request key.
  page.on("dialog", d => d.accept());
  await page.getByRole("button", { name: "重新加载最新单据", exact: true }).click();
  await page.locator("#direct-qty-0").fill("3");
  await page.route(`**/api/v1/direct-orders/${id}`, async route => {
    if (route.request().method() !== "PUT") return route.continue();
    await route.fetch(); await route.abort("failed");
  });
  await page.getByRole("button", { name: "保存草稿", exact: true }).click();
  await page.getByText("原保存结果待核实，保留原标识，避免重复建单。", { exact: true }).waitFor();
  await page.unroute(`**/api/v1/direct-orders/${id}`);
  await page.reload();
  await page.getByRole("button", { name: "核实保存结果", exact: true }).click();
  await page.getByRole("status", { name: "操作结果", exact: true }).filter({ hasText: "已核实原保存结果" }).waitFor();
  assert.equal(await page.locator("#direct-qty-0").inputValue(), "3");
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ result: "passed", id, checks: ["mixed-draft", "reference-prices", "explicit-empty-delivery", "list-detail-edit", "leave-guard", "version-conflict", "lost-response-refresh-recovery"] }));
} catch (e) { if (page) console.error((await page.locator("body").innerText()).slice(0, 4500)); throw e; } finally { if (browser) await browser.close(); vite.kill("SIGTERM"); }
