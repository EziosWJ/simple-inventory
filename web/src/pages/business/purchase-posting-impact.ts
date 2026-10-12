import type { PurchaseDraft, PurchaseLine } from "@/api/business";

function milli(value: string): bigint {
  const [whole, fraction = ""] = value.split(".");
  return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, "0"));
}

function quantity(value: bigint): string {
  const fraction = (value % 1000n).toString().padStart(3, "0").replace(/0+$/, "");
  return `${value / 1000n}${fraction ? `.${fraction}` : ""}`;
}

export function purchaseInventoryImpact(items: Pick<PurchaseLine, "productId" | "productCode" | "productName" | "productModel" | "productSpecification" | "unit" | "quantity">[], directDelivery: boolean): string[] {
  const groups = new Map<string, { line: (typeof items)[number]; total: bigint }>();
  for (const line of items) {
    if (!/^\d+(?:\.\d{0,3})?$/.test(line.quantity)) continue;
    const key = `${line.productId}:${line.unit}`;
    const group = groups.get(key) ?? { line, total: 0n };
    group.total += milli(line.quantity);
    groups.set(key, group);
  }
  return [...(directDelivery ? ["旧版直送采购：先增加账面库存，关联销售过账后再扣减。"] : []), ...[...groups.values()].map(({ line, total }) => `${[line.productCode, line.productName, line.productModel, line.productSpecification].filter(Boolean).join(" · ")}：增加 ${quantity(total)} ${line.unit}`)];
}

export function purchasePostingConfirmation(document: PurchaseDraft): string[] {
  return [
    /^0(?:\.0+)?$/.test(document.totalAmount) ? "零金额，供应商往来余额无变化" : `供应商应付增加 ¥${document.totalAmount}`,
    ...purchaseInventoryImpact(document.items, document.directDelivery),
  ];
}
