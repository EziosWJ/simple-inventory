// 单据弹窗示例页只使用页面内存数据；打开、取消与确认都不得产生写请求。
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4217;
const base = `http://127.0.0.1:${port}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port), "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "ignore" });

const menus = [{
  id: 12, parentId: 0, menuName: "页面示例", menuType: "DIR", path: "/examples", icon: "FileText", permissionCode: "examples", sortOrder: 4, visible: 1,
  children: [{ id: 41, parentId: 12, menuName: "单据弹窗 Demo", menuType: "MENU", path: "/examples/document-dialog", icon: "ClipboardList", permissionCode: "examples:document-dialog", sortOrder: 7, visible: 1, children: [] }],
}];

async function eventually(check, message, timeout = 5000) {
  const deadline = Date.now() + timeout;
  for (;;) {
    try { await check(); return; } catch (error) { if (Date.now() > deadline) throw new Error(`${message}: ${error.message}`); await delay(100); }
  }
}

const scrollToBottom = (el) => {
  const table = el.querySelector("table");
  let scroller = table.parentElement;
  while (scroller && scroller.scrollHeight <= scroller.clientHeight) scroller = scroller.parentElement;
  scroller.scrollTop = scroller.scrollHeight;
};

const frame = (el) => {
  const header = el.querySelector("header").getBoundingClientRect();
  const footer = el.querySelector("footer").getBoundingClientRect();
  return { headerTop: Math.round(header.top), footerBottom: Math.round(footer.bottom), viewportHeight: window.innerHeight };
};

let browser;
try {
  for (let i = 0; i < 100; i++) { try { if ((await fetch(base)).ok) break; } catch { /* Vite is still starting. */ } await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript(() => localStorage.setItem("web-auth", JSON.stringify({ token: "Bearer document-dialog-demo", user: { id: 1, username: "demo", nickname: "演示", roles: [] } })));

  const writes = [];
  await context.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (!path.startsWith("/api/")) return route.fallback();
    if (request.method() !== "GET") writes.push(`${request.method()} ${path}`);
    const respond = (value) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ code: 200, message: "OK", data: value }) });
    if (path === "/api/auth/me") return respond({ id: 1, username: "demo", nickname: "演示", roles: [] });
    if (path === "/api/auth/menus") return respond(menus);
    if (path.endsWith("unread-count")) return respond(0);
    return respond([]);
  });

  const errors = [];
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(String(error)));

  // 1. 菜单入口 → 路由
  await page.goto(`${base}/dashboard`);
  await page.getByRole("button", { name: "页面示例" }).click();
  await page.getByRole("link", { name: "单据弹窗 Demo" }).click();
  await page.waitForURL("**/examples/document-dialog");
  await page.getByRole("heading", { name: "单据弹窗 Demo" }).waitFor();
  console.log("✓ 页面示例菜单入口可打开 /examples/document-dialog");

  // 2. 单据详情：身份/金额/状态在前，明细在折叠辅助之前，折叠区默认收起
  await page.getByRole("button", { name: "查看单据详情" }).click();
  const detail = page.getByRole("dialog", { name: /演示单据 · DEMO-20261014-001/ });
  await detail.waitFor();
  await detail.getByText("示例往来单位 · 业务日期 2026-10-14 · 草稿").waitFor();
  await detail.getByText("¥1426.00").waitFor();
  await detail.getByText("4 行").waitFor();
  assert.ok(await detail.getByText("服务", { exact: true }).count() >= 1, "服务明细应有清晰标识");
  const order = await detail.evaluate((el) => {
    const overview = el.querySelector("section[aria-label='单据概览']");
    const items = el.querySelector("section[aria-label='商品明细']");
    const disclosures = [...el.querySelectorAll("details")];
    const delivery = disclosures.find((node) => node.querySelector("summary")?.textContent?.includes("送货资料"));
    const records = disclosures.find((node) => node.querySelector("summary")?.textContent?.includes("操作记录"));
    const follows = (first, second) => Boolean(first && second && (first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING) > 0);
    return {
      itemsAfterOverview: follows(overview, items),
      disclosureAfterItems: follows(items, delivery),
      recordsAfterDelivery: follows(delivery, records),
      openDisclosures: el.querySelectorAll("details[open]").length,
    };
  });
  assert.deepEqual(order, { itemsAfterOverview: true, disclosureAfterItems: true, recordsAfterDelivery: true, openDisclosures: 0 }, "信息顺序与折叠默认收起");
  console.log("✓ 详情：概览 → 明细 → 折叠辅助 → 操作记录，折叠区默认收起");

  // 3. 长单：正文真实滚动，头部与底部操作区保持不动
  await detail.getByRole("button", { name: "关闭", exact: true }).click();
  await detail.waitFor({ state: "hidden" });
  await page.locator("#demo-scale").selectOption("long");
  await page.getByRole("button", { name: "查看单据详情" }).click();
  const longDetail = page.getByRole("dialog", { name: /演示单据 · DEMO-20261014-001/ });
  await longDetail.getByText("24 行").waitFor();
  await page.setViewportSize({ width: 1024, height: 600 });
  const overflowing = await longDetail.evaluate((el) => {
    const table = el.querySelector("table").getBoundingClientRect();
    const footer = el.querySelector("footer").getBoundingClientRect();
    return table.bottom > footer.top;
  });
  assert.ok(overflowing, "24 行明细应真实溢出正文区域");
  const before = await longDetail.evaluate(frame);
  assert.ok(before.headerTop >= 0 && before.footerBottom <= before.viewportHeight, `头尾应在视口内：${JSON.stringify(before)}`);
  await longDetail.evaluate(scrollToBottom);
  await delay(200);
  const after = await longDetail.evaluate(frame);
  assert.deepEqual(after, before, "正文滚动时头部与底部操作区位置不变");
  const lastRowInView = await longDetail.evaluate((el) => {
    const rows = [...el.querySelectorAll("tbody tr")];
    const box = rows[rows.length - 1].getBoundingClientRect();
    return box.top >= 0 && box.bottom <= window.innerHeight;
  });
  assert.ok(lastRowInView, "滚到底后末行应在视口内");
  console.log("✓ 长单：正文真实滚动，头部与底部操作区固定");

  // 4. 过账确认：取消不产生写请求
  await page.setViewportSize({ width: 1440, height: 900 });
  await longDetail.getByRole("button", { name: "关闭", exact: true }).click();
  await longDetail.waitFor({ state: "hidden" });
  await page.getByRole("button", { name: "保存并过账" }).click();
  const confirm = page.getByRole("dialog", { name: "确认过账演示单据" });
  await confirm.waitFor();
  await confirm.getByText("即将提交的明细").waitFor();
  await confirm.getByRole("button", { name: "取消" }).click();
  await confirm.waitFor({ state: "hidden" });
  assert.deepEqual(writes, [], "打开与取消确认不应产生写请求");
  console.log("✓ 确认：正文与明细默认可见，取消不产生写请求");

  // 5. 提交失败：错误默认可见且确认被锁定
  await page.getByRole("switch", { name: "模拟提交失败" }).click();
  await page.getByRole("button", { name: "保存并过账" }).click();
  const failed = page.getByRole("dialog", { name: "确认过账演示单据" });
  await failed.getByRole("button", { name: "确认过账" }).click();
  await failed.getByRole("alert").waitFor();
  const locked = failed.getByRole("button", { name: "请刷新后重新核对" });
  assert.equal(await locked.isDisabled(), true, "失败后确认按钮应锁定");
  await failed.getByRole("button", { name: "关闭确认" }).click();
  await failed.waitFor({ state: "hidden" });
  console.log("✓ 失败：错误提示默认可见，确认按钮锁定并改名");

  // 6. 正常提交：关闭确认并更新页面状态
  await page.getByRole("switch", { name: "模拟提交失败" }).click();
  await page.getByRole("button", { name: "保存并过账" }).click();
  const posted = page.getByRole("dialog", { name: "确认过账演示单据" });
  await posted.getByRole("button", { name: "确认过账" }).click();
  await posted.waitFor({ state: "hidden" });
  await eventually(async () => assert.ok(await page.getByText("已过账", { exact: true }).count() >= 1), "页面状态应变为已过账");
  await page.getByRole("button", { name: "查看单据详情" }).click();
  const postedDetail = page.getByRole("dialog", { name: /演示单据 · DEMO-20261014-001/ });
  await postedDetail.getByText("示例往来单位 · 业务日期 2026-10-14 · 已过账").waitFor();
  assert.deepEqual(writes, [], "确认过账也不应产生写请求");
  console.log("✓ 正常提交：确认关闭后详情表达已过账状态");

  // 7. 窄屏：单据弹窗占满视口且不产生横向滚动
  await page.setViewportSize({ width: 390, height: 720 });
  await delay(300);
  const narrow = await postedDetail.evaluate((el) => {
    const box = el.getBoundingClientRect();
    const scroller = [...el.querySelectorAll("*")].find((node) => node.scrollWidth > node.clientWidth + 1);
    return {
      left: Math.round(box.left),
      right: Math.round(box.right),
      innerWidth: window.innerWidth,
      pageScrollWidth: document.documentElement.scrollWidth,
      tableScrollsInContainer: Boolean(scroller) && scroller.scrollWidth > scroller.clientWidth,
    };
  });
  assert.ok(narrow.left >= 0 && narrow.right <= narrow.innerWidth, `窄屏弹窗不应溢出视口：${JSON.stringify(narrow)}`);
  assert.equal(narrow.pageScrollWidth, narrow.innerWidth, `弹窗不应撑宽页面：${JSON.stringify(narrow)}`);
  assert.ok(narrow.tableScrollsInContainer, "明细表应只在自身容器内横向滚动");
  await postedDetail.getByRole("button", { name: "关闭", exact: true }).click();
  await postedDetail.waitFor({ state: "hidden" });
  console.log("✓ 390px：单据弹窗占满视口，无横向滚动");

  assert.deepEqual(errors, []);
  assert.deepEqual(writes, []);
  console.log("\n单据弹窗示例页验收通过。");
} finally {
  await browser?.close();
  vite.kill();
}
