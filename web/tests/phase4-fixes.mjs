import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const base = "http://127.0.0.1:4178";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4178", "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "ignore" });
const pageData = records => ({ records, total: records.length, page: 1, pageSize: 10 });
try {
  for (let n = 0; n < 40; n++) { try { if ((await fetch(`${base}/tests/phase4-fixes.html`)).ok) break; } catch { /* starting */ } await delay(250); }
  const browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  try {
    for (const kind of ["purchase", "sale"]) {
      const page = await browser.newPage();
      const itemKey = `${kind}ItemId`, sourceKey = `${kind}Id`, noKey = `${kind}No`;
      const items = [11,22].map((id, i) => ({ [itemKey]: id, productCode: i ? "B" : "A", productName: i ? "商品B" : "商品A", unit: "台", originalQuantity: "10", returnedQuantity: "0", remainingQuantity: "10", unitPrice: "1.00", priorReturnAmount: "0.00" }));
      let draft = { id: 1, [sourceKey]: 3, [noKey]: "ORIGINAL", documentNo: "RETURN", status: "DRAFT", version: 1, businessDate: "2026-10-01", items: [{ ...items[1], quantity: "2", remark: "保留行备注" }] };
      let submitted;
      await page.route("**/api/v1/**", async route => {
        const req = route.request(), path = new URL(req.url()).pathname;
        let data;
        if (req.method() === "PUT") { submitted = req.postDataJSON(); draft = { ...draft, ...submitted, version: draft.version+1 }; data = draft; }
        else if (path.includes("/source/")) data = { [sourceKey]: 3, [noKey]: "ORIGINAL", items };
        else if (path.endsWith("/1")) data = draft;
        else data = pageData([draft]);
        await route.fulfill({ json: { code: 200, message: "success", data } });
      });
      await page.goto(`${base}/tests/phase4-fixes.html?kind=${kind}-returns`);
      await page.getByRole("button", { name: "编辑", exact: true }).click();
      await page.getByRole("dialog").locator('input[inputmode="decimal"]').first().waitFor();
      let inputs = page.getByRole("dialog").locator('input[inputmode="decimal"]');
      assert.deepEqual(await inputs.evaluateAll(nodes=>nodes.map(n=>n.value)), ["0", "2"], `${kind}: B quantity must stay under B`);
      await inputs.nth(1).fill("3");
      await page.getByRole("button", { name: "保存", exact: true }).click();
      await page.getByRole("dialog").waitFor({ state: "hidden" });
      assert.deepEqual(submitted.items, [{ [itemKey]: 22, quantity: "3", remark: "保留行备注" }]);
      await page.getByRole("button", { name: "编辑", exact: true }).click();
      await page.getByRole("dialog").locator('input[inputmode="decimal"]').first().waitFor();
      inputs = page.getByRole("dialog").locator('input[inputmode="decimal"]');
      assert.deepEqual(await inputs.evaluateAll(nodes=>nodes.map(n=>n.value)), ["0", "3"]);
      await inputs.nth(0).fill("1"); await inputs.nth(1).fill("");
      await page.getByRole("button", { name: "保存", exact: true }).click();
      await page.getByRole("dialog").waitFor({ state: "hidden" });
      assert.deepEqual(submitted.items, [{ [itemKey]: 11, quantity: "1" }]);
      await page.getByRole("button", { name: "编辑", exact: true }).click();
      await inputs.first().waitFor();
      assert.deepEqual(await inputs.evaluateAll(nodes=>nodes.map(n=>n.value)), ["1", "0"]);
      await page.close(); console.log(`#32 ${kind} subset/edit/add/clear/read passed`);
    }
    for (const [kind, type, newType] of [["purchases", "GOODS", "GOODS"], ["sales", "GOODS", "GOODS"], ["sales", "SERVICE", "SERVICE"], ["sales", "SERVICE", "GOODS"]]) {
      const page = await browser.newPage({ timezoneId: "Asia/Shanghai" });
      await page.clock.setFixedTime(new Date("2026-10-01T16:15:00Z"));
      const product = { id: 1, code: "UNIT", name: "单位变更商品", type: newType, unit: "次", status: 1 };
      const partner = { id: 1, name: "客户", contact: "档案联系人", phone: "123", address: "档案地址", status: 1 };
      let draft = { id: 1, documentNo: "DRAFT", partnerId: 1, partnerName: "客户", status: "DRAFT", version: 1, businessDate: "2026-10-01", directDelivery: false, directDocuments: [], items: [{ id: 11, productId: 1, productType: type, unit: "小时", quantity: "2.5", unitPrice: "0.00" }] };
      let submitted;
      await page.route("**/api/v1/**", async route => {
        const req = route.request(), path = new URL(req.url()).pathname;
        let data;
        if (["PUT", "POST"].includes(req.method())) { submitted = req.postDataJSON(); draft = { ...draft, ...submitted }; data = draft; }
        else if (path.endsWith("/products")) data = pageData([product]);
        else if (path.endsWith("/partners")) data = pageData([partner]);
        else if (path.endsWith("/1")) data = draft;
        else data = pageData(path.endsWith(`/${kind}`) ? [draft] : []);
        await route.fulfill({ json: { code: 200, message: "success", data } });
      });
      await page.goto(`${base}/tests/phase4-fixes.html?kind=${kind}`);
      await page.getByRole("button", { name: "编辑", exact: true }).click();
      const dialog = page.getByRole("dialog");
      await dialog.getByRole("button", { name: "确认使用新类型和单位" }).waitFor();
      assert.match(await dialog.innerText(), /草稿类型 \/ 单位：.*小时/);
      assert.equal(await dialog.getByRole("button", { name: "保存", exact: true }).isDisabled(), true);
      assert.equal(submitted, undefined);
      assert.equal(draft.items[0].unit, "小时");
      await dialog.getByRole("button", { name: "确认使用新类型和单位" }).click();
      await dialog.getByRole("button", { name: "保存", exact: true }).click();
      await dialog.waitFor({ state: "hidden" });
      assert.equal(submitted.items[0].unit, "次");
      assert.equal(submitted.items[0].productType, newType);
      assert.equal(submitted.items[0].quantity, "2.5");
      console.log(`#33 ${kind} ${type} -> ${newType} explicit confirmation passed`);
      if (kind === "sales") {
        await page.getByRole("button", { name: "新建销售草稿" }).click();
        assert.equal(await dialog.locator('input[type="date"]').inputValue(), "2026-10-02");
        await dialog.locator("select").first().selectOption("1");
        for (const field of ["送货联系人", "送货电话", "送货地址"]) await dialog.getByLabel(field).fill("");
        await dialog.locator("select").last().selectOption("1");
        await dialog.getByRole("button", { name: "保存", exact: true }).click();
        await dialog.waitFor({ state: "hidden" });
        assert.equal(submitted.businessDate, "2026-10-02");
        for (const field of ["deliveryContact", "deliveryPhone", "deliveryAddress"]) assert.equal(submitted[field], "");
        console.log("#34 Shanghai 00:15 local date and explicit empty request passed");
      }
      await page.close();
    }
  } finally { await browser.close(); }
} finally { vite.kill(); }
