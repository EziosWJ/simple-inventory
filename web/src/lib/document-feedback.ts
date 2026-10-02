type PostedDocument = {
  directDelivery?: boolean;
  totalAmount: string;
  items: { productType: string }[];
};

export function postingImpact(document: PostedDocument, kind: "purchase" | "sale") {
  const inventory = document.directDelivery || document.items.every(item => item.productType === "SERVICE")
    ? "库存无变化"
    : kind === "purchase" ? "库存已增加" : "库存已扣减";
  const balance = /^0(?:\.0+)?$/.test(document.totalAmount)
    ? "零金额，往来余额无变化"
    : kind === "purchase" ? "应付款已增加" : "应收款已增加";
  return `${inventory}；${balance}。`;
}

export function cancellationImpact(document: PostedDocument & { status: string }, kind: "purchase" | "sale") {
  if (document.status === "DRAFT") return "草稿已取消，库存与往来余额无变化。";
  const inventory = document.directDelivery || document.items.every(item => item.productType === "SERVICE")
    ? "库存无变化"
    : "原库存变动已冲销";
  const balance = /^0(?:\.0+)?$/.test(document.totalAmount)
    ? "零金额，往来余额无变化"
    : kind === "purchase" ? "原应付款已冲销" : "原应收款已冲销";
  return `${inventory}；${balance}。原记录与冲销记录均保留。`;
}
