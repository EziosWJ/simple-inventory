// Public document layout remains opt-in; ordinary system dialogs keep their behavior.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4210';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4210', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
const user = { id: 2, username: 'ordinary-user', nickname: '兼容回归用户', status: 1, isBuiltin: 0, roles: [] };
const errors = [], writes = [];
let browser;
try {
  for (let i = 0; i < 80; i++) {
    try { if ((await fetch(base)).ok) break; } catch { /* Vite starting */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(() => localStorage.setItem('web-auth', JSON.stringify({ token: 'document-dialog-fixture', user: null })));
  await context.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (route.request().method() !== 'GET') {
      writes.push(path);
      return route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ code: 400, message: 'unexpected write' }) });
    }
    let data = [];
    if (path === '/api/auth/me') data = { id: 1, username: 'fixture', roles: [] };
    else if (path === '/api/system/user/page') data = { records: [user], total: 1, page: 1, pageSize: 10 };
    else if (path.endsWith('/unread-count')) data = 0;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'OK', data }) });
  });
  const page = await context.newPage();
  page.on('pageerror', error => errors.push(error.message));
  page.on('requestfailed', request => errors.push(`${request.url()}: ${request.failure()?.errorText}`));
  page.on('console', message => { if (message.type() === 'error') console.log(message.text()); });
  await page.goto(`${base}/system/user`);
  await page.getByRole('button', { name: '新建用户', exact: true }).click({ timeout: 10000 }).catch(async error => {
    console.log({ errors, body: await page.locator('body').innerText() });
    throw error;
  });
  const form = page.getByRole('dialog', { name: '新建用户', exact: true });
  await form.waitFor();
  assert.equal(Math.round((await form.boundingBox()).width), 760, 'ordinary form retains its 760px width');
  await page.keyboard.press('Escape');
  assert.ok(await form.isVisible(), 'system form retains its explicit Escape-close policy');
  await form.getByRole('button', { name: '取消', exact: true }).click();
  await form.waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: '删除', exact: true }).click();
  const confirmation = page.getByRole('dialog', { name: '删除用户', exact: true });
  await confirmation.waitFor();
  assert.equal(Math.round((await confirmation.boundingBox()).width), 480, 'short confirmation remains compact');
  await confirmation.getByRole('button', { name: '取消', exact: true }).click();
  await confirmation.waitFor({ state: 'hidden' });
  assert.deepEqual(writes, [], 'opening and dismissing dialogs never writes');
  assert.deepEqual(errors, []);
  console.log('PASS document dialog opt-in compatibility: system form size/close policy, compact confirmation and no writes');
} finally {
  await browser?.close();
  vite.kill('SIGTERM');
}
