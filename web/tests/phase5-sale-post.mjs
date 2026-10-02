import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const api = process.env.PHASE5_API_URL ?? "http://127.0.0.1:18109", base = "http://127.0.0.1:4193", prefix = `SP5-${Date.now()}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4193", "--strictPort"], { cwd: new URL("..", import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: "ignore" });
let token, browser;
async function call(path, body, method = body === undefined ? "GET" : "POST", auth = token) {
  const res = await fetch(`${api}/api${path}`, { method, headers: { Authorization: auth ?? "", "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
  const json = await res.json(); assert.equal(res.status, 200, `${path}: ${json.message}`); return json.data;
}
async function select(page, label, code) { const g = page.getByRole("group", { name: label, exact: true }); await g.getByRole("button").first().click(); await g.getByRole("textbox").fill(code); await g.locator('button[aria-pressed]').filter({ hasText: code }).last().click(); }
try {
  for (let i = 0; i < 60; i++) { try { if ((await fetch(base)).ok) break; } catch { /* starting */ } await delay(250); }
  token = (await call("/auth/login", { username: "admin", password: process.env.PHASE5_UI_PASSWORD ?? "admin123" })).tokenValue;
  const partner = await call("/v1/partners", { code: prefix, name: "过账客户", type: "COMPANY", isCustomer: true, isSupplier: true });
  const product = await call("/v1/products", { code: prefix, name: "过账商品", type: "GOODS", unit: "台", salePrice: "1.01" });
  const stock = await call("/v1/purchases",{partnerId:partner.id,businessDate:"2026-10-02",items:[{productId:product.id,productType:"GOODS",unit:"台",quantity:"10",unitPrice:"1.00"}]});
  await call(`/v1/purchases/${stock.id}/post`,{version:stock.version});
  await call("/system/user", { username: prefix, nickname: "另一位过账经营者", deptId: 1, status: 1 });
  const other = (await call("/auth/login", { username: prefix, password: "admin123" })).tokenValue;
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(t => localStorage.setItem("web-auth", JSON.stringify({ token: t, user: null })), token);
  const page = await context.newPage(), errors = []; page.on("pageerror", e => errors.push(e.message));
  page.on("dialog", d => d.accept());
  const newForm = async () => { await page.goto(`${base}/business/sales/new`); await select(page, "客户", partner.code); await select(page, "第 1 行商品", product.code); };
  const confirm = async () => { await page.getByRole("button", { name: "保存并过账", exact: true }).click(); await page.getByRole("dialog").getByRole("button", { name: "确认保存并过账", exact: true }).click(); };
  await newForm();
  let writes = 0;
  page.on("request", r => { if (r.url().includes("/api/v1/sales") && r.method() === "POST") writes++; });
  await page.getByRole("button", { name: "保存并过账", exact: true }).click();
  await page.getByRole("dialog").getByText(/扣减店内实物库存/).waitFor();
  await page.getByRole("dialog").getByRole("button", { name: "取消", exact: true }).click();
  assert.equal(writes, 0);
  // Save is rejected by the real API after master data are disabled; no posting.
  await call(`/v1/partners/${partner.id}/status`, { status: 0 }, "PUT");
  let posts = 0;
  page.on("request", r => { if (r.url().endsWith("/post") && r.method() === "POST") posts++; });
  await confirm(); await page.getByRole("alert").waitFor(); assert.equal(posts, 0);
  await call(`/v1/partners/${partner.id}/status`, { status: 1 }, "PUT");
  // Commit posting, lose the response, and verify the actual state.
  let created, postCalls = 0;
  await page.route("**/api/v1/sales/*/post", async route => { postCalls++; const response = await route.fetch(); created = (await response.json()).data; await route.abort("failed"); });
  await confirm(); await page.getByText(/库存与应收已生效/).waitFor();
  assert.equal(postCalls, 1); assert.equal(created.status, "POSTED");
  assert.equal((await call(`/v1/sales/${created.id}`)).status, "POSTED");
  await page.unroute("**/api/v1/sales/*/post");
  await page.waitForURL(`**/business/sales/${created.id}/edit*`);
  // No request reaches the API; refresh and explicitly retry the original ID/version.
  await newForm(); let unknownID, creations = 0;
  const countCreates = r => { if (r.url().endsWith("/api/v1/sales") && r.method() === "POST") creations++; };
  page.on("request", countCreates);
  await page.route("**/api/v1/sales/*/post", async route => { unknownID = Number(new URL(route.request().url()).pathname.split("/").at(-2)); await route.abort("failed"); });
  await confirm(); await page.getByText(/原过账结果尚未确认/).waitFor();
  assert.equal(await page.getByText(/正在过账/).count(), 0);
  assert.equal((await call(`/v1/sales/${unknownID}`)).status, "DRAFT");
  await page.unroute("**/api/v1/sales/*/post");
  await page.reload(); await page.getByRole("button", { name: "核实过账状态" }).click();
  await page.getByRole("button", { name: "重新核对并重试过账" }).click();
  const response = page.waitForResponse(r => r.url().endsWith(`/sales/${unknownID}/post`));
  await page.getByRole("dialog").getByRole("button", { name: "确认保存并过账" }).dblclick();
  await response; await page.getByText(/库存与应收已生效/).waitFor();
  assert.equal(creations, 1); assert.equal((await call(`/v1/sales/${unknownID}`)).status, "POSTED");
  page.off("request", countCreates);
  // Fail posting through real master-data validation after a successful save.
  await newForm(); let savedID;
  await page.route("**/api/v1/sales/*/post", async route => {
    savedID = Number(new URL(route.request().url()).pathname.split("/").at(-2));
    await call(`/v1/products/${product.id}/status`, { status: 0 }, "PUT");
    await route.continue();
  });
  await confirm(); await page.getByText(/仍为草稿，输入已保留/).waitFor();
  assert.equal((await call(`/v1/sales/${savedID}`)).status, "DRAFT");
  assert.equal(await page.getByLabel("数量").inputValue(), "1");
  await page.unroute("**/api/v1/sales/*/post");
  await call(`/v1/products/${product.id}/status`, { status: 1 }, "PUT");
  // Keep the same saved draft and explicitly confirm retry through safe save+post.
  await page.waitForURL(`**/business/sales/${savedID}/edit*`);
  await confirm(); await page.getByText(/库存与应收已生效/).waitFor();
  assert.equal((await call(`/v1/sales/${savedID}`)).status, "POSTED");
  // Another operator's change between saving and posting stops old confirmation.
  await newForm(); let changedID;
  await page.route("**/api/v1/sales/*/post", async route => {
    changedID = Number(new URL(route.request().url()).pathname.split("/").at(-2));
    const d = await call(`/v1/sales/${changedID}`);
    await call(`/v1/sales/${changedID}/cancel`, { version: d.version, reason: "确认期间取消" }, "POST", other);
    await route.continue();
  });
  await confirm(); await page.getByText(/原过账已停止/).waitFor();
  assert.equal((await call(`/v1/sales/${changedID}`)).status, "CANCELLED");
  assert.equal(await page.getByRole("button", { name: "保存并过账", exact: true }).isDisabled(), true);
  assert.equal(await page.getByText(/正在过账/).count(), 0);
  await page.unroute("**/api/v1/sales/*/post");
  // A second account changes content; the original confirmation must not post v2.
  await newForm(); let editedID;
  await page.route("**/api/v1/sales/*/post", async route => {
    editedID = Number(new URL(route.request().url()).pathname.split("/").at(-2));
    const d = await call(`/v1/sales/${editedID}`);
    await call(`/v1/sales/${editedID}`, { requestKey: `${prefix}-edit`, version: d.version, partnerId: d.partnerId, businessDate: d.businessDate, items: d.items.map(i => ({ id:i.id, productId: i.productId, productType: i.productType, unit: i.unit, quantity: "2", unitPrice: i.unitPrice })) }, "PUT", other);
    await route.continue();
  });
  await confirm(); await page.getByText(/原过账已停止/).waitFor();
  const changed = await call(`/v1/sales/${editedID}`);
  assert.equal(changed.status, "DRAFT"); assert.equal(changed.version, 2);
  assert.equal(await page.getByRole("button", { name: "保存并过账", exact: true }).isDisabled(), true);

  await page.unroute("**/api/v1/sales/*/post");
  // Real stock shortage is a definite refusal and leaves the saved page editable.
  await newForm(); await page.getByLabel("数量").fill("100");
  await confirm(); await page.getByText(/库存不足.*仍为草稿，输入已保留/).waitFor();
  await page.waitForURL(/\/business\/sales\/\d+\/edit(?:\?.*)?$/);
  const shortageID=Number(page.url().match(/sales\/(\d+)\/edit/)[1]);
  assert.equal(await page.getByLabel("数量").isDisabled(),false);
  assert.equal((await call(`/v1/sales/${shortageID}`)).status,"DRAFT");
  await page.getByLabel("数量").fill("1"); await confirm(); await page.getByText(/库存与应收已生效/).waitFor();
  assert.equal((await call(`/v1/sales/${shortageID}`)).status,"POSTED");
  // Same-product split lines aggregate stock; a zero-total sale still posts.
  await newForm(); await page.getByLabel("成交单价（元）").fill("0");
  await page.getByRole("button",{name:"添加明细"}).click(); await select(page,"第 2 行商品",product.code); await page.getByLabel("成交单价（元）").last().fill("0");
  await page.getByRole("button",{name:"保存并过账",exact:true}).click();
  await page.getByRole("dialog").getByText(/过账商品：-2.000 台/).waitFor();
  await page.getByRole("dialog").getByRole("button",{name:"确认保存并过账"}).click(); await page.getByText(/库存与应收已生效/).waitFor();
  // Pure service has no stock, including a lost save response followed by posting.
  const service=await call("/v1/products",{code:`${prefix}-SERVICE`,name:"无库存服务",type:"SERVICE",unit:"次",salePrice:"0.00"});
  await page.goto(`${base}/business/sales/new`); await select(page,"客户",partner.code); await select(page,"第 1 行商品",service.code);
  await page.route("**/api/v1/sales",async route=>{ if(route.request().method()!=="POST")return route.continue();await route.fetch();await route.abort("failed"); });
  await confirm(); await page.getByText(/库存无变化，应收已处理/).waitFor(); await page.unroute("**/api/v1/sales");
  // Direct source must be posted first; retry stays on the same sales document.
  const direct=await call("/v1/purchases",{directDelivery:true,partnerId:partner.id,businessDate:"2026-10-02",items:[{productId:product.id,productType:"GOODS",unit:"台",quantity:"2",unitPrice:"1.00"}]});
  await page.goto(`${base}/business/sales/new?directPurchaseId=${direct.id}`); await select(page,"客户",partner.code);
  await confirm(); await page.getByText(/仍为草稿，输入已保留/).waitFor(); await page.waitForURL(/\/business\/sales\/\d+\/edit(?:\?.*)?$/);
  const directID=Number(page.url().match(/sales\/(\d+)\/edit/)[1]);
  assert.equal((await call(`/v1/sales/${directID}`)).status,"DRAFT");
  await call(`/v1/purchases/${direct.id}/post`,{version:direct.version});
  await confirm(); await page.getByText(/库存无变化，应收已处理/).waitFor();
  assert.equal((await call(`/v1/sales/${directID}`)).directPurchaseId,direct.id);
  await page.getByRole("button",{name:"打印已保存送货单"}).click(); await page.locator(".delivery-page").waitFor();
  assert.deepEqual(errors, []);
  console.log("PASS real browser: stock shortage retains editable same draft, split/zero/service/direct and printing, confirmation cancel writes nothing, real save rejection never posts, lost post response verified, posting failure keeps same draft/input, safe same-ID retry, cancellation stops old confirmation");
} finally { if (browser) await browser.close(); vite.kill(); }
