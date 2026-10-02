import { createRoot } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { DeliveryNotePage } from "@/pages/business/delivery-note";
import { PartnerStatementPage } from "@/pages/business/partner-statement";
import "@/styles/globals.css";

const params = new URLSearchParams(location.search);
const statement = params.get("kind") === "statement";
const query = statement ? `?partnerId=1&direction=${params.get("direction") ?? "CUSTOMER"}&from=2026-10-01T00:00:00Z&to=2026-11-01T00:00:00Z` : `?showAmount=${params.get("showAmount") ?? "true"}`;
createRoot(document.getElementById("root")!).render(
  <MemoryRouter initialEntries={[`/1${query}`]}>
    <Routes><Route path="/:id" element={statement ? <PartnerStatementPage /> : <DeliveryNotePage />} /></Routes>
  </MemoryRouter>,
);
