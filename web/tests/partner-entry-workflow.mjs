// Page workflow checks use intercepted API responses; no real business data writes.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const base = "http://127.0.0.1:4234";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4234", "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "ignore" });
const partner = { id: 1, code: "ENTRY-P", name: "华兴公司", type: "COMPANY", isCustomer: true, isSupplier: true, status: 1 };
let browser;
const errors = [], writes = [];
let balance = "100.00", direction = "CUSTOMER", reject = false, balanceFailure = false, ledgerFailure = false;
const ledgerQueries = [];
let entries = [];
function entry(id, type, amount, before, after) {
  return { id, partnerId: 1, partnerName: partner.name, direction, entryType: type, amount, balanceBefore: before, balanceAfter: after, businessDate: "2026-10-14", effectiveAt: "2026-10-14T02:00:00Z", description: "启用前欠款", documentNo: "ENTRY-" + id, operatorId: 1, operatorName: "经营者", reversesId: null, reversedById: null, paymentMethod: "WECHAT", attachmentCount: 0 };
}
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch {} await delay(100); }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript(() => localStorage.setItem("web-auth", JSON.stringify({ token: "entry-fixture", user: null })));
  await context.route("**/api/**", async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname;
    if (!path.startsWith("/api/")) return route.continue();
    const respond = (data, status = 200, message = "OK") => route.fulfill({ status, contentType: "application/json", body: JSON.stringify({ code: status, data, message }) });
    if (path === "/api/auth/me") return respond({ id: 1, username: "fixture", roles: [] });
    if (path === "/api/auth/menus" || path.includes("/dict/")) return respond([]);
    if (path.endsWith("/unread-count")) return respond(0);
    if (path === "/api/v1/partners/1") return respond(partner);
    if (path === "/api/v1/partners") return respond({ records: [partner], total: 1 });
    if (path === "/api/v1/partner-balances") {
      if (balanceFailure) return respond(null, 503, "余额暂时不可用");
      return respond({ records: [{ partnerId: 1, partnerName: partner.name, direction, amount: balance, hasRecords: true, entryCount: entries.length }], total: 1 });
    }
    if (path === "/api/v1/partner-balances/entries") {
      if (!url.searchParams.get("category")) {
        ledgerQueries.push(Object.fromEntries(url.searchParams));
        if (ledgerFailure) return respond(null, 503, "明细暂时不可用");
        const page = Number(url.searchParams.get("page")), size = Number(url.searchParams.get("pageSize"));
        return respond({ records: entries.slice((page - 1) * size, page * size), total: entries.length });
      }
      const filtered = url.searchParams.get("reversalStatus") === "EFFECTIVE" ? entries.filter(e => e.entryType === "OPENING" && !e.reversedById) : entries;
      return respond({ records: filtered, total: filtered.length });
    }
    if (req.method() === "GET" && /entries\/\d+$/.test(path)) return respond(entries.find(e => e.id === Number(path.split("/").at(-1))));
    if (path.endsWith("/attachments")) return respond([]);
    if (req.method() === "POST") {
      const body = req.postDataJSON(); writes.push({ path, body });
      if (reject) { reject = false; return respond(null, 409, "余额已变化，请核对后重试"); }
      if (path.endsWith("/reverse")) {
        const original = entries.find(e => e.id === Number(path.split("/").at(-2)));
        const after = (Number(balance) - Number(original.amount)).toFixed(2);
        const reversed = { ...entry(2, "REVERSAL", (-Number(original.amount)).toFixed(2), balance, after), reversesId: original.id, reversedDocumentNo: original.documentNo };
        original.reversedById = 2; entries.push(reversed); balance = after; return respond(reversed);
      }
      const opening = path.endsWith("/opening"), refund = path.endsWith("/refunds");
      const delta = (opening || refund ? 1 : -1) * Number(body.amount);
      const after = (Number(balance) + delta).toFixed(2);
      const type = opening ? "OPENING" : refund ? direction + "_REFUND" : direction === "CUSTOMER" ? "RECEIPT" : "PAYMENT";
      const saved = { ...entry(1, type, delta.toFixed(2), balance, after), paymentMethod: body.paymentMethod };
      entries = [saved]; balance = after; return respond(saved);
    }
    return respond({ records: [], total: 0 });
  });
  const page = await context.newPage(); page.setDefaultTimeout(8000);
  page.on("pageerror", e => errors.push(e.message));
  async function visit(route) { await page.goto(base + "/business/" + route); await page.locator("main h1").waitFor().catch(async e => { throw new Error(e.message + " URL=" + page.url() + " errors=" + JSON.stringify(errors) + " body=" + (await page.locator("body").innerText()).slice(0, 1800)); }); }
  for (const [route, dir, amount, action, full] of [
    ["settlements", "CUSTOMER", "100.00", "登记收款", "收齐欠款"],
    ["settlements", "SUPPLIER", "100.00", "登记付款", "付清欠款"],
    ["refunds", "CUSTOMER", "-100.00", "向客户退款", "退清款项"],
    ["refunds", "SUPPLIER", "-100.00", "登记供应商退款", "收齐退款"],
    ["opening-balances", "CUSTOMER", "100.00", "录入客户原有欠款", null],
    ["opening-balances", "SUPPLIER", "100.00", "录入欠供应商款", null],
  ]) {
    direction = dir; balance = amount; entries = Array.from({ length: 12 }, (_, i) => entry(i + 90, "SALE", "10.00", "90.00", "100.00"));
    await visit(route);
    assert.equal(await page.getByRole("dialog").count(), 0, "default is records, no permanent form");
    assert.equal(await page.locator("main textarea").count(), 0);
    if (dir === "CUSTOMER") await page.screenshot({ path: "/tmp/partner-entry-list-" + route + ".png", fullPage: true });
    await page.getByRole("button", { name: action, exact: true }).click();
    let dialog = page.getByRole("dialog");
    assert.equal(await dialog.getByLabel("资金方向").count(), 0, "no repeated direction selection");
    const group = dialog.getByRole("group", { name: full ? "资金往来单位" : "期初往来单位", exact: true });
    await group.getByRole("combobox").fill(partner.code);
    await group.getByRole("option").filter({ hasText: partner.code }).click();
    const summary = dialog.getByLabel("当前往来余额");
    await summary.getByText("¥100.00", { exact: true }).waitFor();
    assert.ok(await summary.getByText("¥100.00", { exact: true }).evaluate(n => parseFloat(getComputedStyle(n).fontSize) >= 28), "balance amount is prominent");
    const input = dialog.getByLabel(full ? "资金金额" : "期初金额", { exact: true });
    if (full) { await dialog.getByRole("button", { name: full, exact: true }).click(); assert.equal(await input.inputValue(), "100.00"); }
    await input.fill("20.00");
    if (!full) await dialog.getByLabel("期初说明").fill("接入前未结欠款");
    const urlBefore = page.url(), queriesBefore = ledgerQueries.length;
    assert.equal(await dialog.getByLabel("当前单位往来明细").count(), 0, "details are not loaded before click");
    ledgerFailure = true;
    await summary.getByRole("button", { name: "查看往来明细", exact: true }).click();
    const inlineLedger = dialog.getByLabel("当前单位往来明细");
    await inlineLedger.getByText("明细暂时不可用").waitFor();
    ledgerFailure = false;
    await inlineLedger.getByRole("button", { name: "重试明细" }).click();
    await inlineLedger.getByRole("row").filter({ hasText: "ENTRY-90" }).waitFor();
    assert.ok(ledgerQueries.length >= queriesBefore + 2, "click loads details and retry sends a fresh query");
    assert.equal(ledgerQueries.at(-1).partnerId, "1"); assert.equal(ledgerQueries.at(-1).direction, dir);
    await inlineLedger.getByRole("button", { name: "下一页" }).click();
    await inlineLedger.getByRole("row").filter({ hasText: "ENTRY-100" }).waitFor();
    if (route === "settlements" && dir === "CUSTOMER") await page.screenshot({ path: "/tmp/partner-entry-inline-ledger.png", fullPage: true });
    assert.equal(page.url(), urlBefore, "inline lookup never navigates"); assert.equal(await input.inputValue(), "20.00");
    await summary.getByRole("button", { name: "收起往来明细" }).click();
    assert.equal(await dialog.getByLabel("当前单位往来明细").count(), 0);
    assert.equal(await dialog.locator("details[open]").count(), 0, "optional fields start folded");
    const count = writes.length;
    await dialog.getByRole("button", { name: "核对并登记" }).click();
    await dialog.getByLabel("登记核对").waitFor();
    assert.equal(writes.length, count, "review has no write");
    assert.ok(!(await dialog.innerText()).includes("-100.00"), "refund uses positive meaning");
    await dialog.getByRole("button", { name: "返回填写" }).click(); assert.equal(await input.inputValue(), "20.00");
    await dialog.getByRole("button", { name: "核对并登记" }).click();
    reject = true;
    await dialog.getByRole("button", { name: "确认登记并生效" }).click();
    await dialog.getByText("余额已变化，请核对后重试").waitFor(); assert.equal(await input.inputValue(), "20.00");
    await dialog.getByRole("button", { name: "核对并登记" }).click();
    await dialog.getByRole("button", { name: "确认登记并生效" }).click();
    await page.getByLabel("登记结果").waitFor(); assert.equal(await page.getByRole("dialog").count(), 0);
    assert.equal(writes.at(-1).body.requestKey, writes.at(-2).body.requestKey, "retry retains key");
    await page.getByRole("button", { name: "查看本笔记录" }).click();
    dialog = page.getByRole("dialog");
    await dialog.getByLabel("金额影响").waitFor();
    assert.ok(!(await dialog.getByLabel("金额影响").innerText()).includes("-20.00"), "actual funds are positive");
    await dialog.getByRole("button", { name: "冲销此记录" }).click();
    dialog = page.getByRole("dialog"); await dialog.getByText(/预计冲销后/).waitFor();
    await dialog.getByLabel("冲销原因（必填）").fill("录错款项");
    await dialog.getByRole("button", { name: "确认冲销", exact: true }).click();
    await page.getByRole("row").filter({ hasText: "ENTRY-1" }).getByRole("button", { name: "查看详情" }).click();
    dialog = page.getByRole("dialog"); await dialog.getByRole("button", { name: "查看冲销记录" }).click();
    await dialog.getByRole("button", { name: "ENTRY-1", exact: true }).waitFor();
    assert.equal(await dialog.getByRole("button", { name: "冲销此记录" }).count(), 0);
    await dialog.getByRole("button", { name: "ENTRY-1", exact: true }).click();
    await dialog.getByText("原记录已冲销", { exact: false }).waitFor();
    await dialog.getByRole("button", { name: "关闭", exact: true }).click();
    console.log("PASS", route, dir, "list/editor/review/retry/detail/reversal links");
  }
  direction = "CUSTOMER"; balance = "5025.00"; entries = [entry(1, "OPENING", "5025.00", "0.00", "5025.00")];
  await visit("opening-balances?partnerId=1&direction=CUSTOMER");
  let dialog = page.getByRole("dialog"); await dialog.getByText(/已有 1 笔未冲销期初记录/).waitFor();
  await dialog.getByRole("button", { name: "浏览选择期初往来单位" }).click();
  const picker = page.getByRole("dialog", { name: "选择期初往来单位", exact: true });
  await picker.getByRole("combobox").waitFor();
  for (let i = 0; i < 8; i++) { await page.keyboard.press("Tab"); assert.ok(await picker.evaluate(n => n.contains(document.activeElement)), "browse keeps focus inside"); }
  await page.keyboard.press("Escape");
  dialog = page.getByRole("dialog"); assert.equal(await dialog.getByLabel("期初金额").count(), 1, "Escape closes only browse");
  await dialog.getByLabel("期初金额").fill("50.00"); await dialog.getByLabel("期初说明").fill("追加补记");
  await page.keyboard.press("Escape"); await page.getByRole("alertdialog").waitFor();
  await page.getByRole("alertdialog").getByRole("button", { name: "继续填写" }).click();
  assert.equal(await dialog.getByLabel("期初金额").inputValue(), "50.00");
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 650 });
    await page.screenshot({ path: "/tmp/partner-entry-form-" + width + ".png", fullPage: true });
    const box = await dialog.getByRole("button", { name: "核对并登记" }).boundingBox();
    assert.ok(box.y >= 0 && box.y + box.height <= 650, "footer stays visible");
    assert.ok(await dialog.evaluate(n => n.scrollWidth <= n.clientWidth), "editor has no horizontal overflow");
    if (width >= 1024) assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  }
  await page.keyboard.press("Escape"); await page.getByRole("alertdialog").getByRole("button", { name: "放弃并离开" }).click();
  balanceFailure = true;
  await visit("refunds?partnerId=1&direction=SUPPLIER");
  dialog = page.getByRole("dialog"); await dialog.getByText("余额暂时不可用").waitFor();
  assert.ok(await dialog.getByRole("button", { name: "核对并登记" }).isDisabled());
  balanceFailure = false; direction = "SUPPLIER"; balance = "-100.00";
  await dialog.getByRole("button", { name: "重试余额" }).click();
  await dialog.getByLabel("当前往来余额").getByText("供应商还需退回", { exact: true }).waitFor();
  assert.deepEqual(errors, []);
  console.log("PASS existing opening warning, source auto-open, discard guard, short/narrow layouts, balance retry");
} finally { await browser?.close(); vite.kill("SIGTERM"); }
