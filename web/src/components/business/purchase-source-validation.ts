import type { PurchaseSourceInput } from "@/api/business";
export function sourceQuantityMilli(value: string): bigint | null {
  if (!/^\d+(\.\d{1,3})?$/.test(value)) return null;
  const [whole, frac = ""] = value.split(".");
  return BigInt(whole) * 1000n + BigInt(frac.padEnd(3, "0"));
}
export function purchaseSourcesInvalid(sources: PurchaseSourceInput[], quantity: string, businessDate: string): boolean {
  const cap = sourceQuantityMilli(quantity); if (cap === null) return true;
  let total = 0n; const seen = new Set<number>();
  for (const source of sources) {
    const qty = sourceQuantityMilli(source.quantity);
    if (!source.purchaseItemId || seen.has(source.purchaseItemId) || qty === null || qty <= 0n || (source.businessDate && source.businessDate > businessDate && !source.confirmLaterDate)) return true;
    seen.add(source.purchaseItemId); total += qty;
  }
  return total > cap;
}
