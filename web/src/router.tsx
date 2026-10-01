import { createBrowserRouter, Navigate } from "react-router-dom";
import { RequireAuth } from "@/components/auth/require-auth";
import { AppShell } from "@/components/layout/app-shell";
import { AccountProfilePage } from "@/pages/account-profile";
import { ChangePasswordPage } from "@/pages/change-password";
import { DashboardPage } from "@/pages/dashboard";
import { DetailDemoPage } from "@/pages/examples/detail-demo";
import { FileUploadDemoPage } from "@/pages/examples/file-upload-demo";
import { FormExamplePage } from "@/pages/form-example";
import { ListDemoPage } from "@/pages/examples/list-demo";
import { TreeDemoPage } from "@/pages/examples/tree-demo";
import { TreeTableDemoPage } from "@/pages/examples/tree-table-demo";
import { LoginPage } from "@/pages/login";
import { NotFoundPage } from "@/pages/not-found";
import { SettingsPage } from "@/pages/settings";
import { SystemConfigsPage } from "@/pages/system/configs";
import { SystemDeptsPage } from "@/pages/system/depts";
import { SystemDictsPage } from "@/pages/system/dicts";
import { SystemFilesPage } from "@/pages/system/files";
import { SystemLoginLogsPage } from "@/pages/system/logs/login-logs";
import { SystemOperLogsPage } from "@/pages/system/logs/oper-logs";
import { SystemMenusPage } from "@/pages/system/menus";
import { SystemRolesPage } from "@/pages/system/roles";
import { UsersPage } from "@/pages/system/users";
import { NotificationsPage } from "@/pages/notifications";
import { NotificationManagePage } from "@/pages/system/notifications";
import { ProductsPage } from "@/pages/business/products";
import { PartnersPage } from "@/pages/business/partners";
import { WarehousePage } from "@/pages/business/warehouse";
import { InventoryAdjustmentsPage } from "@/pages/business/inventory-adjustments";
import { InventoryBalancesPage } from "@/pages/business/inventory-balances";
import { InventoryEntriesPage } from "@/pages/business/inventory-entries";
import { PrintProfilePage } from "@/pages/business/print-profile";
import { PurchasesPage } from "@/pages/business/purchases";
import { PurchaseReturnsPage } from "@/pages/business/purchase-returns";
import { SaleReturnsPage } from "@/pages/business/sale-returns";
import { SalesPage } from "@/pages/business/sales";
import { DeliveryNotePage } from "@/pages/business/delivery-note";
import { PartnerBalancesPage } from "@/pages/business/partner-balances";

export const router = createBrowserRouter([
  {
    path: "/login",
    element: <LoginPage />,
  },
  {
    path: "/",
    element: (
      <RequireAuth>
        <AppShell />
      </RequireAuth>
    ),
    children: [
      {
        index: true,
        element: <Navigate to="/dashboard" replace />,
      },
      {
        path: "dashboard",
        element: <DashboardPage />,
      },
      {
        path: "system/user",
        element: <UsersPage />,
      },
      { path: "business/products", element: <ProductsPage /> },
      { path: "business/partners", element: <PartnersPage /> },
      { path: "business/warehouse", element: <WarehousePage /> },
      { path: "business/inventory-adjustments", element: <InventoryAdjustmentsPage /> },
      { path: "business/inventory-balances", element: <InventoryBalancesPage /> },
      { path: "business/inventory-entries", element: <InventoryEntriesPage /> },
      { path: "business/print-profile", element: <PrintProfilePage /> },
      { path: "business/purchases", element: <PurchasesPage /> },
      { path: "business/purchase-returns", element: <PurchaseReturnsPage /> },
      { path: "business/sale-returns", element: <SaleReturnsPage /> },
      { path: "business/sales", element: <SalesPage /> },
      { path: "business/sales/:id/delivery-note", element: <DeliveryNotePage /> },
      { path: "business/partner-balances", element: <PartnerBalancesPage /> },
      { path: "business/refunds", element: <PartnerBalancesPage refundOnly /> },
      {
        path: "forms/basic",
        element: <FormExamplePage />,
      },
      {
        path: "examples/list",
        element: <ListDemoPage />,
      },
      {
        path: "examples/tree",
        element: <TreeDemoPage />,
      },
      {
        path: "examples/tree-table",
        element: <TreeTableDemoPage />,
      },
      {
        path: "examples/detail",
        element: <DetailDemoPage />,
      },
      {
        path: "examples/file-upload",
        element: <FileUploadDemoPage />,
      },
      {
        path: "settings",
        element: <SettingsPage />,
      },
      {
        path: "system",
        element: <Navigate to="/system/user" replace />,
      },
      {
        path: "system/dept",
        element: <SystemDeptsPage />,
      },
      {
        path: "system/dict",
        element: <SystemDictsPage />,
      },
      {
        path: "system/config",
        element: <SystemConfigsPage />,
      },
      {
        path: "system/role",
        element: <SystemRolesPage />,
      },
      {
        path: "system/menu",
        element: <SystemMenusPage />,
      },
      {
        path: "system/login-log",
        element: <SystemLoginLogsPage />,
      },
      {
        path: "system/oper-log",
        element: <SystemOperLogsPage />,
      },
      {
        path: "system/file",
        element: <SystemFilesPage />,
      },
      {
        path: "notifications",
        element: <NotificationsPage />,
      },
      {
        path: "notifications/:id",
        element: <NotificationsPage />,
      },
      {
        path: "system/notification",
        element: <NotificationManagePage />,
      },
      {
        path: "account/profile",
        element: <AccountProfilePage />,
      },
      {
        path: "account/change-password",
        element: <ChangePasswordPage />,
      },
      {
        path: "*",
        element: <NotFoundPage />,
      },
    ],
  },
]);
