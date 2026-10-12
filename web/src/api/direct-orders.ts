import { http } from "@/lib/http";
import type { BusinessPage } from "@/api/business";
export type DirectOrderLineInput = {
  productId: number; productType: "GOODS" | "SERVICE"; unit: string;
  quantity: string; purchasePrice: string; salePrice: string; remark?: string;
};
export type DirectOrderInput = {
  requestKey?: string; supplierId: number; customerId: number; businessDate: string;
  deliveryContact: string; deliveryPhone: string; deliveryAddress: string; remark: string;
  items: DirectOrderLineInput[];
};
export type DirectOrder = DirectOrderInput & {
  id: number; documentNo: string; status: "DRAFT" | "POSTED" | "CANCELLED"; version: number;
  supplierName: string; customerName: string; createdBy: number; createdByName: string; createTime: string;
  purchaseTotal: string; saleTotal: string;
  purchaseId: number | null; saleId: number | null; postedAt: string | null; postedByName: string; cancelledAt: string | null; cancelledByName: string; cancelReason: string | null; activeSaleReturns: number[]; activePurchaseReturns: number[];
  items: (DirectOrderLineInput & { id: number; productCode: string; productName: string; purchaseAmount: string; saleAmount: string })[];
};
export type DirectSaveResult = {
  state: "COMMITTED" | "UNCONFIRMED" | "NOT_COMMITTED";
  receipt?: { documentId: number; savedVersion: number }; document?: DirectOrder;
};
export const directOrderPage = (query: Record<string, string | number>) => http.get<BusinessPage<DirectOrder>>("/api/v1/direct-orders", { query });
export const getDirectOrder = (id: number) => http.get<DirectOrder>(`/api/v1/direct-orders/${id}`);
export const createDirectOrder = (body: DirectOrderInput) => http.post<DirectOrder>("/api/v1/direct-orders", body);
export const updateDirectOrder = (id: number, body: DirectOrderInput & { version: number }) => http.put<DirectOrder>(`/api/v1/direct-orders/${id}`, body);
export const directSaveResult = (operation: string, key: string, resolve = false) => {
  const url = `/api/v1/direct-orders/save-requests/${operation}/${encodeURIComponent(key)}`;
  return resolve ? http.post<DirectSaveResult>(`${url}/resolve`, {}) : http.get<DirectSaveResult>(url);
};

export const directStatus = { DRAFT: "草稿", POSTED: "已确认", CANCELLED: "已取消" };

export const postDirectOrder = (id: number, version: number) => http.post<DirectOrder>(`/api/v1/direct-orders/${id}/post`, { version });
export const cancelDirectOrder = (id: number, version: number, reason: string) => http.post<DirectOrder>(`/api/v1/direct-orders/${id}/cancel`, { version, reason });
