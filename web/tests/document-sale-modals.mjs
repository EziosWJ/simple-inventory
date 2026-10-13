import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const base = "http://127.0.0.1:4212";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4212", "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "inherit" });
const partner = { id: 1, code: "MOCK-C", name: "销售确认测试客户", type: "COMPANY", isCustomer: true, isSupplier: false, status: 1 };
const goods = { id: 1, code: "MOCK-G", name: "确认测试商品", model: "G1", specification: "标准规格", type: "GOODS", unit: "台", salePrice: "10.00", status: 1 };
const service = { id: 2, code: "MOCK-S", name: "安装服务", type: "SERVICE", unit: "次", salePrice: "0.00", status: 1 };
const rows = values => ({ records: values, total: values.length, page: 1, pageSize: 10 });
const writes = [];
let nextID = 20, saved = null, failSave = false, failPost = false, loseNextPost = false, changeVersionNextPost = false, browser;
const errors = [];
const amount = (q, p) => (Number(q) * Number(p)).toFixed(2);
function asDraft(body, id) {
  return { ...body, id, documentNo: `MOCK-SALE-${id}`, status: "DRAFT", version: 1, totalAmount: body.items.reduce((sum, item) => sum + Number(amount(item.quantity, item.unitPrice)), 0).toFixed(2), partnerName: partner.name, createTime: "2026-10-12T04:00:00Z", createdByName: "测试用户", items: body.items.map((item, index) => ({ ...item, id: index + 1, amount: amount(item.quantity, item.unitPrice), productName: item.productId === service.id ? service.name : goods.name, productCode: item.productId === service.id ? service.code : goods.code, productModel: item.productId === service.id ? null : goods.model, productSpecification: item.productId === service.id ? null : goods.specification })) };
}
const response = (data, status = 200) => ({ status, contentType: "application/json", body: JSON.stringify({ code: status, message: status === 200 ? "OK" : "mock failure", data }) });
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch { /* starting */ } await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(() => localStorage.setItem("web-auth", JSON.stringify({ token: "sale-modal-mock", user: { id: 17, username: "mock", nickname: "Mock", roles: [] } })));
  await context.route("**/api/**", async route => {
      const req = route.request(), url = new URL(req.url()), path = url.pathname;
      if (!path.startsWith("/api/")) return route.continue();
    if (req.method() !== "GET") {
      const body = req.postDataJSON(); writes.push({ method: req.method(), path, body });
      if (path === "/api/v1/sales" && failSave) { failSave = false; return route.fulfill(response(null, 400)); }
      if (path === "/api/v1/sales" && req.method() === "POST") { saved = asDraft(body, ++nextID); return route.fulfill(response(saved)); }
      if (/\/api\/v1\/sales\/\d+\/post$/.test(path)) {
        if (failPost) { failPost = false; return route.fulfill(response(null, 409)); }
        if (changeVersionNextPost) { changeVersionNextPost = false; saved = { ...saved, status: "DRAFT", version: saved.version + 1 }; return route.abort("failed"); }
        saved = { ...saved, status: "POSTED", version: saved.version + 1, postedAt: "2026-10-12T04:01:00Z", postedByName: "测试用户" };
        if (loseNextPost) { loseNextPost = false; return route.abort("failed"); }
        return route.fulfill(response(saved));
      }
      return route.fulfill(response(saved));
    }
    let data = rows([]);
    if (path === "/api/auth/me") data = { id: 17, username: "mock", roles: [] };
    else if (path === "/api/auth/menus") data = [];
    else if (path === "/api/v1/products") data = rows([goods, service].filter(p => !url.searchParams.get("keyword") || `${p.code} ${p.name}`.includes(url.searchParams.get("keyword"))));
    else if (path === "/api/v1/products/1") data = goods;
    else if (path === "/api/v1/products/2") data = service;
    else if (path === "/api/v1/partners") data = rows([partner]);
    else if (path === "/api/v1/partners/1") data = partner;
    else if (path === "/api/v1/sales") data = rows(saved ? [saved] : []);
    else if (/\/api\/v1\/sales\/\d+$/.test(path)) data = saved;
    return route.fulfill(response(data));
  });
  const page = await context.newPage(); page.setDefaultTimeout(10000); page.on("pageerror", error => errors.push(error.message));
  const select = async (label, code) => {
    const group = page.getByRole("group", { name: label, exact: true });
    await group.getByRole("combobox").fill(code);
    await page.getByRole("option").filter({ has: page.getByText(code, { exact: true }) }).click();
  };
  const openForm = async (url = `${base}/business/sales/new`) => { await page.goto(url); await select("客户", partner.code); await select("第 1 行商品", goods.code); };
  const openConfirm = async () => { await page.getByRole("button", { name: "保存并过账", exact: true }).click(); return page.getByRole("dialog"); };

  await openForm();
  const dialog = await openConfirm();
  await dialog.getByText("新单待保存").waitFor();
  await dialog.getByRole("table").getByText(goods.name).waitFor();
  await dialog.screenshot({ path: "/tmp/issue90-sale-confirm.png" });
  await dialog.getByRole("button", { name: "返回修改", exact: true }).click();
  assert.equal(writes.length, 0, "closing an unconfirmed preview must not write");

  await page.getByRole("button", { name: "添加明细" }).click(); await select("第 2 行商品", goods.code);
  await page.getByLabel("数量").last().fill("2"); await page.getByLabel("成交单价（元）").last().fill("12.00");
  const mixed = await openConfirm();
  assert.equal(await mixed.getByRole("table").getByRole("row").filter({ hasText: goods.name }).count(), 2, "same product at different prices remains two sale rows");
  assert.equal(await mixed.getByRole("table").getByRole("cell", { name: "¥10.00", exact: true }).count(), 2);
  await mixed.getByRole("table").getByRole("cell", { name: "¥12.00", exact: true }).waitFor();
  await mixed.getByText(/扣减 3 台/).waitFor();
  await mixed.getByRole("button", { name: "返回修改", exact: true }).click();

  await page.getByRole("button", { name: "添加明细" }).click(); await select("第 3 行商品", service.code);
  const withService = await openConfirm(); await withService.getByText("2 行实物 · 1 行服务").waitFor();
  await withService.getByText("确认后客户方向往来余额增加 ¥34.00").waitFor();
  await withService.getByRole("button", { name: "返回修改", exact: true }).click();

  failSave = true;
  const before = writes.length;
  const saveFailDialog = await openConfirm(); await saveFailDialog.getByRole("button", { name: "确认保存并过账" }).click();
  await saveFailDialog.getByRole("alert").waitFor();
  assert.equal(writes.slice(before).filter(item => item.path.endsWith("/post")).length, 0, "save rejection must not post");
  await saveFailDialog.getByRole("button", { name: "返回修改", exact: true }).click();

  // Save succeeds, while a definite posting refusal keeps the same saved draft visible.
  failPost = true;
  const failedPostDialog = await openConfirm(); await failedPostDialog.getByRole("button", { name: "确认保存并过账" }).click();
  await failedPostDialog.getByRole("alert").filter({ hasText: "仍为草稿" }).waitFor();
  const sameID = saved.id;
  assert.equal(saved.status, "DRAFT");
  assert.equal(writes.filter(item => item.path === "/api/v1/sales").length, 2);
  await failedPostDialog.screenshot({ path: "/tmp/issue90-sale-post-failed.png" });
  await failedPostDialog.getByRole("button", { name: "返回修改", exact: true }).click();

  // A lost post response is reconciled by the existing detail read; a version change is not replayed.
  await page.goto(`${base}/business/sales/${sameID}/edit`);
  await page.getByRole("button", { name: "保存并过账", exact: true }).waitFor();
  loseNextPost = true;
  const retryConfirm = await openConfirm(); await retryConfirm.getByRole("button", { name: "确认保存并过账" }).click();
  await page.getByRole("status", { name: "操作结果" }).filter({ hasText: "销售单过账成功" }).waitFor();
  assert.equal(saved.status, "POSTED"); assert.equal(saved.id, sameID);

  // A changed draft version is read back and the old post is never replayed.
  await page.goto(`${base}/business/sales/new`); await select("客户", partner.code); await select("第 1 行商品", goods.code);
  changeVersionNextPost = true;
  const changedVersionDialog = await openConfirm();
  const postCountBeforeConflict = writes.filter(item => item.path.endsWith("/post")).length;
  await changedVersionDialog.getByRole("button", { name: "确认保存并过账" }).click();
  await changedVersionDialog.getByRole("alert").filter({ hasText: "原过账已停止" }).waitFor();
  assert.ok(await changedVersionDialog.getByRole("button", { name: "确认保存并过账" }).isDisabled(), "conflict stops the old confirmation");
  assert.equal(saved.status, "DRAFT"); assert.equal(saved.version, 2);
  assert.equal(writes.filter(item => item.path.endsWith("/post")).length, postCountBeforeConflict + 1, "version change is verified without replay");
  await changedVersionDialog.getByRole("button", { name: "返回修改" }).click();

  // Pure service zero amount: no inventory and no receivable movement.
  saved = null; await page.goto(`${base}/business/sales/new`); await select("客户", partner.code); await select("第 1 行商品", service.code);
  const serviceDialog = await openConfirm(); await serviceDialog.getByText("0 行实物 · 1 行服务").waitFor();
  await serviceDialog.getByText("纯服务销售，不改变实物库存。").waitFor();
  await serviceDialog.getByText(/零金额单据不会产生往来金额流水/).waitFor();
  await serviceDialog.getByRole("button", { name: "返回修改", exact: true }).click();

  // List flow: inspect a long draft at a short/narrow viewport and keep failures in the modal.
  const manyItems = Array.from({ length: 24 }, (_, i) => ({ id: i + 1, productId: goods.id, productCode: goods.code, productName: `${goods.name} ${i + 1}`, productModel: goods.model, productSpecification: goods.specification, productType: "GOODS", unit: goods.unit, quantity: "1", unitPrice: "10.00", amount: "10.00" }));
  saved = { id: 80, documentNo: "MOCK-LONG", status: "DRAFT", version: 1, partnerId: partner.id, partnerName: partner.name, businessDate: "2026-10-12", totalAmount: "240.00", items: manyItems };
  await page.setViewportSize({ width: 390, height: 600 }); await page.goto(`${base}/business/sales`);
  await page.getByRole("button", { name: "过账", exact: true }).click();
  const listDialog = page.getByRole("dialog"); await listDialog.getByRole("table").getByText(`${goods.name} 24`).waitFor();
  const body = listDialog.locator("form > div").first();
  const footer = listDialog.locator("form > footer");
  await listDialog.screenshot({ path: "/tmp/issue90-sale-confirm-narrow.png" });
  assert.ok(await body.evaluate(el => el.scrollHeight > el.clientHeight), "long preview body should scroll in short viewport");
  assert.ok(await footer.evaluate(el => { const r = el.getBoundingClientRect(); return r.bottom <= innerHeight && r.top >= 0; }), "confirm footer remains visible");
  await listDialog.getByRole("button", { name: "返回核对", exact: true }).click();
  const priorListPostCount = writes.filter(item => item.path.endsWith("/post")).length;
  failPost = true;
  await page.getByRole("button", { name: "过账", exact: true }).click();
  const listFailure = page.getByRole("dialog"); await listFailure.getByRole("button", { name: "确认过账" }).click();
  await listFailure.getByRole("alert").filter({ hasText: "请打开单据详情核实当前状态" }).waitFor();
  assert.ok(await listFailure.getByRole("button", { name: "确认过账", exact: true }).isDisabled(), "failed/unknown list confirmation requires state verification before retry");
  assert.equal(writes.filter(item => item.path.endsWith("/post")).length, priorListPostCount + 1, "list confirmation submits once per explicit confirm");
  assert.deepEqual(errors, []);
  console.log("PASS isolated browser mock: cancel causes no write; same-product prices stay separate while inventory aggregates; mixed and pure service zero-amount previews; save refusal never posts; same-ID save/post failure and lost-response/version-change reconciliation; list confirmation failure stays visible; 24-line preview scrolls with fixed footer at 390x600");
} finally { await browser?.close(); vite.kill(); }
