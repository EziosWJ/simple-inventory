// Requires an isolated API started with task api:sqlite. All setup uses public APIs.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const api = process.env.PHASE4_API_URL ?? "http://127.0.0.1:18099";
const base = "http://127.0.0.1:4179";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4179", "--strictPort"], { cwd: new URL("..", import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: "ignore" });
let token;
const prefix = `UI-${Date.now()}`;
async function request(path, body, method = body === undefined ? "GET" : "POST", auth = token) {
  const response = await fetch(`${api}/api${path}`, { method, headers: { "Content-Type": "application/json", ...(auth ? { Authorization: auth } : {}) }, body: body === undefined ? undefined : JSON.stringify(body) });
  const envelope = await response.json(); assert.equal(response.status, 200, `${method} ${path}: ${envelope.message}`); return envelope.data;
}
const row = (page, no) => page.getByRole("row").filter({ hasText: no });
const item = (p, quantity = "10") => ({ productId: p.id, productType: p.type, unit: p.unit, quantity, unitPrice: "1.01" });
const post = (kind, d, auth = token) => request(`/v1/${kind}/${d.id}/post`, { version: d.version }, "POST", auth);
async function product(code, type = "GOODS", unit = "台") { return request("/v1/products", { code: `${prefix}-${code}`, name: `${prefix}-${code}`, type, unit }); }
try {
  for (let n=0;n<40;n++) {try { if((await fetch(base)).ok) break; }catch{/*starting*/} await delay(250);}
  token = (await request("/auth/login", { username: "admin", password: process.env.PHASE4_UI_PASSWORD ?? "admin123" })).tokenValue;
  const partner = await request("/v1/partners", { code: prefix, name: prefix, type: "COMPANY", isCustomer: true, isSupplier: true, contact: "档案联系人", phone: "123", address: "档案地址" });
  const a = await product("A"), b = await product("B");
  const pi = await post("purchases", await request("/v1/purchases", { partnerId: partner.id, businessDate: "2026-10-01", items: [item(a), item(b)] }));
  const so = await post("sales", await request("/v1/sales", { partnerId: partner.id, businessDate: "2026-10-01", items: [item(a), item(b)] }));
  const browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  try {
    const context = await browser.newContext({ timezoneId: "Asia/Shanghai", viewport: { width: 1400, height: 1000 } });
    await context.addInitScript(value => localStorage.setItem("web-auth", JSON.stringify({ token: value, user: null })), token);
    const page = await context.newPage();
    const errors = []; page.on("pageerror", e => errors.push(e.message));
    for (const [kind, origin] of [["purchase", pi], ["sale", so]]) {
      const draft = await request(`/v1/${kind}-returns`, { [`${kind}Id`]: origin.id, businessDate: "2026-10-01", items: [{ [`${kind}ItemId`]: origin.items[1].id, quantity: "2" }] });
      await page.goto(`${base}/business/${kind}-returns`);
      await row(page, draft.documentNo).getByRole("button", { name: "编辑", exact: true }).click();
      const inputs = page.getByRole("dialog").locator('input[inputmode="decimal"]');
      await inputs.first().waitFor(); assert.deepEqual(await inputs.evaluateAll(nodes=>nodes.map(n=>n.value)), ["0", "2"]);
      await inputs.nth(1).fill("3");
      await page.getByRole("dialog").getByRole("button", { name: "保存", exact: true }).click();
      await page.getByRole("dialog").waitFor({ state: "hidden" });
      let saved = await request(`/v1/${kind}-returns/${draft.id}`);
      assert.equal(saved.items.length, 1); assert.equal(saved.items[0][`${kind}ItemId`], origin.items[1].id); assert.equal(saved.items[0].quantity, "3");
      await row(page, draft.documentNo).getByRole("button", { name: "编辑", exact: true }).click();
      await inputs.first().waitFor(); await inputs.nth(0).fill("1"); await inputs.nth(1).fill("");
      await page.getByRole("dialog").getByRole("button", { name: "保存", exact: true }).click(); await page.getByRole("dialog").waitFor({ state: "hidden" });
      saved = await request(`/v1/${kind}-returns/${draft.id}`); assert.equal(saved.items.length, 1); assert.equal(saved.items[0][`${kind}ItemId`], origin.items[0].id);
      console.log(`#32 real API + UI ${kind} edit/add/clear persisted correctly`);
    }
    for (const [kind, type] of [["purchases", "GOODS"], ["sales", "GOODS"], ["sales", "SERVICE"]]) {
      const p = await product(`${kind}-${type}`, type, "小时");
      const d = await request(`/v1/${kind}`, { partnerId: partner.id, businessDate: "2026-10-01", items: [item(p, "2.5")] });
      await request(`/v1/products/${p.id}`, { name: p.name, type, unit: "次" }, "PUT");
      await page.goto(`${base}/business/${kind}`); await row(page, d.documentNo).getByRole("button", { name: "编辑", exact: true }).click();
      const dialog = page.getByRole("dialog"); await dialog.getByRole("button", { name: "确认使用新类型和单位" }).waitFor();
      assert.equal(await dialog.getByRole("button", { name: "保存", exact: true }).isDisabled(), true);
      assert.equal((await request(`/v1/${kind}/${d.id}`)).items[0].unit, "小时");
      await dialog.getByRole("button", { name: "确认使用新类型和单位" }).click();
      await dialog.getByRole("button", { name: "保存", exact: true }).click(); await dialog.waitFor({ state: "hidden" });
      const saved = await request(`/v1/${kind}/${d.id}`); assert.equal(saved.items[0].unit, "次"); assert.equal(saved.items[0].quantity, "2.5");
      console.log(`#33 real API + UI ${kind} ${type} confirmation persisted without conversion`);
    }
    // Date and delivery snapshot are verified against the complete sales/print routes.
    await page.clock.setFixedTime(new Date("2026-10-01T16:15:00Z"));
    await page.goto(`${base}/business/sales`); await page.getByRole("button", { name: "新建销售草稿" }).click();
    const dialog = page.getByRole("dialog"); assert.equal(await dialog.locator('input[type="date"]').inputValue(), "2026-10-02");
    await dialog.locator("select").first().selectOption(String(partner.id));
    for (const field of ["送货联系人", "送货电话", "送货地址"]) await dialog.getByLabel(field).fill("");
    const service = await product("PRINT-SERVICE", "SERVICE", "次");
    // Refresh the catalog by reopening the page after creating the service.
    await dialog.getByRole("button", { name: "取消", exact: true }).click(); await page.reload();
    await page.getByRole("button", { name: "新建销售草稿" }).click(); await dialog.locator("select").first().selectOption(String(partner.id));
    for (const field of ["送货联系人", "送货电话", "送货地址"]) await dialog.getByLabel(field).fill("");
    await dialog.locator("select").last().selectOption(String(service.id));
    const createdResponse = page.waitForResponse(r=>r.url().endsWith("/api/v1/sales")&&r.request().method()==="POST");
    await dialog.getByRole("button", { name: "保存", exact: true }).click(); const created = (await (await createdResponse).json()).data;
    await dialog.waitFor({ state: "hidden" }); assert.equal(created.businessDate, "2026-10-02");
    for (const field of ["deliveryContact", "deliveryPhone", "deliveryAddress"]) assert.equal(created[field], "");
    await post("sales", created);
    await request(`/v1/partners/${partner.id}`, { name: partner.name, type: "COMPANY", isCustomer: true, isSupplier: true, contact: "后来联系人", phone: "456", address: "后来地址" }, "PUT");
    await page.goto(`${base}/business/sales/${created.id}/delivery-note`); await page.locator(".delivery-page").waitFor();
    const printText = await page.locator(".delivery-note-print").innerText(); assert.equal(printText.includes("后来联系人"), false); assert.equal(printText.includes("后来地址"), false);
    console.log("#34 real Shanghai midnight sale and empty posted delivery-note reprint passed");
    await page.clock.resume();
    // Direct order refusal, then navigate the provided sales-return action.
    const g = await product("DIRECT");
    const directPi = await post("purchases", await request("/v1/purchases", { directDelivery: true, partnerId: partner.id, businessDate: "2026-10-01", items: [item(g,"3")] }));
    await page.goto(`${base}/business/sales?directPurchaseId=${directPi.id}`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: "新建销售草稿", exact: true }).waitFor();
    await dialog.locator("select").nth(1).selectOption(String(partner.id));
    assert.equal(await dialog.getByRole("button", { name: "保存", exact: true }).isDisabled(), false);
    const directResponse = page.waitForResponse(r=>r.url().endsWith("/api/v1/sales")&&r.request().method()==="POST");
    await dialog.getByRole("button", { name: "保存", exact: true }).click();
    const directDraft = (await (await directResponse).json()).data;
    assert.equal(directDraft.items[0].unit, "台");
    const directSo = await post("sales", directDraft);
    console.log("direct sales draft from purchase UI keeps its source unit");
    await post("purchases", await request("/v1/purchases", { partnerId: partner.id, businessDate: "2026-10-01", items: [item(g,"5")] }));
    const pr = await request("/v1/purchase-returns", { purchaseId: directPi.id, businessDate: "2026-10-01", items: [{ purchaseItemId: directPi.items[0].id, quantity: "1" }] });
    await page.goto(`${base}/business/purchase-returns`);
    const refusal = page.waitForResponse(r=>r.url().endsWith(`/purchase-returns/${pr.id}/post`));
    await row(page,pr.documentNo).getByRole("button", { name: "过账", exact: true }).click(); assert.equal((await refusal).status(),409);
    assert.equal((await request(`/v1/purchase-returns/${pr.id}`)).status,"DRAFT");
    await row(page,pr.documentNo).getByRole("button", { name: "编辑", exact: true }).click();
    await dialog.getByRole("button", { name: `第一步：销售退货 · ${directSo.documentNo}`, exact: true }).click();
    await page.waitForURL(`**/business/sale-returns?saleId=${directSo.id}`);
    await page.getByRole("heading", { name: "新建销售退货草稿", exact: true }).waitFor();
    await dialog.locator('input[inputmode="decimal"]').fill("1");
    const salesReturnResponse=page.waitForResponse(r=>r.url().endsWith("/api/v1/sale-returns")&&r.request().method()==="POST");
    await dialog.getByRole("button", { name: "保存", exact: true }).click();const sr=(await (await salesReturnResponse).json()).data;await dialog.waitFor({state:"hidden"});
    await request("/system/user",{username:`poster-${prefix}`,nickname:"UI实际过账人",deptId:1,status:1});
    const other=(await request("/auth/login",{username:`poster-${prefix}`,password:process.env.PHASE4_UI_PASSWORD??"admin123"})).tokenValue;
    await post("sale-returns",sr,other);
    await page.goto(`${base}/business/sale-returns`);
    assert.match(await row(page,sr.documentNo).innerText(),/UI实际过账人/);
    await row(page,sr.documentNo).getByRole("button",{name:"查看",exact:true}).click();
    assert.match(await page.getByRole("dialog").innerText(),/过账人：UI实际过账人/);
    assert.match(await page.getByRole("dialog").innerText(),/2026/);
    console.log("#35 real refusal and linked sales-return navigation; #36 actual poster list/detail passed");
    await page.goto(`${base}/business/purchase-returns`);
    const success=page.waitForResponse(r=>r.url().endsWith(`/purchase-returns/${pr.id}/post`));
    await row(page,pr.documentNo).getByRole("button",{name:"过账",exact:true}).click();assert.equal((await success).status(),200);
    console.log("#35 real purchase-return succeeds after sales-return posting");
    const large = await product("LARGE");
    const huge = { ...item(large, "6000000000000000"), unitPrice: "0.00" };
    const largePi = await post("purchases", await request("/v1/purchases", { partnerId: partner.id, businessDate: "2026-10-01", items: [huge] }));
    const largeSo = await post("sales", await request("/v1/sales", { partnerId: partner.id, businessDate: "2026-10-01", items: [huge] }));
    for (const [kind, origin] of [["sale", largeSo], ["purchase", largePi]]) {
      const body = { [`${kind}Id`]: origin.id, businessDate: "2026-10-01", items: [{ [`${kind}ItemId`]: origin.items[0].id, quantity: huge.quantity }] };
      const old = await post(`${kind}-returns`, await request(`/v1/${kind}-returns`, body));
      await request(`/v1/${kind}-returns/${old.id}/cancel`, { version: old.version, reason: "UI历史验收" });
      await post(`${kind}-returns`, await request(`/v1/${kind}-returns`, body));
      await page.goto(`${base}/business/${kind}-returns?${kind}ReturnId=${old.id}`);
      await page.getByRole("dialog").waitFor();
      const history = await page.getByRole("dialog").innerText();
      assert.match(history, /6000000000000000/); assert.match(history, /CANCELLED/);
      await page.getByRole("dialog").getByRole("button", { name: "关闭详情弹窗" }).click();
      await page.getByPlaceholder("输入单号").fill(old.documentNo);
      await page.getByRole("button", { name: "查询", exact: true }).click();
      await row(page, old.documentNo).waitFor(); assert.match(await row(page, old.documentNo).innerText(), /已取消/);
      console.log(`#36 real ${kind} cancelled large history detail/list readable after re-return`);
    }
    assert.deepEqual(errors,[]);await context.close();
  } finally {await browser.close();}
} finally {vite.kill();}
