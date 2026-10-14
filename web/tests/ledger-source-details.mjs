// UI-only source inspection: all API calls are intercepted, no business data writes.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const base = "http://127.0.0.1:4235";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4235", "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "ignore" });
const partner = { id: 1, name: "华兴公司", code: "LEDGER-P", type: "COMPANY", isCustomer: true, isSupplier: true, status: 1 };
function makeEntry(id, type, extra = {}) {
  return { id, partnerId: 1, partnerName: partner.name, direction: "CUSTOMER", entryType: type, amount: "245.00", balanceBefore: "0.00", balanceAfter: "245.00", businessDate: "2026-10-14", effectiveAt: "2026-10-14T02:00:00Z", operatorId: 1, operatorName: "经营者", documentNo: "LEDGER-" + id, description: "业务说明", reversesId: null, reversedById: null, ...extra };
}
const entries = [
  makeEntry(1, "SALE", { saleId: 10 }), makeEntry(2, "PURCHASE", { purchaseId: 20, direction: "SUPPLIER" }),
  makeEntry(3, "SALE_RETURN", { saleReturnId: 30, amount: "-20.00" }),
  makeEntry(4, "PURCHASE_RETURN", { purchaseReturnId: 40, direction: "SUPPLIER", amount: "-20.00" }),
  makeEntry(5, "OPENING"), makeEntry(6, "RECEIPT", { amount: "-10.00", paymentMethod: "WECHAT", transactionNo: "WX-123" }),
  makeEntry(7, "PAYMENT", { direction: "SUPPLIER", amount: "-10.00", paymentMethod: "BANK_TRANSFER" }),
  makeEntry(8, "CUSTOMER_REFUND", { paymentMethod: "CASH" }), makeEntry(9, "SUPPLIER_REFUND", { direction: "SUPPLIER", paymentMethod: "ALIPAY" }),
  makeEntry(10, "REVERSAL", { reversesId: 1, amount: "-245.00", reversedDocumentNo: "LEDGER-1" }),
  makeEntry(11, "REVERSAL", { reversesId: 6, amount: "10.00", reversedDocumentNo: "LEDGER-6" }),
  makeEntry(12, "SALE"),
];
function document(id, name, status = "POSTED", count = 2) {
  return { id, documentNo: name, partnerName: partner.name, businessDate: "2026-10-14", status, totalAmount: "245.00", remark: "单据备注", createdByName: "经营者", createTime: "2026-10-14T01:00:00Z", postedByName: "经营者", postedAt: "2026-10-14T02:00:00Z",
    items: Array.from({ length: count }, (_, i) => ({ id: i + 1, productCode: "HIST-" + i, productName: i === 1 ? "安装服务" : "历史硒鼓", productModel: "历史型号", productSpecification: "原始规格", productType: i === 1 ? "SERVICE" : "GOODS", quantity: "1", unit: i === 1 ? "次" : "支", unitPrice: "122.50", amount: "122.50" })) };
}
const sources = {
  "/api/v1/sales/10": document(10, "SO-10"),
  "/api/v1/purchases/20": { ...document(20, "PI-20", "CANCELLED"), cancelReason: "原采购已取消" },
  "/api/v1/sale-returns/30": { ...document(30, "SR-30"), saleNo: "SO-ORIGINAL" },
  "/api/v1/purchase-returns/40": { ...document(40, "PR-40"), purchaseNo: "PI-ORIGINAL" },
};
let browser, failPath = "", delayedPath = "";
const errors = [], requests = [], writes = [];
try {
  for (let i = 0; i < 80; i++) { try { if ((await fetch(base)).ok) break; } catch {} await delay(100); }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript(() => localStorage.setItem("web-auth", JSON.stringify({ token: "ledger-fixture", user: null })));
  await context.route("**/api/**", async route => {
    const request = route.request(), path = new URL(request.url()).pathname;
    if (!path.startsWith("/api/")) return route.continue();
    requests.push(path);
    if (request.method() !== "GET") writes.push(path);
    const respond = (data, status = 200, message = "OK") => route.fulfill({ status, contentType: "application/json", body: JSON.stringify({ code: status, data, message }) });
    if (path === failPath) return respond(null, 503, "来源暂时不可用");
    if (path === delayedPath) await delay(1000);
    if (path === "/api/auth/me") return respond({ id: 1, username: "fixture", roles: [] });
    if (path === "/api/auth/menus" || path.includes("/dict/")) return respond([]);
    if (path.endsWith("/unread-count")) return respond(0);
    if (path === "/api/v1/partners/1") return respond(partner);
    if (path === "/api/v1/partners") return respond({ records: [partner], total: 1 });
    if (path === "/api/v1/partner-balances/entries") return respond({ records: entries, total: 32 });
    if (sources[path]) return respond(sources[path]);
    if (path.startsWith("/api/v1/partner-balances/entries/")) return respond(entries.find(e => e.id === Number(path.split("/").at(-1))));
    return respond({ records: [], total: 0 });
  });
  const page = await context.newPage(); page.setDefaultTimeout(8000); page.on("pageerror", e => errors.push(e.message));
  const url = base + "/business/partner-ledger?partnerId=1&direction=CUSTOMER&page=2";
  await page.goto(url);
  await page.getByRole("button", { name: "LEDGER-1", exact: true }).waitFor();
  assert.equal(requests.filter(p => p in sources).length, 0, "source docs are lazy");
  const row = id => page.locator("table").first().locator("tbody > tr").filter({ hasText: "LEDGER-" + id }).filter({ has: page.getByRole("button", { name: "LEDGER-" + id, exact: true }) }).first();
  for (const entry of entries.slice(0, 11)) {
    await row(entry.id).getByRole("button", { name: "展示细节", exact: true }).click();
    const region = page.getByRole("region", { name: "来源内容细节" });
    await region.waitFor();
    assert.equal(page.url(), url, "expanded row preserves URL and page");
    if (entry.id <= 4 || entry.id === 10) {
      await region.getByRole("columnheader", { name: "商品 / 服务", exact: true }).waitFor();
      await region.getByText("历史硒鼓", { exact: true }).first().waitFor();
      if (entry.id === 1) await region.getByText("安装服务", { exact: true }).waitFor();
      if (entry.id === 2) await region.getByText("原采购已取消").waitFor();
      if (entry.id === 3) await region.getByText("原单：SO-ORIGINAL").waitFor();
      if (entry.id === 4) await region.getByText("原单：PI-ORIGINAL").waitFor();
    } else {
      await region.getByText("业务说明", { exact: true }).waitFor();
      if (entry.id === 6 || entry.id === 11) await region.getByText("微信", { exact: true }).waitFor();
    }
    if (entry.entryType === "REVERSAL") await region.getByRole("heading", { name: "被冲销的原业务" }).waitFor();
    if (entry.id === 1) await page.screenshot({ path: "/tmp/ledger-source-row.png", fullPage: true });
    await row(entry.id).getByRole("button", { name: "收起细节", exact: true }).click();
    assert.equal(await page.getByRole("region", { name: "来源内容细节" }).count(), 0);
    console.log("PASS inline source", entry.entryType, entry.id);
  }
  await page.getByRole("button", { name: "LEDGER-1", exact: true }).click();
  let modal = page.getByRole("dialog");
  await modal.getByLabel("本笔金额影响").waitFor();
  assert.ok(await modal.getByLabel("本笔金额影响").getByText("¥245.00", { exact: true }).evaluate(n => parseFloat(getComputedStyle(n).fontSize) >= 28));
  assert.equal(await modal.getByRole("button", { name: "打开来源业务" }).count(), 0);
  failPath = "/api/v1/sales/10";
  await modal.getByRole("button", { name: "展示细节", exact: true }).click();
  await modal.getByText("来源暂时不可用").waitFor();
  failPath = "";
  await modal.getByRole("button", { name: "重试细节" }).click();
  await modal.getByRole("region", { name: "来源内容细节" }).getByText("SO-10", { exact: true }).waitFor();
  assert.equal(page.url(), url);
  await page.screenshot({ path: "/tmp/ledger-source-modal.png", fullPage: true });
  await modal.getByRole("button", { name: "收起细节", exact: true }).click();
  await modal.getByRole("button", { name: "关闭", exact: true }).click();
  // Collapsing a pending read and opening another record must not show stale source content.
  delayedPath = "/api/v1/sales/10";
  await row(1).getByRole("button", { name: "展示细节" }).click();
  await page.getByText("正在读取单据细节…").waitFor();
  await row(1).getByRole("button", { name: "收起细节" }).click();
  await row(2).getByRole("button", { name: "展示细节" }).click();
  await page.getByRole("region", { name: "来源内容细节" }).getByText("PI-20", { exact: true }).waitFor();
  await delay(1200);
  assert.equal(await page.getByRole("region", { name: "来源内容细节" }).getByText("SO-10", { exact: true }).count(), 0);
  await row(2).getByRole("button", { name: "收起细节" }).click(); delayedPath = "";
  await row(12).getByRole("button", { name: "展示细节" }).click();
  await page.getByText("这笔记录缺少来源关联，无法读取单据细节。").waitFor();
  await row(12).getByRole("button", { name: "收起细节" }).click();
  sources["/api/v1/sales/10"] = document(10, "SO-10", "POSTED", 25);
  await page.getByRole("button", { name: "LEDGER-1", exact: true }).click();
  modal = page.getByRole("dialog"); await modal.getByRole("button", { name: "展示细节" }).click();
  await modal.getByRole("region", { name: "来源内容细节" }).waitFor();
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 650 });
    const box = await modal.getByRole("button", { name: "关闭", exact: true }).boundingBox();
    assert.ok(box.y >= 0 && box.y + box.height <= 650, "long details keep close visible");
    assert.ok(await modal.evaluate(n => n.scrollWidth <= n.clientWidth));
    await page.keyboard.press("Tab"); assert.ok(await modal.evaluate(n => n.contains(document.activeElement)));
  }
  await modal.getByRole("button", { name: "关闭", exact: true }).click();
  await page.setViewportSize({ width: 1440, height: 900 });
  await row(1).getByRole("button", { name: "展示细节" }).click();
  await page.getByRole("region", { name: "来源内容细节" }).waitFor();
  await page.getByRole("button", { name: "上一页", exact: true }).click();
  await page.waitForURL(u => u.searchParams.get("page") === "1");
  await page.getByRole("region", { name: "来源内容细节" }).waitFor({ state: "hidden" });
  assert.equal(await page.getByRole("region", { name: "来源内容细节" }).count(), 0, "page changes clear expansions");
  entries[0].documentNo = "SO20261002-040ba5bc329641dc373e-long-document-number";
  await page.goto(base + "/business/partner-ledger?partnerId=1&direction=CUSTOMER&entryId=1");
  modal = page.getByRole("dialog"); await modal.getByLabel("本笔金额影响").waitFor();
  await page.setViewportSize({ width: 390, height: 650 });
  assert.ok(await modal.evaluate(n => n.scrollWidth <= n.clientWidth), "long document title wraps inside modal");
  await modal.getByRole("button", { name: "关闭", exact: true }).click();
  await page.waitForURL(u => !u.searchParams.has("entryId"));
  assert.deepEqual(writes, []); assert.deepEqual(errors, []);
  console.log("PASS modal summary, lazy expansion/retry, source snapshots/services/returns/reversals, no navigation or writes, stale reads, page context, long short-screen layout");
} finally { await browser?.close(); vite.kill("SIGTERM"); }
