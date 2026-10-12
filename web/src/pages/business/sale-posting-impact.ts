import type { SaleDraft, SaleLine } from "@/api/business";

// Sales quantities have three decimal places. Keep the confirmation exact even
// when the same product appears on several lines with different prices.
function milli(value: string): bigint {
  const [whole, fraction = ""] = value.split(".");
  return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, "0"));
}
function quantity(value: bigint): string {
  const fraction = (value % 1000n).toString().padStart(3, "0").replace(/0+$/, "");
  return `${value / 1000n}${fraction ? `.${fraction}` : ""}`;
}
export function saleInventoryImpact(items: Pick<SaleLine, "productId" | "productCode" | "productName" | "productModel" | "productSpecification" | "productType" | "unit" | "quantity">[], directDelivery = false): string {
  const groups = new Map<string, { line: (typeof items)[number]; total: bigint }>();
  for (const line of items) {
    if (line.productType !== "GOODS" || !/^\d+(?:\.\d{0,3})?$/.test(line.quantity)) continue;
    const key = `${line.productId}:${line.unit}`;
    const group = groups.get(key) ?? { line, total: 0n };
    group.total += milli(line.quantity);
    groups.set(key, group);
  }
  return directDelivery ? "直送销售：无实物库存变化" :
    [...groups.values()].map(({ line, total }) =>
      `${[line.productCode, line.productName, line.productModel, line.productSpecification].filter(Boolean).join(" · ")}：扣减 ${quantity(total)} ${line.unit}`,
    ).join("\n") || "无实物库存变化（服务不扣减库存）";
}

export function salePostingConfirmation(document: SaleDraft): string {
  const inventory = saleInventoryImpact(document.items, document.directDelivery);
  const balance = /^0(?:\.0+)?$/.test(document.totalAmount) ? "零金额，客户应收无变化" : `客户应收增加 ¥${document.totalAmount}`;
  return `确认过账 ${document.documentNo}？\n${balance}\n${inventory}`;
}
