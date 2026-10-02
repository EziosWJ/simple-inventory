import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const output = process.env.PRINT_LAYOUT_OUTPUT_DIR ?? "/tmp/simple-inventory-print-layout-pdfs";
const port = 4187;
const url = `http://127.0.0.1:${port}`;
const server = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port), "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: ["ignore", "pipe", "pipe"] });
let serverLog = "";
server.stderr.on("data", chunk => { serverLog += chunk; });
const profile = { name: "太原市新创办公设备有限公司", phone: "0351-1234567", address: "太原市办公设备服务中心 2楼201室" };
const documentNo = "SO20261002-040ba5bc329641dc373e0123456789ab";
function note({ long = false, empty = false, posted = true, historical = false } = {}) {
  const items = Array.from({ length: empty ? 0 : long ? 40 : 22 }, (_, i) => ({
    productId: i + 1, productCode: `G${i + 1}`, productName: long ? `商品${i + 1}：${"办公设备及耗材".repeat(8)}` : `商品${i + 1}`,
    productModel: i % 2 ? `M${i + 1}` : null, productSpecification: long ? "适用于办公打印设备的详细商品规格" : `S${i + 1}`,
    unit: "支", quantity: "2", unitPrice: "49.00", amount: "98.00", remark: long ? "随货核对型号与数量，送达指定楼层办公室" : null,
  }));
  return { documentNo, status: posted ? "POSTED" : "DRAFT", posted, businessDate: "2026-10-02", partnerId: 1,
    partnerName: "太原市房产交易服务中心", deliveryContact: "纪强", deliveryPhone: historical ? null : "13012341235",
    deliveryAddress: historical ? null : long ? "南屯为民服务中心 3楼301室；" + "从东侧入口进入，送至综合服务办公室并联系收货人。".repeat(6) : "南屯为民服务中心 3楼301室",
    ownerName: historical ? "" : profile.name, ownerPhone: historical ? "" : profile.phone, ownerAddress: historical ? "" : profile.address,
    remark: long ? "本单所有明细请逐项验收。".repeat(12) : null, items, totalQuantity: String(items.length * 2), totalAmount: `${items.length * 98}.00` };
}
function statement(direction, empty = false, long = false) {
  const records = Array.from({ length: empty ? 0 : 43 }, (_, i) => ({
    id: i + 1, businessDate: "2026-10-02", effectiveAt: "2026-10-02T06:30:00Z", entryType: i % 3 ? "SALE" : "REVERSAL",
    documentNo: `${i % 3 ? "SO" : "REV"}20261002-${String(i).padStart(32, "0")}`,
    reversedDocumentNo: long || i % 3 === 0 ? documentNo : null, amount: "98.00", balanceAfter: `${(i + 1) * 98}.00`,
  }));
  return { partnerId: 1, partnerName: "太原市房产交易服务中心", direction, from: "2026-10-01T00:00:00Z", to: "2026-11-01T00:00:00Z", openingAmount: "0.00", increaseAmount: `${records.length * 98}.00`, decreaseAmount: "0.00", netChange: `${records.length * 98}.00`, closingAmount: `${records.length * 98}.00`, records };
}
const cases = [
  { name: "delivery-visible", data: note() },
  { name: "delivery-hidden", data: note(), hidden: true },
  { name: "delivery-draft-long", data: note({ long: true, posted: false }) },
  { name: "delivery-long-hidden", data: note({ long: true }), hidden: true },
  { name: "delivery-cancelled", data: { ...note({ posted: false }), status: "CANCELLED" } },
  { name: "delivery-snapshot-empty", data: note({ historical: true }) },
  { name: "delivery-no-lines", data: note({ empty: true }) },
  { name: "statement-customer", data: statement("CUSTOMER"), statement: true },
  { name: "statement-supplier-long", data: statement("SUPPLIER", false, true), statement: true },
  { name: "statement-empty", data: statement("CUSTOMER", true), statement: true },
];
const results = [];
let browser;
try {
  await mkdir(output, { recursive: true });
  for (let attempt = 0; ; attempt++) {
    try { if ((await fetch(`${url}/tests/print-layout.html`)).ok) break; } catch { /* Starting. */ }
    if (attempt === 40) throw new Error(`Vite failed: ${serverLog}`);
    await delay(250);
  }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  for (const scenario of cases) {
    const page = await browser.newPage({ viewport: { width: 1000, height: 1200 }, timezoneId: "Asia/Shanghai" });
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    await page.route("**/api/v1/**", route => {
      const data = route.request().url().includes("print-profile") ? profile : scenario.data;
      return route.fulfill({ json: { code: 200, message: "ok", data } });
    });
    await page.goto(`${url}/tests/print-layout.html?kind=${scenario.statement ? "statement" : "delivery"}&direction=${scenario.data.direction ?? ""}&showAmount=${!scenario.hidden}`);
    await page.locator(".delivery-page").first().waitFor();
    await page.evaluate(() => document.fonts.ready);
    await delay(150);
    await page.emulateMedia({ media: "print" });
    await delay(150);
    const measured = await page.locator(".delivery-note-print").evaluate(root => {
      const sheets = [...root.querySelectorAll(".delivery-page")];
      return sheets.map(sheet => {
        const box = sheet.getBoundingClientRect();
        const title = sheet.querySelector(".delivery-title").getBoundingClientRect();
        const table = sheet.querySelector("table").getBoundingClientRect();
        const meta = sheet.querySelector(".delivery-meta").getBoundingClientRect();
        const footer = sheet.querySelector("footer").getBoundingClientRect();
        const tail = sheet.querySelector(".delivery-tail")?.getBoundingClientRect();
        const number = sheet.querySelector(".delivery-document-no");
        return { height: box.height, centered: Math.abs((title.left + title.right) / 2 - (box.left + box.right) / 2) < 1,
          aligned: Math.abs(table.left - meta.left) < 1 && Math.abs(table.right - meta.right) < 1,
          fits: table.bottom <= footer.top && (!tail || tail.bottom <= footer.top),
          numberFits: !number || number.scrollWidth <= number.clientWidth,
          rows: sheet.querySelectorAll("tbody tr").length };
      });
    });
    for (const sheet of measured) {
      assert(sheet.centered, `${scenario.name}: title not centered`);
      assert(sheet.aligned, `${scenario.name}: edges not aligned`);
      assert(sheet.fits, `${scenario.name}: footer overlaps content`);
      assert(sheet.numberFits, `${scenario.name}: document number too wide`);
      assert(Math.abs(sheet.height - 1122.52) < 2, `${scenario.name}: page grew past A4 (${sheet.height})`);
    }
    assert.equal(await page.locator(".delivery-title").first().evaluate(node => getComputedStyle(node).fontSize), "26.6667px");
    const text = await page.locator(".delivery-note-print").innerText();
    if (scenario.statement) {
      assert(text.includes(profile.name) && text.includes(profile.phone));
      assert(text.includes(scenario.data.direction === "CUSTOMER" ? "客户（应收）" : "供应商（应付）"));
      assert(text.includes("（不含）"));
      assert.equal(await page.locator(".statement-check").count(), 1);
    } else {
      assert.equal(await page.locator(".delivery-receipt-notice").count(), 1);
      assert(text.includes("收货人电话："));
      assert(text.includes(documentNo));
      assert.equal(await page.locator(".delivery-watermark").count(), scenario.data.posted ? 0 : measured.length);
      if (scenario.hidden) assert(!text.includes("¥") && !text.includes("合计金额"));
      if (!scenario.data.ownerName) assert(!text.includes(profile.name), "historical blanks must not use current owner profile");
    }
    const rowCount = measured.reduce((sum, sheet) => sum + sheet.rows, 0);
    assert.equal(rowCount, scenario.statement ? Math.max(scenario.data.records.length, 1) : scenario.data.items.length);
    const numbers = await page.locator("tbody tr td:first-child").allTextContents();
    if (scenario.data.items?.length || scenario.data.records?.length) {
      assert.deepEqual(numbers.map(Number), Array.from({ length: rowCount }, (_, i) => i + 1));
    }
    await page.pdf({ path: `${output}/${scenario.name}.pdf`, printBackground: true, preferCSSPageSize: true });
    await page.locator(".delivery-page").first().screenshot({ path: `${output}/${scenario.name}.png` });
    assert.deepEqual(errors, []);
    results.push({ name: scenario.name, pages: measured.length, hidden: !!scenario.hidden, statement: !!scenario.statement,
      lastText: scenario.statement ? `期末余额：${scenario.data.closingAmount}` : "收货人签字即代表货物数量无误且外观完好" });
    if (scenario.name === "delivery-draft-long") {
      await page.emulateMedia({ media: "screen" });
      await page.getByRole("button", { name: "隐藏金额" }).click();
      await delay(150);
      assert.equal(await page.locator(".col-money").count(), 0);
      assert(await page.locator(".delivery-page").count() < measured.length, "hiding money must remeasure long rows and update pagination");
      const content = await page.locator(".delivery-note-print").innerText();
      assert(!content.includes("¥"));
    }
    await page.close();
  }
  // A failed profile request must not produce a printable statement without its owner.
  const failed = await browser.newPage();
  await failed.route("**/api/v1/**", route => route.request().url().includes("print-profile")
    ? route.fulfill({ status: 503, json: { code: 503, message: "经营者资料暂时不可用" } })
    : route.fulfill({ json: { code: 200, data: statement("CUSTOMER") } }));
  await failed.goto(`${url}/tests/print-layout.html?kind=statement`);
  await failed.getByRole("alert").waitFor();
  assert.equal(await failed.locator(".delivery-page").count(), 0);
  assert(await failed.getByRole("button", { name: "打印 A4" }).isDisabled());
  await failed.close();
  await writeFile(`${output}/result.json`, JSON.stringify(results, null, 2));
  const checked = spawnSync("python3", ["tests/print-layout-pdf.py", output], { cwd: new URL("..", import.meta.url), encoding: "utf8" });
  assert.equal(checked.status, 0, checked.stderr || checked.stdout);
  console.log(checked.stdout);
  console.log(`Print layout acceptance passed. Evidence: ${output}`);
} finally {
  await browser?.close();
  server.kill();
}
