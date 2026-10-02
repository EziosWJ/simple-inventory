import { FileSearch, ShoppingCart, Truck, Wallet } from "lucide-react";
import { Link } from "react-router-dom";
import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";
import { useAuthStore } from "@/store/auth-store";
import type { CurrentUserMenu } from "@/types";

function menuPaths(menus:CurrentUserMenu[]):Set<string>{
 const paths=new Set<string>();
 for(const menu of menus){if(menu.visible===1){if(menu.menuType==="MENU")paths.add(menu.path);for(const path of menuPaths(menu.children??[]))paths.add(path)}}
 return paths;
}
export function DashboardPage(){
 const menus=useAuthStore(s=>s.menus),loading=useAuthStore(s=>s.isLoadingMenus);
 const paths=menuPaths(menus);
 const areas=[
  {title:"开采购单",icon:Truck,description:"搜索供应商和实物商品，保存草稿或确认过账。",links:[{menu:"/business/purchases",path:"/business/purchases/new",label:"新建采购单"}]},
  {title:"开销售单",icon:ShoppingCart,description:"录入客户、实物或服务和送货资料。",links:[{menu:"/business/sales",path:"/business/sales/new",label:"新建销售单"}]},
  {title:"查欠款",icon:Wallet,description:"客户应收与供应商应付分别查账，追溯往来来源。",links:[{menu:"/business/partner-balances",path:"/business/partner-balances",label:"查看往来余额"}]},
  {title:"查单据",icon:FileSearch,description:"按对象、商品、状态和业务日期查询原单。",links:[{menu:"/business/purchases",path:"/business/purchases",label:"查询采购单"},{menu:"/business/sales",path:"/business/sales",label:"查询销售单"}]},
 ];
 return <div className="space-y-space-4">
  <PageHeader title="工作台" description="简单进销存 · 两位经营者共享店铺业务，分别记录实际操作人。"/>
  {loading&&<p role="status" className="text-sm text-text-secondary">正在读取业务入口…</p>}
  <div className="grid gap-space-4 md:grid-cols-2 xl:grid-cols-4">
   {areas.map(area=><ContentCard key={area.title} bodyClassName="space-y-space-3">
    <area.icon className="h-5 w-5 text-primary" aria-hidden/>
    <h2 className="text-card-title font-semibold">{area.title}</h2>
    <p className="text-sm leading-6 text-text-secondary">{area.description}</p>
    <div className="flex flex-wrap gap-space-3">{area.links.filter(link=>paths.has(link.menu)).map(link=><Link className="text-sm font-medium text-primary underline" key={link.path} to={link.path}>{link.label}</Link>)}</div>
    {!loading&&!area.links.some(link=>paths.has(link.menu))&&<p className="text-sm text-text-tertiary">暂无该业务菜单，请联系管理员检查账号菜单配置。</p>}
   </ContentCard>)}
  </div>
 </div>;
}
