import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
const api = process.env.DIRECT_ORDER_API_URL ?? "http://127.0.0.1:18113", base = "http://127.0.0.1:4198", prefix = `DR-${Date.now()}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4198", "--strictPort"], { cwd: new URL("..", import.meta.url), env: { ...process.env, VITE_API_BASE_URL: api }, stdio: "ignore" });
let token, browser, page;
async function call(path, body, method = body === undefined ? "GET" : "POST") {
  const res = await fetch(`${api}/api${path}`, { method, headers: { Authorization: token ?? "", "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
  const json = await res.json(); assert.equal(res.status, 200, `${path}: ${json.message}`); return json.data;
}
async function select(label, code) {
  const group = page.getByRole("group", { name: label, exact: true }); await group.getByRole("combobox").fill(code);
  const option = page.getByRole("option").filter({ has: page.getByText(code, { exact: true }) }); const name = await option.locator("strong").innerText(); await option.click();
  await page.waitForFunction(({label,name}) => [...document.querySelectorAll('[role="group"]')].find(g=>g.getAttribute("aria-label")===label)?.querySelector('[role="combobox"]')?.value===name,{label,name});
}
async function sourcePick(label, documentNo) {
  const group = page.getByRole("group", { name: label, exact: true }); await group.getByRole("button").first().click();
  const panel = page.getByRole("region", { name: `${label}搜索面板`, exact: true }); await panel.getByRole("textbox", { name: `搜索${label}`, exact: true }).fill(documentNo);
  await panel.getByRole("button").filter({ hasText: documentNo }).click();
  await panel.waitFor({ state: "hidden" });
}
try {
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break;}catch{}await delay(300);}
  token=(await call("/auth/login",{username:"admin",password:process.env.ACCEPTANCE_PASSWORD??"admin123"})).tokenValue;
  const partner=await call("/v1/partners",{code:prefix,name:"直送与来源经营者",type:"COMPANY",isCustomer:true,isSupplier:true});
  const goods=await call("/v1/products",{code:`${prefix}-G`,name:"内存配件",type:"GOODS",unit:"条",purchasePrice:"10.00",salePrice:"15.00"});
  browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH}:{})});
  const context=await browser.newContext({viewport:{width:1440,height:1100}});await context.addInitScript(t=>localStorage.setItem("web-auth",JSON.stringify({token:t,user:null})),token);page=await context.newPage();const errors=[];page.on("pageerror",e=>errors.push(e.message));
  // New standalone direct delivery: save, confirm, inspect generated children and print.
  await page.goto(`${base}/business/direct-orders/new`);await select("供应商",partner.code);await select("客户",partner.code);await select("第 1 行商品",goods.code);
  await page.locator("#direct-qty-0").fill("2");await page.getByRole("button",{name:"保存草稿",exact:true}).click();await page.waitForURL(/direct-orders\/\d+\/edit$/);
  const directID=Number(new URL(page.url()).pathname.split("/").at(-2));
  await page.getByRole("button",{name:"确认直送",exact:true}).click();await page.getByRole("dialog").getByRole("button",{name:"确认并同时过账",exact:true}).click();
  await page.getByRole("status",{name:"操作结果",exact:true}).filter({hasText:"直送已确认"}).waitFor();
  const direct=await call(`/v1/direct-orders/${directID}`);assert.equal(direct.status,"POSTED");assert.ok(direct.purchaseId&&direct.saleId);
  const p=await call(`/v1/purchases/${direct.purchaseId}`),s=await call(`/v1/sales/${direct.saleId}`);assert.equal(p.sourceOrderId,directID);assert.equal(s.sourceOrderId,directID);assert.equal(p.status,"POSTED");assert.equal(s.status,"POSTED");
  const note=await call(`/v1/sales/${direct.saleId}/delivery-note`);assert.equal(note.totalAmount,"30.00");assert.ok(!("purchaseTotal" in note));
  await page.goto(`${base}/business/purchases?purchaseId=${direct.purchaseId}`);await page.getByRole("button",{name:"查看来源直送单",exact:true}).waitFor();assert.equal(await page.getByRole("row").filter({hasText:p.documentNo}).getByRole("button",{name:"整单取消",exact:true}).count(),0);
  await page.goto(`${base}/business/direct-orders/${directID}`);await page.getByRole("button",{name:"整笔取消直送",exact:true}).click();await page.getByRole("textbox",{name:"直送取消原因",exact:true}).fill("实际保留数量变化，独立重录");await page.getByRole("dialog").getByRole("button",{name:"确认整笔取消",exact:true}).click();await page.getByRole("status",{name:"操作结果",exact:true}).filter({hasText:"直送已整笔取消"}).waitFor();
  assert.equal((await call(`/v1/direct-orders/${directID}`)).status,"CANCELLED");
  // Independent new direct delivery and response-loss recovery.
  const body={supplierId:partner.id,customerId:partner.id,businessDate:"2026-10-01",items:[{productId:goods.id,productType:"GOODS",unit:"条",quantity:"1",purchasePrice:"10",salePrice:"15"}]};
  const second=await call("/v1/direct-orders",body);assert.notEqual(second.id,directID);await page.goto(`${base}/business/direct-orders/${second.id}`);
  let finishDrop; const dropDone=new Promise(resolve=>{finishDrop=resolve;});
  await page.route(`**/api/v1/direct-orders/${second.id}/post`,async route=>{await route.fetch();await route.abort("failed");finishDrop();});
  await page.getByRole("button",{name:"确认直送",exact:true}).click();await page.getByRole("dialog").getByRole("button",{name:"确认并同时过账",exact:true}).click();await dropDone;await page.getByText("直送操作结果待核实，保留原版本，不另建单据。",{exact:true}).waitFor();await page.unroute(`**/api/v1/direct-orders/${second.id}/post`);
  await page.reload();await page.getByRole("button",{name:"核实直送操作结果",exact:true}).click();await page.getByRole("status",{name:"操作结果",exact:true}).filter({hasText:"已核实直送操作结果"}).waitFor();
  const secondPosted=await call(`/v1/direct-orders/${second.id}`);const secondSale=await call(`/v1/sales/${secondPosted.saleId}`);
  const returned=await call("/v1/sale-returns",{saleId:secondSale.id,businessDate:"2026-10-02",items:[{saleItemId:secondSale.items[0].id,quantity:"1"}]});await call(`/v1/sale-returns/${returned.id}/post`,{version:returned.version});
  await page.reload();await page.getByText(/存在生效退货，不能整笔取消/).waitFor();assert.equal(await page.getByRole("button",{name:"整笔取消直送",exact:true}).isDisabled(),true);
  // Legacy purchase remains eligible for its original sales path.
  const legacy=await call("/v1/purchases",{directDelivery:true,partnerId:partner.id,businessDate:"2026-10-01",items:[{productId:goods.id,productType:"GOODS",unit:"条",quantity:"1",unitPrice:"10"}]});
  await page.goto(`${base}/business/purchases?purchaseId=${legacy.id}`);await page.getByRole("button",{name:"关联新建直送销售",exact:true}).click();await select("客户",partner.code);await page.getByRole("group",{name:"关联直送采购",exact:true}).waitFor();
  // Ordinary sales record multiple procurement lines; later dates require explicit confirmation.
  async function procurement(date){const d=await call("/v1/purchases",{partnerId:partner.id,businessDate:date,items:[{productId:goods.id,productType:"GOODS",unit:"条",quantity:"10",unitPrice:"10"}]});return call(`/v1/purchases/${d.id}/post`,{version:d.version});}
  const source1=await procurement("2026-10-01"),source2=await procurement("2026-10-05");
  page.on("dialog",d=>d.accept());await page.goto(`${base}/business/sales/new`);await select("客户",partner.code);await select("第 1 行商品",goods.code);await page.locator("#sale-date").fill("2026-10-02");await page.locator("#quantity-0").fill("4");assert.equal(await page.getByRole("group",{name:"关联直送采购",exact:true}).count(),0);
  await page.getByRole("button",{name:"添加采购来源",exact:true}).click();await sourcePick("第 1 行采购来源 1",source1.documentNo);await page.getByRole("textbox",{name:"第 1 行采购来源 1数量",exact:true}).fill("1");
  await page.getByRole("button",{name:"添加采购来源",exact:true}).click();await sourcePick("第 1 行采购来源 2",source2.documentNo);await page.getByRole("textbox",{name:"第 1 行采购来源 2数量",exact:true}).fill("2");assert.equal(await page.getByRole("button",{name:"保存草稿",exact:true}).isDisabled(),true);await page.getByRole("checkbox",{name:"采购日期晚于销售日期，已核对并确认关联"}).check();
  await page.getByRole("button",{name:"保存草稿",exact:true}).click();await page.waitForURL(/sales\/\d+\/edit(?:\?|$)/);const saleID=Number(new URL(page.url()).pathname.split("/").at(-2));const savedSale=await call(`/v1/sales/${saleID}`);assert.equal(savedSale.items[0].purchaseSources.length,2);
  await call(`/v1/sales/${saleID}/post`,{version:savedSale.version});await page.reload();await page.getByRole("button",{name:"补填或更正采购来源",exact:true}).click();const dialog=page.getByRole("dialog",{name:"补填或更正采购来源",exact:true});await dialog.getByRole("textbox",{name:`销售明细 ${savedSale.items[0].id}来源 1数量`,exact:true}).fill("0.5");await dialog.getByRole("button",{name:"保存采购来源",exact:true}).click();await page.getByText("采购来源已保存，库存和往来未变化。",{exact:true}).waitFor();assert.equal((await call(`/v1/sales/${saleID}`)).items[0].purchaseSources[0].quantity,"0.5");
  await page.getByRole("button",{name:"查看来源修改历史",exact:true}).click();await page.getByText(/修改前：采购明细/).first().waitFor();
  await call(`/v1/purchases/${source1.id}/cancel`,{version:source1.version,reason:"来源单录错"});await page.reload();await page.getByText(/来源采购已取消，请核对/).first().waitFor();
  assert.deepEqual(errors,[]);console.log(JSON.stringify({result:"passed",checks:["direct-confirm","managed-child-guards","delivery-note","cancel-and-independent-create","lost-response-refresh-recovery","return-blocks-cancel","legacy-source-path","multi-purchase-sources","later-date-confirmation","posted-source-edit-history","cancelled-source-preserved"]}));
}catch(e){if(page)console.error((await page.locator("body").innerText()).slice(0,5000));throw e;}finally{if(browser)await browser.close();vite.kill("SIGTERM");}
