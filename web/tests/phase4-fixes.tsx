import { createRoot } from "react-dom/client";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { PurchaseReturnsPage } from "@/pages/business/purchase-returns";
import { SaleReturnsPage } from "@/pages/business/sale-returns";
import { PurchasesPage } from "@/pages/business/purchases";
import { PurchaseFormPage } from "@/pages/business/purchase-form";
import { useAuthStore } from "@/store/auth-store";
import { SaleFormPage } from "@/pages/business/sale-form";
import { SalesPage } from "@/pages/business/sales";
import "@/styles/globals.css";
const kind = new URLSearchParams(location.search).get("kind") ?? "purchase-returns";
const pages = { "purchase-returns": PurchaseReturnsPage, "sale-returns": SaleReturnsPage, purchases: PurchasesPage, sales: SalesPage };
const Page = pages[kind as keyof typeof pages];
useAuthStore.setState({ user: { id: 1, username: "fixture", nickname: "fixture", roles: [] } });
const router = createMemoryRouter([
  { path: "/", element: <Page /> },
  { path: "/business/purchases/:id/edit", element: <PurchaseFormPage /> },
  { path: "/business/sales/:id/edit", element: <SaleFormPage /> },
  { path: "/business/sales", element: <SalesPage /> },
  { path: "/business/sales/new", element: <SaleFormPage /> },
]);
createRoot(document.getElementById("root")!).render(<RouterProvider router={router} />);
