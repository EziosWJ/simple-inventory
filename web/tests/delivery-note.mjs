import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4176;
const baseUrl = `http://127.0.0.1:${port}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port)], {
  cwd: new URL("..", import.meta.url),
  stdio: ["ignore", "pipe", "pipe"],
});

async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      const response = await fetch(`${baseUrl}/tests/delivery-note.html`);
      if (response.ok) return;
    } catch {
      // Vite is still starting.
    }
    await delay(250);
  }
  throw new Error("Vite test server did not start");
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

try {
  await waitForServer();
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH }
      : {}),
  });
  try {
    const page = await browser.newPage({ viewport: { width: 900, height: 1200 } });
    await page.goto(`${baseUrl}/tests/delivery-note.html`, { waitUntil: "networkidle" });

    // 22 lines at 18 rows per page must span exactly two A4 sheets.
    const sheetCount = await page.locator(".delivery-page").count();
    assert(sheetCount === 2, `expected 2 pages, got ${sheetCount}`);
    const perPage = await page.locator(".delivery-page").first().locator("tbody tr").count();
    assert(perPage === 18, `expected 18 rows on the first page, got ${perPage}`);
    const lastPageRows = await page.locator(".delivery-page").nth(1).locator("tbody tr").count();
    assert(lastPageRows === 4, `expected 4 rows on the last page, got ${lastPageRows}`);

    // Page numbers and the running document number print on every page.
    const firstFoot = await page.locator(".delivery-page").first().locator(".delivery-foot").innerText();
    const lastFoot = await page.locator(".delivery-page").nth(1).locator(".delivery-foot").innerText();
    assert(firstFoot.includes("第 1 / 2 页"), `first page footer missing page number: ${firstFoot}`);
    assert(lastFoot.includes("第 2 / 2 页"), `last page footer missing page number: ${lastFoot}`);

    // Totals and the signature block appear only once, on the last page.
    const totalCount = await page.locator(".delivery-total").count();
    assert(totalCount === 1, `totals must appear once, got ${totalCount}`);
    const lastTotal = await page.locator(".delivery-page").nth(1).locator(".delivery-total").innerText();
    assert(lastTotal.includes("44") && lastTotal.includes("66.00"), `unexpected totals: ${lastTotal}`);
    const signatureCount = await page.locator(".delivery-sign").count();
    assert(signatureCount === 1, `signature block must appear once, got ${signatureCount}`);

    // A posted note shows no watermark.
    assert(await page.locator(".delivery-watermark").count() === 0, "posted note must not show a watermark");

    // Hiding amounts removes unit price, line amount and the total amount, not
    // just one column.
    const moneyHeaderBefore = await page.locator(".col-money").count();
    assert(moneyHeaderBefore > 0, "amount columns must be present before hiding");
    await page.getByTestId("toggle-amount").click();
    await page.waitForFunction(() => document.querySelectorAll(".col-money").length === 0);
    const bodyText = await page.locator(".delivery-note-print").innerText();
    assert(!bodyText.includes("¥"), `hidden mode still leaks money: ${bodyText}`);
    assert(!bodyText.includes("合计金额"), `hidden mode still shows total amount label: ${bodyText}`);
    assert(bodyText.includes("合计数量"), "hidden mode must keep the quantity total");

    // A draft carries the not-posted watermark.
    await page.getByTestId("use-draft").click();
    await page.locator(".delivery-watermark").first().waitFor();
    const watermark = await page.locator(".delivery-watermark").first().innerText();
    assert(watermark.includes("未过账"), `draft watermark unexpected: ${watermark}`);

    // The printable page is A4 portrait with the sheet sized to a full page.
    const box = await page.locator(".delivery-page").first().boundingBox();
    assert(box !== null, "delivery page has no layout box");
    const mm = box.width / 3.7795275591;
    assert(Math.abs(mm - 210) < 2, `expected ~210mm wide A4 sheet, got ${mm.toFixed(1)}mm`);
    const printCount = await page.locator(".delivery-page").evaluateAll((nodes) =>
      nodes.map((n) => getComputedStyle(n).pageBreakAfter).length,
    );
    assert(printCount === 2, "both sheets must carry page-break styling");

    console.log("delivery-note print acceptance passed");
  } finally {
    await browser.close();
  }
} finally {
  vite.kill();
}
