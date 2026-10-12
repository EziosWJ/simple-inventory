import { useLayoutEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";

export type DocumentFilters = {
  documentNo: string;
  partnerId: string;
  productId: string;
  partnerKeyword: string;
  productKeyword: string;
  status: string;
  businessFrom: string;
  businessTo: string;
};
export const blankDocumentFilters: DocumentFilters = {
  documentNo: "", partnerId: "", productId: "", partnerKeyword: "",
  productKeyword: "", status: "", businessFrom: "", businessTo: "",
};

export function useDocumentSearch() {
  const [params, setParams] = useSearchParams();
  // Only filter changes replace the editable inputs or invalidate a query.
  const filterKey = JSON.stringify(Object.keys(blankDocumentFilters).map(key => params.get(key) ?? ""));
  const query = useMemo(() => {
    const values = JSON.parse(filterKey) as string[];
    return Object.fromEntries(Object.keys(blankDocumentFilters).map((key, index) => [key, values[index]])) as DocumentFilters;
  }, [filterKey]);
  const [filters, setFilters] = useState(query);
  useLayoutEffect(() => setFilters(query), [query]);
  const rawPage = Number(params.get("page"));
  const page = Number.isSafeInteger(rawPage) && rawPage > 0 ? rawPage : 1;

  function setPage(next: number) {
    setParams(current => {
      const updated = new URLSearchParams(current);
      updated.set("page", String(next));
      return updated;
    });
  }

  function apply(next: DocumentFilters, resetExtra: string[] = []) {
    const normalized = Object.fromEntries(Object.entries(next).map(([key, value]) => [key, value.trim()])) as DocumentFilters;
    if (normalized.partnerKeyword) normalized.partnerId = "";
    if (normalized.productKeyword) normalized.productId = "";
    setFilters(normalized);
    setParams(current => {
      const updated = new URLSearchParams(current);
      for (const [key, value] of Object.entries(normalized)) {
        if (value) updated.set(key, value);
        else updated.delete(key);
      }
      updated.delete("page");
      for (const key of resetExtra) updated.delete(key);
      return updated;
    });
  }
  return { params, setParams, query, filters, setFilters, page, setPage, apply };
}
