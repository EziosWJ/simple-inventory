// MCP 令牌管理页回归：列表渲染、状态/有效期文案、旧版禁用、查看原值与复制、重置确认。
// 通过固定 fixture 驱动真实页面，除显式 stub 的 PUT/POST 外只允许 GET。
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4198';
const vite = spawn(
  process.execPath,
  ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4198', '--strictPort'],
  { cwd: new URL('..', import.meta.url), stdio: 'ignore' },
);

const records = [
  {
    id: 1, name: '手机客户端', userId: 2, userName: '张三', enabled: true,
    permanent: true, expiresAt: null, expired: false, revoked: false, usable: true,
    secretViewable: true, legacy: false, createdAt: '2026-10-01T00:00:00Z', createdBy: 'admin',
  },
  {
    id: 2, name: '桌面端', userId: 3, userName: '李四', enabled: false,
    permanent: false, expiresAt: '2026-12-31T00:00:00Z', expired: false, revoked: false, usable: false,
    secretViewable: true, legacy: false, createdAt: '2026-10-02T00:00:00Z', createdBy: 'admin',
  },
  {
    id: 3, name: '旧版脚本', userId: 4, userName: '王五', enabled: true,
    permanent: false, expiresAt: '2020-01-01T00:00:00Z', expired: true, revoked: false, usable: false,
    secretViewable: false, legacy: true, createdAt: '2025-01-01T00:00:00Z', createdBy: 'admin',
  },
];
const updatedRecord = { ...records[0], enabled: false };

let browser;
let secretRequested = false;
const pageErrors = [];
try {
  for (let i = 0; i < 80; i++) {
    try { if ((await fetch(base)).ok) break; } catch { /* vite starting */ }
    await delay(100);
  }

  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: base });
  await context.addInitScript(() =>
    localStorage.setItem('web-auth', JSON.stringify({ token: 'isolated-mcp-tokens', user: null })),
  );
  await context.route('**/api/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    if (!path.startsWith('/api/')) return route.continue();

    if (method !== 'GET') {
      // 只允许显式 stub 的写方法，避免测试意外触达真实写操作。
      if (method === 'PUT' && path === '/api/system/mcp/tokens/1/status') {
        assert.deepEqual(request.postDataJSON(), { enabled: false });
        return route.fulfill({ json: { code: 200, message: 'ok', data: updatedRecord } });
      }
      if (method === 'POST' && path === '/api/system/mcp/tokens/1/reset') {
        return route.fulfill({
          json: {
            code: 200, message: 'ok',
            data: { token: 'simcp_reset_value', permanent: true, expiresAt: null, record: records[0] },
          },
        });
      }
      throw new Error(`unexpected ${method} ${path}`);
    }

    let data = { records: [], total: 0, page: 1, pageSize: 10 };
    if (path === '/api/auth/me') data = { id: 1, username: 'admin', roles: [] };
    else if (path === '/api/auth/menus') data = [];
    else if (path.endsWith('/unread-count')) data = 0;
    else if (path === '/api/system/mcp/tokens/page') {
      data = { records, total: records.length, page: 1, pageSize: 10 };
    } else if (path === '/api/system/mcp/tokens/1/secret') {
      secretRequested = true;
      data = { token: 'simcp_view_value_1', permanent: true, expiresAt: null, record: records[0] };
    }
    return route.fulfill({ json: { code: 200, message: 'ok', data } });
  });

  const page = await context.newPage();
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await page.goto(`${base}/system/mcp-token`);
  const rows = page.locator('tbody tr');
  await rows.first().waitFor({ timeout: 8000 }).catch(async (error) => {
    throw new Error(`${error.message}\n${await page.locator('body').innerText()}`);
  });
  assert.equal(await rows.count(), 3, '应渲染三条 fixture 记录');

  // 状态文案
  const rowStates = [];
  for (let i = 0; i < 3; i++) {
    const cells = await rows.nth(i).locator('td').allTextContents();
    rowStates.push(cells[2].trim());
  }
  assert.deepEqual(rowStates, ['启用中', '已停用', '已过期'], '状态文案');

  // 有效期文案
  const expiry0 = (await rows.nth(0).locator('td').allTextContents())[3].trim();
  const expiry1 = (await rows.nth(1).locator('td').allTextContents())[3].trim();
  const expiry2 = (await rows.nth(2).locator('td').allTextContents())[3].trim();
  assert.equal(expiry0, '永久', '永久令牌应显示 永久');
  assert.equal(expiry1, '2026-12-31 08:00:00', '具体有效期按东八区格式化');
  assert(expiry2.includes('2020-01-01 08:00:00') && expiry2.includes('已过期'), '过期令牌在同格标注');

  // legacy 行不可查看原值
  const legacyView = rows.nth(2).getByRole('button', { name: '查看原值' });
  assert.equal(await legacyView.isDisabled(), true, '旧版令牌查看按钮应禁用');
  assert.equal(await legacyView.getAttribute('title'), '旧版令牌，重置后可查看');

  // 查看原值 → 请求独立接口 → 弹窗显示原值 → 复制
  await rows.nth(0).getByRole('button', { name: '查看原值' }).click();
  const valueDialog = page.getByRole('dialog', { name: '令牌原值' });
  await valueDialog.waitFor({ timeout: 5000 });
  assert.equal(secretRequested, true, '应请求 GET :id/secret');
  assert.equal(await valueDialog.getByText('simcp_view_value_1').count(), 0, '默认遮罩隐藏原值');
  await valueDialog.getByRole('button', { name: '显示' }).click();
  await valueDialog.getByText('simcp_view_value_1').waitFor();
  await valueDialog.getByRole('button', { name: '复制' }).click();
  await valueDialog.getByRole('button', { name: '已复制' }).waitFor({ timeout: 5000 });
  await valueDialog.getByRole('button', { name: '知道了' }).click();
  await valueDialog.waitFor({ state: 'detached' });

  // 重置二次确认
  await rows.nth(0).getByRole('button', { name: '重置' }).click();
  const confirmDialog = page.getByRole('dialog', { name: '重置令牌' });
  await confirmDialog.waitFor({ timeout: 5000 });
  assert((await confirmDialog.innerText()).includes('重置后旧令牌立即失效'), '重置确认提示');
  await confirmDialog.getByRole('button', { name: '取消' }).click();
  await confirmDialog.waitFor({ state: 'detached' });

  // 停用二次确认（走显式 stub 的 PUT）
  await rows.nth(0).getByRole('button', { name: '停用' }).click();
  const stopDialog = page.getByRole('dialog', { name: '停用令牌' });
  await stopDialog.waitFor({ timeout: 5000 });
  assert((await stopDialog.innerText()).includes('停用后该令牌的请求会立即失败'), '停用确认提示');
  await stopDialog.getByRole('button', { name: '停用' }).click();
  await stopDialog.waitFor({ state: 'detached' });

  assert.equal(pageErrors.length, 0, `页面不应有 PAGE_ERROR：${pageErrors.join('; ')}`);
  console.log('PASS mcp tokens: 列表/状态/有效期/旧版禁用/查看原值与复制/重置与停用确认；仅固定 fixture 与显式 stub');
} finally {
  await browser?.close();
  vite.kill();
}
