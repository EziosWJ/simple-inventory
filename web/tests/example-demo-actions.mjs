// 页面示例的三个 Demo 页不允许出现「点了没反应」的控件：每个可见按钮都要产生可见结果。
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4218;
const base = `http://127.0.0.1:${port}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port), "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "ignore" });

let browser;
try {
  for (let i = 0; i < 100; i++) { try { if ((await fetch(base)).ok) break; } catch { /* Vite is still starting. */ } await delay(100); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, timezoneId: "Asia/Shanghai" });
  await context.addInitScript(() => localStorage.setItem("web-auth", JSON.stringify({ token: "example-demo-actions", user: { id: 1, username: "demo", roles: [] } })));
  await context.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith("/api/")) return route.fallback();
    let data = [];
    if (path === "/api/auth/me") data = { id: 1, username: "demo", nickname: "演示", roles: [] };
    else if (path.endsWith("/unread-count")) data = 0;
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ code: 200, message: "OK", data }) });
  });

  const errors = [];
  const page = await context.newPage();
  page.setDefaultTimeout(8000);
  page.on("pageerror", (error) => errors.push(error.message));
  const dialog = () => page.getByRole("dialog");
  const toast = () => page.getByRole("region", { name: "消息通知" });
  const row = (text) => page.getByRole("row").filter({ hasText: text });

  // 标准列表页：行操作、新建、刷新、查询与重置都必须有结果。
  await page.goto(`${base}/examples/list`, { waitUntil: "networkidle" });

  await row("客户资料同步").getByRole("button", { name: "查看" }).click();
  await dialog().getByRole("heading", { name: "客户资料同步" }).waitFor();
  await dialog().getByRole("button", { name: "关闭", exact: true }).click();
  await dialog().waitFor({ state: "detached" });

  await row("客户资料同步").getByRole("button", { name: "编辑" }).click();
  await dialog().getByLabel("业务名称").fill("客户资料同步（已改）");
  await dialog().getByRole("button", { name: "保存" }).click();
  await row("客户资料同步（已改）").waitFor();
  await toast().getByText(/已保存/).waitFor();

  await page.getByRole("button", { name: "新建记录" }).click();
  await dialog().getByLabel("业务名称").fill("演示新建记录");
  await dialog().getByLabel("业务编码").fill("demo_created");
  await dialog().getByRole("button", { name: "创建" }).click();
  await row("演示新建记录").waitFor();
  await toast().getByText(/已新建/).waitFor();

  await page.getByRole("button", { name: "刷新" }).click();
  await toast().getByText(/演示数据已重新载入/).waitFor();

  await page.getByPlaceholder("搜索名称或编码").fill("归档");
  await page.getByRole("button", { name: "查询" }).click();
  await row("归档任务清理").waitFor();
  assert.equal(await row("演示新建记录").count(), 0, "查询应过滤掉不匹配的记录");
  await page.getByRole("button", { name: "重置" }).click();
  await row("演示新建记录").waitFor();

  // 详情页：返回回到列表页，编辑写回页面内容。
  await page.goto(`${base}/examples/detail`, { waitUntil: "networkidle" });
  await page.getByRole("button", { name: "编辑" }).click();
  await dialog().getByLabel("标题").fill("企业客户开通申请（已改）");
  await dialog().getByRole("button", { name: "保存" }).click();
  await page.getByRole("heading", { name: "企业客户开通申请（已改）" }).waitFor();
  await page.getByRole("button", { name: "返回" }).click();
  await page.waitForURL(`${base}/examples/list`);

  // 左树右表：查询、重置、新增成员与刷新都必须有结果。
  await page.goto(`${base}/examples/tree-table`, { waitUntil: "networkidle" });
  await page.getByPlaceholder("搜索姓名或账号").fill("林远");
  await page.getByRole("button", { name: "查询" }).click();
  await row("林远").waitFor();
  assert.equal(await row("周宁").count(), 0, "查询应按关键词过滤成员");
  await page.getByRole("button", { name: "重置" }).click();
  await row("周宁").waitFor();

  await page.getByRole("button", { name: "新增成员" }).click();
  await dialog().getByLabel("姓名").fill("演示新成员");
  await dialog().getByLabel("账号").fill("demo_member");
  await dialog().getByRole("button", { name: "新增", exact: true }).click();
  await row("演示新成员").waitFor();
  await toast().getByText(/已新增成员/).waitFor();

  await page.getByRole("button", { name: "刷新" }).click();
  await toast().getByText(/演示数据已重新载入/).waitFor();

  assert.deepEqual(errors, [], `示例页不应抛错：${errors.join("; ")}`);
  console.log("example demo actions: OK");
} finally {
  await browser?.close();
  vite.kill();
}
