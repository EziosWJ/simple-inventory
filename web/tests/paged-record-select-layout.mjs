// Real purchase form with intercepted APIs; no backend or business data writes.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const base = "http://127.0.0.1:4197";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4197", "--strictPort"], {
  cwd: new URL("..", import.meta.url), stdio: "ignore",
});
const products = Array.from({ length: 12 }, (_, index) => ({
  id: index + 1, code: `G-${index + 1}`, name: "打印机硒鼓", brand: "HP", model: "2612A",
  specification: "标准容量", type: "GOODS", unit: "支", status: 1, purchasePrice: "12.00",
}));
const suppliers = Array.from({ length: 12 }, (_, index) => ({
  id: index + 1, code: `P-${index + 1}`, name: "办公用品供应商", contact: "王先生", phone: "13912341234",
  type: "COMPANY", isSupplier: true, isCustomer: false, status: 1,
}));
let browser;
try {
  for (let n = 0; n < 80; n++) {
    try { if ((await fetch(base)).ok) break; } catch { /* Vite starting */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, timezoneId: "Asia/Shanghai" });
  context.setDefaultTimeout(5000);
  await context.addInitScript(origin => {
    if (location.origin === origin) localStorage.setItem("web-auth", JSON.stringify({ token: "select-layout-fixture", user: null }));
  }, base);
  const errors = [];
  context.on("page", page => page.on("pageerror", error => errors.push(error.message)));
  await context.route("**/api/**", async route => {
    const url = new URL(route.request().url()), path = url.pathname;
    if (!path.startsWith("/api/")) return route.continue();
    assert.equal(route.request().method(), "GET", "selection must not submit the draft");
    let data = [];
    if (path === "/api/auth/me") data = { id: 1, username: "layout-fixture", roles: [] };
    else if (path.endsWith("/unread-count")) data = 0;
    else if (/^\/api\/v1\/(products|partners)$/.test(path)) {
      const source = path.endsWith("products") ? products : suppliers;
      const keyword = url.searchParams.get("keyword") ?? "";
      const filtered = source.filter(record => `${record.code} ${record.name}`.includes(keyword));
      const page = Number(url.searchParams.get("page") ?? 1);
      data = { records: filtered.slice((page - 1) * 10, page * 10), total: filtered.length, page, pageSize: 10 };
    } else if (/^\/api\/v1\/(products|partners)\/\d+$/.test(path)) {
      data = (path.includes("products") ? products : suppliers).find(record => record.id === Number(path.split("/").at(-1)));
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ code: 200, message: "OK", data }) });
  });

  for (const width of [1440, 1024, 465, 320]) {
    const page = await context.newPage();
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`${base}/business/purchases/new`);
    await page.getByRole("heading", { name: "新建采购单", exact: true }).waitFor().catch(async error => {
      console.log({ url: page.url(), text: await page.locator("body").innerText(), errors });
      throw error;
    });
    for (const label of ["供应商", "第 1 行商品"]) {
      const group = page.getByRole("group", { name: label, exact: true });
      const trigger = group.getByRole("button").first();
      await trigger.scrollIntoViewIfNeeded();
      const before = await page.locator("main form").boundingBox();
      await trigger.click();
      const search = group.getByRole("textbox", { name: `搜索${label}`, exact: true });
      await search.waitFor();
      await group.getByText("1-10", { exact: true }).waitFor();
      const after = await page.locator("main form").boundingBox();
      assert.ok(Math.abs(after.height - before.height) < 1, `${width}px ${label}: opening changed form height by ${after.height - before.height}px`);
      const panel = group.locator("[popover]");
      assert.ok(await panel.evaluate(node => node.matches(":popover-open")), "selector must be in the top layer");
      assert.ok(await panel.evaluate(node => {
        const rect = node.getBoundingClientRect();
        return rect.left >= 0 && rect.right <= innerWidth && rect.top >= 0 && rect.bottom <= innerHeight;
      }), `${label} panel must fit the viewport`);
      assert.equal(await search.evaluate(node => node === document.activeElement), true);
      await group.getByRole("button", { name: "下一页", exact: true }).click();
      await group.getByText("11-12", { exact: true }).waitFor();
      await search.fill("no-match");
      await group.getByText("没有匹配资料").waitFor();
      await search.fill(label === "供应商" ? "P-1" : "G-1");
      await group.getByRole("button").filter({ hasText: label === "供应商" ? "P-1 ·" : "G-1 ·" }).last().waitFor();
      await search.press("Enter");
      await search.press("Escape");
      await search.waitFor({ state: "hidden" });
      assert.equal(await trigger.evaluate(node => node === document.activeElement), true);
      await trigger.click();
      await search.waitFor();
      await group.getByRole("button").filter({ hasText: label === "供应商" ? "P-1 ·" : "G-1 ·" }).last().click();
      await search.waitFor({ state: "hidden" });
      assert.match(await trigger.innerText(), label === "供应商" ? /P-1/ : /G-1/);
      await trigger.click();
      await search.waitFor();
      await page.getByRole("heading", { name: "新建采购单", exact: true }).click();
      await search.waitFor({ state: "hidden" });
    }
    assert.equal(await page.locator("#price-0").inputValue(), "12.00");
    if (width === 1440) {
      const group = page.getByRole("group", { name: "供应商", exact: true });
      const trigger = group.getByRole("button").first();
      const panel = group.locator("[popover]");
      const search = group.getByRole("textbox", { name: "搜索供应商", exact: true });
      await page.setViewportSize({ width, height: 440 });
      await trigger.scrollIntoViewIfNeeded();
      // Even a clipping/transformed ancestor must not contain the top layer.
      await group.evaluate(node => {
        const card = node.closest("section");
        card.style.overflow = "hidden";
        card.style.transform = "translateZ(0)";
      });
      await trigger.click();
      await search.fill("");
      await group.getByText("1-10", { exact: true }).waitFor();
      const bounds = await panel.boundingBox(), anchor = await trigger.boundingBox();
      assert.ok(bounds.y + bounds.height <= anchor.y, "insufficient space below must flip the panel upwards");
      assert.ok(await panel.evaluate(node => {
        const r = node.getBoundingClientRect();
        return node.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2));
      }), "clipping ancestors must not cover the panel");
      const list = panel.locator("div.overflow-y-auto");
      assert.ok(await list.evaluate(node => node.scrollHeight > node.clientHeight), "long results must scroll inside the panel");
      await list.evaluate(node => { node.scrollTop = 100; });
      assert.ok(Math.abs((await panel.boundingBox()).y - bounds.y) < 1, "result scrolling must not move the panel");
      await page.evaluate(() => window.scrollBy(0, 20));
      await page.waitForFunction(id => {
        const group = document.querySelector(`[role="group"][aria-label="${id}"]`);
        const panel = group.querySelector("[popover]"), trigger = group.querySelector("button");
        return Math.abs(panel.getBoundingClientRect().bottom + 4 - trigger.getBoundingClientRect().top) < 1;
      }, "供应商");
      await group.getByRole("button", { name: "收起搜索", exact: true }).focus();
      await page.keyboard.press("Tab");
      await search.waitFor({ state: "hidden" });
      console.log("PASS top layer escapes clipping, flips upwards, follows page scrolling, scrolls results independently, closes on Tab out");
    }
    if (width === 465) {
      await page.getByRole("group", { name: "供应商", exact: true }).getByRole("button").first().click();
      await page.getByRole("textbox", { name: "搜索供应商", exact: true }).waitFor();
      await page.getByRole("group", { name: "供应商", exact: true }).locator("button[aria-pressed]").first().waitFor();
      await page.screenshot({ path: "/tmp/paged-record-select-465.png", fullPage: true });
    }
    await page.close();
    console.log(`PASS ${width}px: stable form layout, top layer, viewport fit, pagination/search, keyboard, selection, outside dismissal`);
  }
  assert.deepEqual(errors, []);
} finally {
  await browser?.close();
  vite.kill("SIGTERM");
}
