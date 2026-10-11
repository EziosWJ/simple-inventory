// Real public API fixtures and Chromium; use only an isolated database.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const api=process.env.PHASE5_API_URL??'http://127.0.0.1:18109',base='http://127.0.0.1:4197',prefix=`Q66-${Date.now()}`;
const vite=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4197','--strictPort'],{cwd:new URL('..',import.meta.url),env:{...process.env,VITE_API_BASE_URL:api},stdio:'ignore'});
let browser;
async function call(path,body,method=body===undefined?'GET':'POST'){const r=await fetch(`${api}/api${path}`,{method,headers:{Authorization:token??'','Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});const j=await r.json();assert.equal(r.status,200,`${path}: ${j.message}`);return j.data;}
async function waitRows(page,count){for(let n=0;n<100;n++){if(await page.getByRole('row').count()===count)return;await delay(50);}assert.equal(await page.getByRole('row').count(),count);}
async function waitValue(locator,value){for(let n=0;n<60;n++){if(await locator.inputValue()===value)return;await delay(50);}assert.equal(await locator.inputValue(),value);}
async function select(page,label,code){const g=page.getByRole('group',{name:label,exact:true});await g.getByRole('combobox').fill(code);await page.locator('button[role="option"]').filter({hasText:code}).last().click();}
let token;
try{
 for(let n=0;n<60;n++){try{if((await fetch(base)).ok)break;}catch{}await delay(250);}
 token=(await call('/auth/login',{username:'admin',password:process.env.PHASE5_UI_PASSWORD??'admin123'})).tokenValue;
 const customer=await call('/v1/partners',{code:`${prefix}-C`,name:'销售查询客户',type:'COMPANY',isCustomer:true});
 const other=await call('/v1/partners',{code:`${prefix}-O`,name:'销售查询客户乙',type:'COMPANY',isCustomer:true});
 const product=await call('/v1/products',{code:`${prefix}-A`,name:'甲商品A',type:'GOODS',unit:'台'});
 const productB=await call('/v1/products',{code:`${prefix}-B`,name:'乙商品B',type:'GOODS',unit:'支'});
 const service=await call('/v1/products',{code:`${prefix}-S`,name:'丙服务C',type:'SERVICE',unit:'次'});
 for(const stock of [{id:product.id,unit:'台'},{id:productB.id,unit:'支'}]){
  const opening=await call('/v1/inventory/adjustments',{items:[{productId:stock.id,productType:'GOODS',unit:stock.unit,quantity:'100',reason:'OPENING'}]});
  await call(`/v1/inventory/adjustments/${opening.id}/post`,{version:1});
 }
 const newSale=async(status,businessDate,partnerId,items)=>{
  const sale=await call('/v1/sales',{partnerId,businessDate,items});
  if(status==='POSTED')await call(`/v1/sales/${sale.id}/post`,{version:1});
  if(status==='CANCELLED')await call(`/v1/sales/${sale.id}/cancel`,{version:1,reason:'录错取消'});
  return sale;
 };
 const line=(product,type,unit,quantity,unitPrice)=>({productId:product.id,productType:type,unit,quantity,unitPrice});
 // A second, larger range keeps paging and sorting meaningful without touching the scenario below.
 const bulk=[];
 for(let n=0;n<12;n++)bulk.push(await newSale('POSTED','2026-07-15',customer.id,[line(productB,'GOODS','支','1',`${10+n%3}.00`)]));
 const first=await newSale('POSTED','2026-09-10',customer.id,[line(product,'GOODS','台','10','100.00')]);
 const mixed=await newSale('POSTED','2026-09-10',customer.id,[line(product,'GOODS','台','3','90.00'),line(productB,'GOODS','支','5','2.50'),line(service,'SERVICE','次','1','50.00')]);
 const draft=await newSale('DRAFT','2026-10-01',other.id,[line(product,'GOODS','台','2','100.00')]);
 const serviceOnly=await newSale('POSTED','2026-10-05',other.id,[line(service,'SERVICE','次','2','30.00')]);
 const source=await call(`/v1/sale-returns/source/${first.id}`),postedReturn=await call('/v1/sale-returns',{saleId:first.id,businessDate:'2026-10-20',items:[{saleItemId:source.items[0].saleItemId,quantity:'2'}]});
 await call(`/v1/sale-returns/${postedReturn.id}/post`,{version:1});
 await call(`/v1/products/${product.id}`,{name:'新甲商品Z',type:'GOODS',unit:'台'},'PUT');
 browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH}:{})});
 const context=await browser.newContext({viewport:{width:1500,height:1050}});await context.addInitScript(t=>localStorage.setItem('web-auth',JSON.stringify({token:t,user:null})),token);
 const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));page.on('dialog',d=>d.accept());
 const doc=no=>page.getByRole('row').filter({hasText:no});
 const summary=page.getByRole('region',{name:'销售明细全结果合计'});
 await page.goto(`${base}/business/sales`);
 // 1. Fields use the whole panel width and the query buttons sit below them, right aligned.
 await page.getByRole('heading',{name:'销售出库',exact:true}).waitFor();
 const numberInput=page.getByLabel('销售单号');
 await numberInput.waitFor();
 const queryButton=page.getByRole('button',{name:'查询',exact:true}),resetButton=page.getByRole('button',{name:'重置',exact:true});
 const fieldsBox=await page.evaluate(()=>{const r=document.querySelector('input[aria-label="销售单号"]').closest('div.grid').getBoundingClientRect();return{left:r.left,right:r.right,width:r.width};});
 const inputBox=await numberInput.boundingBox(),queryBox=await queryButton.boundingBox(),resetBox=await resetButton.boundingBox();
 assert.ok(queryBox.y>=inputBox.y+inputBox.height,`查询 must sit below the fields: ${JSON.stringify({queryBox,inputBox})}`);
 assert.ok(fieldsBox.width>900,`fields must use the panel width: ${fieldsBox.width}`);
 assert.ok(Math.abs(resetBox.x+resetBox.width-fieldsBox.right)<=6,`重置 must align with the right edge of the fields: ${JSON.stringify({resetBox,fieldsBox})}`);
 assert.equal(await page.getByRole('button',{name:'刷新',exact:true}).count(),1);
 // 2. Shortcuts, then a reset back to the full default range.
 const today=new Date(),month=new Intl.DateTimeFormat('en-CA',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit'}).format(today);
 const lastDay=new Date(Date.UTC(Number(month.slice(0,4)),Number(month.slice(5,7)),0)).getUTCDate();
 await page.getByRole('button',{name:'本月',exact:true}).click();
 await waitValue(page.getByLabel('业务日期起'),`${month}-01`);
 await waitValue(page.getByLabel('业务日期止'),`${month}-${String(lastDay).padStart(2,'0')}`);
 assert.equal(new URL(page.url()).searchParams.get('businessFrom'),`${month}-01`);
 await resetButton.click();
 assert.equal(new URL(page.url()).searchParams.get('businessFrom'),null);
 await waitValue(page.getByLabel('业务日期起'),'');
 await page.getByRole('button',{name:'已过账',exact:true}).click();
 await doc(first.documentNo).waitFor();
 await waitValue(page.getByLabel('销售状态'),'POSTED');
 assert.equal(await doc(draft.documentNo).count(),0);
 for(const row of await page.getByRole('row').all())if(await row.locator('td').count())assert.ok((await row.innerText()).includes('已过账'));
 await resetButton.click();
 await doc(draft.documentNo).waitFor();
 // 3. Document view: content summary, whole-document expansion, matched line highlight.
 const mixedRow=doc(mixed.documentNo);
 assert.ok((await mixedRow.innerText()).includes('甲商品A ×3台'),'the row must summarise its content');
 assert.ok((await mixedRow.innerText()).includes('乙商品B ×5支'));
 await mixedRow.getByRole('button',{name:'展开明细',exact:true}).click();
 const expanded=page.getByRole('list',{name:`${mixed.documentNo} 全部明细`});
 await expanded.waitFor();
 assert.equal(await expanded.getByRole('listitem').count(),3);
 assert.ok((await expanded.innerText()).includes('乙商品B'));
 assert.ok((await expanded.innerText()).includes('丙服务C'));
 await mixedRow.getByRole('button',{name:'收起明细',exact:true}).click();
 await expanded.waitFor({state:'detached'});
 await select(page,'商品或服务筛选',product.code);
 await page.waitForURL(url=>url.searchParams.get('productId')===String(product.id));
 await doc(mixed.documentNo).waitFor();
 await waitRows(page,4);
 assert.equal(await doc(mixed.documentNo).locator('[data-matched="true"]').count(),1);
 await doc(mixed.documentNo).getByRole('button',{name:'展开明细',exact:true}).click();
 const matchedLines=page.getByRole('list',{name:`${mixed.documentNo} 全部明细`}).locator('[data-matched="true"]');
 await matchedLines.waitFor();
 assert.equal(await matchedLines.count(),1);
 assert.ok((await matchedLines.innerText()).includes('甲商品A'));
 assert.equal(await page.getByRole('list',{name:`${mixed.documentNo} 全部明细`}).getByRole('listitem').count(),3);
 // 4. Item view keeps the filter and summarises every matching line, not one page.
 await page.getByRole('group',{name:'销售浏览视图'}).getByRole('button',{name:'按明细',exact:true}).click();
 await page.getByRole('columnheader',{name:/净销售额/}).waitFor();
 const afterSwitch=new URL(page.url()).searchParams;
 assert.equal(afterSwitch.get('view'),'item');
 assert.equal(afterSwitch.get('productId'),String(product.id));
 assert.equal(afterSwitch.get('page'),null);
 const itemRows=page.getByRole('row');
 await waitRows(page,4);
 for(const row of await itemRows.all())if(await row.locator('td').count())assert.ok((await row.innerText()).includes('甲商品A'));
 assert.ok((await summary.innerText()).includes('¥1270.00'),'posted original amount of the filtered lines');
 assert.ok((await summary.innerText()).includes('-¥200.00'),'effective returns belong to their line');
 assert.ok((await summary.innerText()).includes('¥1070.00'),'net sales excludes the returned amount');
 assert.ok((await summary.innerText()).includes('¥200.00'),'draft keeps its original amount');
 const groups=summary.getByRole('list',{name:'数量合计明细'});
 assert.equal(await groups.getByRole('listitem').count(),2);
 assert.ok((await groups.getByRole('listitem').filter({hasText:'已过账'}).innerText()).includes('原 13 台 · 退 2 · 净 11'));
 assert.ok((await groups.getByRole('listitem').filter({hasText:'草稿'}).innerText()).includes('原 2 台'));
 // A service line cannot be returned, so its net stays its original quantity.
 await resetButton.click();
 await waitRows(page,11);
 await select(page,'商品或服务筛选',service.code);
 await page.waitForURL(url=>url.searchParams.get('productId')===String(service.id));
 await waitRows(page,3);
 assert.equal(await itemRows.filter({hasText:'丙服务C'}).count(),2);
 for(const row of await itemRows.all())if(await row.locator('td').count())assert.ok((await row.innerText()).includes('不适用'));
 assert.ok((await summary.innerText()).includes('¥110.00'));
 assert.ok((await summary.innerText()).includes('净销售额'));
 assert.equal(await groups.getByRole('listitem').count(),1);
 assert.ok((await groups.getByRole('listitem').innerText()).includes('原 3 次 · 净 3'));
 assert.ok(!(await groups.getByRole('listitem').innerText()).includes('退'));
 // 5. Paging, sorting and page size over one whole result.
 await resetButton.click();
 await waitRows(page,11);
 await select(page,'商品或服务筛选',productB.code);
 await page.waitForURL(url=>url.searchParams.get('productId')===String(productB.id));
 await page.getByRole('row').filter({hasText:bulk[11].documentNo}).waitFor();
 await waitRows(page,11);
 await page.getByLabel('业务日期起').fill('2026-07-01');await page.getByLabel('业务日期止').fill('2026-07-31');
 assert.equal(await page.getByLabel('业务日期起').inputValue(),'2026-07-01');
 await queryButton.click();
 await page.waitForURL(url=>url.searchParams.get('businessFrom')==='2026-07-01');
 await page.getByRole('row').filter({hasText:bulk[11].documentNo}).waitFor();
 await waitRows(page,11);
 const firstPageSummary=await summary.innerText();
 assert.ok(firstPageSummary.includes('¥132.00'));
 await page.getByRole('button',{name:'下一页'}).click();
 await page.getByRole('row').filter({hasText:bulk[0].documentNo}).waitFor();
 await waitRows(page,3);
 assert.equal(await summary.innerText(),firstPageSummary,'the summary must not depend on the page');
 const amountsOnPage=()=>page.evaluate(()=>Array.from(document.querySelectorAll('tbody tr')).map(row=>{const cells=row.querySelectorAll('td');return cells.length?Number(cells[5].innerText.replace('¥','')):null;}).filter(value=>value!==null));
 const sortedAmounts=async(ascending)=>{for(let n=0;n<100;n++){const amounts=await amountsOnPage();if(amounts.length===10&&amounts.every((value,index)=>index===0||(ascending?value>=amounts[index-1]:value<=amounts[index-1])))return amounts;await delay(50);}return amountsOnPage();};
 await page.getByRole('columnheader',{name:/原金额/}).click();
 await page.waitForURL(url=>url.searchParams.get('sort')==='amount'&&url.searchParams.get('dir')==='asc');
 assert.equal(new URL(page.url()).searchParams.get('ipage'),null,'sorting returns to the first page');
 assert.deepEqual(await sortedAmounts(true),[10,10,10,10,11,11,11,11,12,12],'ascending original amount');
 await page.getByRole('columnheader',{name:/原金额/}).click();
 await page.waitForURL(url=>url.searchParams.get('dir')==='desc');
 assert.deepEqual(await sortedAmounts(false),[12,12,12,12,11,11,11,11,10,10],'descending original amount');
 await page.getByLabel('每页条数').selectOption('20');
 await page.getByRole('row').filter({hasText:bulk[0].documentNo}).waitFor();
 await waitRows(page,13);
 assert.equal(await summary.innerText(),firstPageSummary,'the page size must not change the summary');
 await page.getByLabel('每页条数').selectOption('10');
 await page.getByRole('row').filter({hasText:bulk[11].documentNo}).waitFor();
 // 6. A slow page-two response cannot replace a newer query.
 let delayed=false;
 await page.route('**/api/v1/sales/items?*',async route=>{const u=new URL(route.request().url());if(u.searchParams.get('page')==='2'){const response=await route.fetch();delayed=true;await delay(1800);try{await route.fulfill({response});}catch{}}else await route.continue();});
 await page.getByRole('button',{name:'下一页'}).click();
 for(let n=0;n<50&&!delayed;n++)await delay(50);
 assert.ok(delayed);
 await page.getByLabel('销售状态').selectOption('CANCELLED');await queryButton.click();
 await page.getByText('没有匹配的销售明细').waitFor();await delay(2000);
 assert.equal(await page.getByText('没有匹配的销售明细').count(),1);
 await waitRows(page,0);
 assert.ok((await summary.innerText()).includes('¥0.00'),'an empty result reports zero totals');
 assert.equal(await groups.count(),0);
 await page.unroute('**/api/v1/sales/items?*');
 // 7. Opening the source document and coming back keeps view, filter, sort and page.
 await page.getByLabel('销售状态').selectOption('');await queryButton.click();
 await page.getByRole('row').filter({hasText:bulk[11].documentNo}).waitFor();
 await page.getByRole('columnheader',{name:/原金额/}).click();
 await page.waitForURL(url=>url.searchParams.get('dir')==='asc');
 await page.getByRole('button',{name:'下一页'}).click();
 await waitRows(page,3);
 const beforeDetail=new URL(page.url()).search;
 const pagedDoc=await page.evaluate(()=>document.querySelectorAll('tbody tr')[0].querySelectorAll('td')[8].innerText.trim());
 const pagedRow=page.getByRole('row').filter({hasText:pagedDoc});
 await pagedRow.getByRole('button',{name:pagedDoc,exact:true}).click();
 const dialog=page.getByRole('dialog');
 await dialog.getByText(pagedDoc,{exact:false}).first().waitFor();
 assert.ok((await dialog.innerText()).includes('乙商品B'));
 await dialog.getByRole('button',{name:'关闭详情弹窗',exact:true}).click();
 await page.getByRole('dialog').waitFor({state:'detached'});
 assert.equal(new URL(page.url()).search,beforeDetail);
 assert.equal(new URL(page.url()).searchParams.get('view'),'item');
 assert.equal(new URL(page.url()).searchParams.get('ipage'),'2');
 await waitRows(page,3);
 assert.equal(await summary.innerText(),firstPageSummary);
 assert.deepEqual(errors,[]);
 console.log('PASS real browser: full-width filter fields with right aligned actions, month/posted shortcuts, document summaries and whole-document expansion with matched-line highlight, cross-document item view with status grouped amounts and product/unit quantities, sorting, paging, page-size and view/sort/page context after the source document, empty totals and late response ownership');
}finally{if(browser)await browser.close();vite.kill();}
