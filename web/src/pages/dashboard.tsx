import { Boxes, Package, ShoppingCart, Truck, Warehouse } from "lucide-react";
import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";

const productAreas = [
  {
    title: "基础资料",
    description: "商品、客户 / 供应商与仓库等核心主数据。",
    icon: Package,
  },
  {
    title: "采购入库",
    description: "记录采购单据，并在过账后形成库存入库流水。",
    icon: Truck,
  },
  {
    title: "销售出库",
    description: "记录销售单据，校验库存并形成库存出库流水。",
    icon: ShoppingCart,
  },
  {
    title: "库存管理",
    description: "查看当前库存、库存流水，并处理库存调整。",
    icon: Warehouse,
  },
];

export function DashboardPage() {
  return (
    <>
      <PageHeader
        title="工作台"
        description="简单进销存 · 采购、销售与库存管理"
      />

      <ContentCard bodyClassName="p-6">
        <div className="flex items-start gap-4">
          <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-blue-50 text-primary">
            <Boxes className="h-6 w-6" aria-hidden />
          </span>
          <div>
            <h2 className="text-xl font-semibold text-text-primary">
              欢迎使用简单进销存
            </h2>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
              当前基础平台已完成产品化，认证、权限、菜单、用户、日志和通知等管理能力已就绪。
              进销存业务模块将围绕下列核心流程逐步接入。
            </p>
          </div>
        </div>
      </ContentCard>

      <div className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {productAreas.map((item) => {
          const Icon = item.icon;
          return (
            <ContentCard key={item.title} bodyClassName="p-5">
              <Icon className="h-5 w-5 text-primary" aria-hidden />
              <h3 className="mt-4 font-semibold text-text-primary">{item.title}</h3>
              <p className="mt-2 text-sm leading-6 text-text-secondary">
                {item.description}
              </p>
            </ContentCard>
          );
        })}
      </div>
    </>
  );
}
