// Two real business roles/sessions, no mocked permissions or business API.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const api=process.env.PHASE5_API_URL??'http://127.0.0.1:18109',base='http://127.0.0.1:4196',prefix=`A5-${Date.now()}`;
const vite=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4196','--strictPort'],{cwd:new URL('..',import.meta.url),env:{...process.env,VITE_API_BASE_URL:api},stdio:'ignore'});
let browser,admin;
async function call(path,body,method=body===undefined?'GET':'POST',auth=admin){const r=await fetch(`${api}/api${path}`,{method,headers:{Authorization:auth??'','Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});const j=await r.json();assert.equal(r.status,200,`${path}: ${j.message}`);return j.data;}
async function select(page,label,code){const g=page.getByRole('group',{name:label,exact:true});await g.getByRole('button').first().click();await g.getByRole('textbox').fill(code);await g.locator('button[aria-pressed]').filter({hasText:code}).last().click();}
try{
 for(let n=0;n<60;n++){try{if((await fetch(base)).ok)break;}catch{}await delay(250);}
 admin=(await call('/auth/login',{username:'admin',password:process.env.PHASE5_UI_PASSWORD??'admin123'})).tokenValue;
 const flatten=nodes=>nodes.flatMap(n=>[n,...flatten(n.children??[])]);
 const businessMenus=flatten(await call('/system/menu/tree')).filter(m=>m.path?.startsWith('/business/')).map(m=>m.id);
 await call('/system/role',{roleName:'经营者验收角色',roleCode:prefix,status:1});
 const role=(await call('/system/role/options')).find(r=>r.roleCode===prefix);assert.ok(role);
 await call(`/system/role/${role.id}/menus`,{menuIds:businessMenus},'PUT');
 const users=[];
 for(let n=1;n<=2;n++){
  const username=`${prefix}-${n}`,nickname=`经营者${n}`;
  await call('/system/user',{username,nickname,deptId:1,status:1});const user=(await call(`/system/user/page?username=${username}`)).records.find(u=>u.username===username);assert.ok(user);
  await call(`/system/user/${user.id}/roles`,{roleIds:[role.id]},'PUT');users.push({...user,nickname,token:(await call('/auth/login',{username,password:'admin123'})).tokenValue});
 }
 const partner=await call('/v1/partners',{code:prefix,name:'双人共用往来',type:'COMPANY',isSupplier:true,isCustomer:true});
 const product=await call('/v1/products',{code:prefix,name:'双人共用商品',type:'GOODS',unit:'台',purchasePrice:'2.00',salePrice:'4.00'});
 browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH}:{})});
 const pages=[];
 for(const user of users){
  const context=await browser.newContext({viewport:{width:1500,height:1000}}),page=await context.newPage();page.on('dialog',d=>d.accept());
  await page.goto(`${base}/business/purchases/new`);await page.waitForURL('**/login');
  await page.getByLabel('用户名',{exact:true}).fill(user.username);await page.getByLabel('密码',{exact:true}).fill('admin123');await page.locator('button[type=submit]').click();
  await page.waitForURL('**/business/purchases/new');await page.goto(`${base}/dashboard`);
  for(const title of ['开采购单','开销售单','查欠款','查单据'])await page.getByRole('heading',{name:title,exact:true}).waitFor();
  await page.getByRole('link',{name:'新建采购单',exact:true}).waitFor();assert.equal(await page.getByRole('link',{name:'用户管理',exact:true}).count(),0);
  const denied=await fetch(`${api}/api/system/notification`,{method:'POST',headers:{Authorization:user.token,'Content-Type':'application/json'},body:JSON.stringify({title:'无权发布',content:'验收通知',recipientIds:[user.id]})});assert.equal(denied.status,403);
  pages.push(page);
 }
 const [first,second]=pages;
 await first.getByRole('link',{name:'新建采购单',exact:true}).click();await select(first,'供应商',partner.code);await select(first,'第 1 行商品',product.code);
 await first.getByRole('button',{name:'保存草稿',exact:true}).click();await first.waitForURL(/\/purchases\/\d+\/edit(?:\?.*)?$/);
 const id=Number(first.url().match(/purchases\/(\d+)\/edit/)[1]);
 await second.getByRole('link',{name:'查询采购单',exact:true}).click();await second.getByRole('row').filter({hasText:(await call(`/v1/purchases/${id}`)).documentNo}).getByRole('button',{name:'编辑',exact:true}).click();
 await first.getByLabel('数量').fill('3');await first.getByRole('button',{name:'保存草稿',exact:true}).click();await first.getByRole('status').filter({hasText:/原保存版本 2/}).waitFor();
 await second.getByLabel('数量').fill('9');await second.getByRole('button',{name:'保存草稿',exact:true}).click();await second.getByRole('button',{name:'重新加载最新单据'}).waitFor();assert.equal(await second.getByLabel('数量').inputValue(),'9');
 await second.getByRole('button',{name:'重新加载最新单据'}).click();await second.getByLabel('数量').waitFor();await second.getByRole('button',{name:'保存并过账',exact:true}).click();await second.getByRole('dialog').getByRole('button',{name:'确认保存并过账'}).click();await second.getByRole("status",{name:"操作结果"}).filter({hasText:"采购单过账成功"}).waitFor();
 const posted=await call(`/v1/purchases/${id}`);assert.equal(posted.createdBy,users[0].id);assert.equal(posted.postedBy,users[1].id);assert.equal(posted.postedByName,users[1].nickname);
 await first.reload();await first.getByText(/单据已生效或取消/).waitFor();
 await second.goto(`${base}/dashboard`);await second.getByRole('link',{name:'新建销售单',exact:true}).click();await select(second,'客户',partner.code);await select(second,'第 1 行商品',product.code);
 await second.getByRole('button',{name:'保存并过账',exact:true}).click();await second.getByRole('dialog').getByRole('button',{name:'确认保存并过账'}).click();await second.getByRole("status",{name:"操作结果"}).filter({hasText:"销售单过账成功"}).waitFor();
 await first.goto(`${base}/dashboard`);await first.getByRole('link',{name:'查看往来余额',exact:true}).click();await select(first,'筛选往来单位',partner.code);await first.getByRole('row').filter({hasText:'客户欠店铺'}).waitFor();await first.getByRole('row').filter({hasText:'店铺欠供应商'}).waitFor();
 await first.getByRole('row').filter({hasText:'客户欠店铺'}).getByRole('button',{name:'查看明细'}).click();await first.getByRole('button',{name:'打开来源',exact:true}).first().click();await first.getByRole('dialog').getByText(/创建人：经营者2/).waitFor();
 await second.goto(`${base}/dashboard`);await second.getByRole('button',{name:'菜单搜索',exact:true}).first().click();await second.getByRole('combobox',{name:'搜索菜单'}).fill('销售');await second.getByRole('listbox',{name:'菜单搜索结果'}).getByText('销售出库',{exact:true}).click();await second.waitForURL('**/business/sales');
 await second.reload();await second.getByRole('heading',{name:'销售出库',exact:true}).waitFor();
 console.log('PASS real browser: two actual business roles/login sessions, home actions/menu search, shared drafts/balances/sources, version conflict retaining input, real creators/posters and denied notification administration');
}finally{if(browser)await browser.close();vite.kill();}
