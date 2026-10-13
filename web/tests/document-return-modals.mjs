import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const base = "http://127.0.0.1:4213";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4213", "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "ignore" });
const statuses = ["DRAFT", "POSTED", "CANCELLED"];
const errors = [], writes = [];
const longItems = Array.from({ length: 24 }, (_, index) => makeItem(index + 1));
function makeItem(id) {
  return { id, purchaseItemId: id, saleItemId: id, productId: id === 2 ? 1 : id, productCode: `RET-${id}`, productName: id === 1 ? "长名称商品用于验证详情表格不会挤压弹窗" : `退货商品 ${id}`, productModel: "型号 A", productSpecification: "规格一", unit: id === 2 ? "箱" : "台", originalQuantity: "10", returnedQuantity: "2", remainingQuantity: "8", quantity: "1", unitPrice: "3.33", amount: "3.33", priorReturnAmount: "6.66", remark: id === 1 ? "行备注换行\n第二行备注" : "" };
}
function makeReturn(kind, id, status) {
  const purchase = kind === "purchase", directTrace = {
    purchase: { id: 100, kind: "PURCHASE", documentNo: "PI-DIRECT-100", status: "POSTED", partnerName: "追溯供应商", postedAt: "2026-10-12T02:00:00Z", postedByName: "操作员", items: [] },
    sales: [], purchaseReturns: [], saleReturns: [],
  };
  const items = status === "POSTED" ? [makeItem(1)] : longItems;
  return {
    id, documentNo: `${purchase ? "PR" : "SR"}-00${id}`, purchaseId: 100, saleId: 200,
    purchaseNo: "PI-ORIGINAL-100", saleNo: "SO-ORIGINAL-200", partnerId: purchase ? 11 : 22,
    partnerName: purchase ? "示例供应商" : "示例客户", businessDate: "2026-10-12", status,
    version: 4, postedBy: status === "POSTED" ? 8 : null, postedByName: status === "POSTED" ? "王操作员" : "",
    postedAt: status === "POSTED" ? "2026-10-12T03:04:05Z" : null, remark: "单据备注第一行\n第二行",
    createdBy: 7, createdByName: "李操作员", createTime: "2026-10-12T01:00:00Z",
    cancelledByName: status === "CANCELLED" ? "李操作员" : "", cancelledAt: status === "CANCELLED" ? "2026-10-12T04:00:00Z" : null,
    cancelReason: status === "CANCELLED" ? "客户撤回本次退货申请" : null,
    directTrace, items, totalAmount: status === "DRAFT" ? "79.92" : "3.33",
  };
}
const docs = Object.fromEntries(["purchase", "sale"].flatMap(kind => statuses.map((status, i) => [`${kind}-${i + 1}`, makeReturn(kind, i + 1, status)])));
docs["purchase-2"].items[0].returnedQuantity = "3";
docs["purchase-2"].items[0].remainingQuantity = "7";
docs["sale-1"].totalAmount = "0.00";
docs["sale-1"].items = docs["sale-1"].items.map(item => ({ ...item, unitPrice: "0.00", amount: "0.00" }));
let rejectPost = false;
let browser;
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch { /* Vite starting */ } await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1024, height: 600 }, timezoneId: "Asia/Shanghai" });
  await context.addInitScript(() => { if (location.origin === "http://127.0.0.1:4213") localStorage.setItem("web-auth", JSON.stringify({ token: "return-modal-fixture", user: null })); });
  context.on("page", page => page.on("pageerror", error => { errors.push(error.message); console.error("pageerror", page.url(), error.message); }));
  await context.route("**/api/**", async route => {
    const request = route.request(), path = new URL(request.url()).pathname;
    if (!path.startsWith("/api/")) return route.continue();
    if (request.method() !== "GET") {
      writes.push({ method: request.method(), path, body: request.postDataJSON() });
      if (path.endsWith("/post") && rejectPost) {
        rejectPost = false;
        return route.fulfill({ status: 409, contentType: "application/json", body: JSON.stringify({ code: 409, message: "当前有效退货额度不足，请重新核对" }) });
      }
      const id = Number(path.split("/").at(-2));
      const kind = path.includes("purchase-returns") ? "purchase" : "sale";
      return route.fulfill({ contentType: "application/json", body: JSON.stringify({ code: 200, message: "OK", data: { ...docs[`${kind}-${id}`], status: "POSTED" } }) });
    }
    let data = { records: [], total: 0, page: 1, pageSize: 10 };
    if (path === "/api/auth/me") data = { id: 1, username: "returns", nickname: "退货验收", roles: [] };
    else if (path === "/api/auth/menus") data = [];
    else if (path.endsWith("/unread-count")) data = 0;
    else if (path.startsWith("/api/v1/dict/")) data = [];
    else if (/^\/api\/v1\/(purchase|sale)-returns$/.test(path)) {
      const kind = path.includes("purchase-returns") ? "purchase" : "sale";
      data = { records: statuses.map((status, i) => docs[`${kind}-${i + 1}`]), total: 3, page: 1, pageSize: 10 };
    } else if (/^\/api\/v1\/(purchase|sale)-returns\/\d+$/.test(path)) {
      const kind = path.includes("purchase-returns") ? "purchase" : "sale";
      data = docs[`${kind}-${path.split("/").at(-1)}`];
    } else if (/^\/api\/v1\/(purchase|sale)-returns\/source\//.test(path)) {
      const kind = path.includes("purchase-returns") ? "purchase" : "sale";
      data = { ...docs[`${kind}-1`], id: undefined, documentNo: undefined, createdBy: undefined, createdByName: undefined, cancelledBy: undefined, cancelledByName: undefined, cancelReason: undefined, createTime: undefined, cancelledAt: undefined, remark: undefined };
    } else if (path === "/api/v1/purchases/100") data = { id: 100, documentNo: "PI-ORIGINAL-100", partnerName: "示例供应商", businessDate: "2026-10-01", totalAmount: "33.30", status: "POSTED", items: [] };
    else if (path === "/api/v1/sales/200") data = { id: 200, documentNo: "SO-ORIGINAL-200", partnerName: "示例客户", businessDate: "2026-10-01", totalAmount: "33.30", status: "POSTED", items: [] };
    else if (path === "/api/v1/purchases" || path === "/api/v1/sales") data = { records: [], total: 0, page: 1, pageSize: 10 };
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ code: 200, message: "OK", data }) });
  });

  const page = await context.newPage();
  page.on("console", message => { if (message.type() === "error") console.error(`browser console: ${message.text()}`); });
  page.on("requestfailed", request => console.error(`request failed: ${request.method()} ${request.url()} ${request.failure()?.errorText ?? ""}`));
  const visit = async kind => { await page.goto(`${base}/business/${kind}-returns`); await page.locator("main h1").filter({ hasText: kind === "purchase" ? "采购退货" : "销售退货" }).waitFor(); };
  const openDetail = async (kind, rowStatus) => {
    const row = page.locator("tbody tr").filter({ hasText: `${kind === "purchase" ? "PR" : "SR"}-00${statuses.indexOf(rowStatus) + 1}` }).first();
    await row.getByRole("button", { name: "查看" }).click();
    await page.getByRole("dialog").getByRole("heading", { name: new RegExp(`${kind === "purchase" ? "采购" : "销售"}退货 ·`) }).waitFor();
  };

  for (const kind of ["purchase", "sale"]) {
    await visit(kind);
    for (const status of statuses) {
      await openDetail(kind, status);
      const dialog = page.getByRole("dialog");
      await dialog.getByText(status === "DRAFT" ? "草稿" : status === "POSTED" ? "已过账" : "已取消", { exact: true }).first().waitFor();
      await dialog.getByRole("columnheader", { name: "原单数量", exact: true }).waitFor();
      await page.keyboard.press("Tab");
      assert.ok(await dialog.evaluate(node => node.contains(document.activeElement)), "document detail keeps keyboard focus inside");
      await dialog.getByRole("columnheader", { name: "当前有效已退", exact: true }).waitFor();
      await dialog.getByRole("columnheader", { name: "本次退货数量", exact: true }).waitFor();
      assert.equal(await dialog.getByText(/原(?:采购|销售)明细ID/).count(), 0);
      if (status === "CANCELLED") await dialog.getByText("客户撤回本次退货申请").waitFor();
      if (status === "POSTED") {
        await dialog.getByText(kind === "purchase" ? "3 台" : "2 台", { exact: true }).waitFor();
        await dialog.getByText("1 台", { exact: true }).waitFor();
      }
      if (kind === "purchase" && status === "DRAFT") {
        await dialog.getByRole("alert").filter({ hasText: "先过账关联销售退货" }).waitFor();
        await dialog.getByRole("columnheader", { name: "当前有效已退", exact: true }).waitFor();
        await dialog.getByText("2 台").first().waitFor();
        assert.equal(await dialog.locator("details[open]").count(), 0, "trace starts collapsed");
        await page.screenshot({ path: "/tmp/issue90-purchase-return.png", fullPage: true });
        await dialog.getByRole("button", { name: "PI-ORIGINAL-100" }).click();
        await page.waitForURL("**/business/purchases?purchaseId=100");
        await page.getByRole("dialog").getByText("PI-ORIGINAL-100").waitFor();
        await page.goto(`${base}/business/purchase-returns`);
      } else {
        await dialog.getByRole("button", { name: "关闭详情弹窗" }).click();
        await dialog.waitFor({ state: "hidden" });
        const trigger = page.locator("tbody tr").filter({ hasText: `${kind === "purchase" ? "PR" : "SR"}-00${statuses.indexOf(status) + 1}` }).first().getByRole("button", { name: "查看", exact: true });
        assert.ok(await trigger.evaluate(node => node === document.activeElement), "closing detail restores focus to its trigger");
      }
    }
  }

  // User cancellation closes the controlled posting preview without any write.
  await visit("purchase");
  const draftRow = page.locator("tbody tr").filter({ hasText: "PR-001" }).first();
  await draftRow.getByRole("button", { name: "过账" }).click();
  const confirm = page.getByRole("dialog");
  await confirm.getByRole("heading", { name: "确认后库存影响", exact: true }).waitFor();
  await confirm.getByText("供应商方向往来余额减少").waitFor();
  await confirm.getByRole("columnheader", { name: "当前有效已退", exact: true }).waitFor();
  const beforeCancel = writes.length;
  await confirm.getByRole("button", { name: "返回核对" }).click();
  await confirm.waitFor({ state: "detached" });
  assert.equal(writes.length, beforeCancel, "closing confirmation must not write");

  // Definite rejection remains visible in the same modal; zero amount and direct order are covered.
  await draftRow.getByRole("button", { name: "过账" }).click();
  rejectPost = true;
  await page.getByRole("dialog").getByRole("button", { name: "确认过账" }).click();
  await page.getByRole("dialog").getByRole("alert").filter({ hasText: "有效退货额度不足" }).waitFor();
  assert.equal(await page.getByRole("dialog").count(), 1, "rejected post remains in modal");
  assert.equal(await page.getByRole("dialog").getByRole("button", { name: "确认过账" }).isDisabled(), true, "failed version cannot be submitted again from the stale preview");
  await page.getByRole("dialog").getByRole("button", { name: "核对退货单详情 · PR-001" }).click();
  await page.getByRole("dialog").getByRole("heading", { name: "采购退货 · PR-001" }).waitFor();
  await page.getByRole("dialog").getByRole("button", { name: "关闭详情弹窗" }).click();
  assert.equal(writes.filter(write => write.path.endsWith("/post")).length, 1);

  await visit("sale");
  const saleDraft = page.locator("tbody tr").filter({ hasText: "SR-001" }).first();
  await saleDraft.getByRole("button", { name: "过账" }).click();
  await page.getByRole("dialog").getByRole("heading", { name: "确认后库存影响", exact: true }).waitFor();
  await page.getByRole("dialog").getByText("零金额，往来余额无变化。").waitFor();
  await page.getByRole("dialog").getByText("零金额，往来余额无变化。").waitFor();
  await page.getByRole("dialog").getByText(/退货不会自动退款/).waitFor();
  await page.screenshot({ path: "/tmp/issue90-sale-return-confirm.png", fullPage: true });

  // Short viewport, long detail, fixed footer, and narrow full-screen width protection.
  await page.getByRole("dialog").getByRole("button", { name: "返回核对" }).click();
  await openDetail("sale", "DRAFT");
  const detail = page.getByRole("dialog");
  await page.screenshot({ path: "/tmp/issue90-sale-return-detail.png", fullPage: true });
  assert.ok(await detail.evaluate(node => { const body = node.querySelector(".overflow-y-auto"); return body && body.scrollHeight > body.clientHeight; }), "long detail body scrolls independently");
  await page.setViewportSize({ width: 390, height: 720 });
  assert.ok(await detail.evaluate(node => { const rect = node.getBoundingClientRect(); return rect.left >= 0 && rect.right <= innerWidth && rect.width <= innerWidth; }), "390px detail dialog stays inside viewport");
  assert.ok(await detail.locator(".overflow-x-auto table").evaluate(table => table.parentElement.scrollWidth > table.parentElement.clientWidth), "narrow detail keeps table scrolling inside its own container");
  await detail.getByRole("button", { name: "关闭详情弹窗" }).click();
  assert.deepEqual(errors, []);
  console.log("PASS mock browser: both return directions and states, quota labels, original navigation, direct order warning, canceled confirmation has zero writes, rejected posting stays open, customer/supplier impact, long short-screen layout and 390px width");
} finally { if (browser) await browser.close(); vite.kill(); }
