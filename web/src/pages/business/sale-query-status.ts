export function statusText(s: string) {
  return { DRAFT: "草稿", POSTED: "已过账", CANCELLED: "已取消" }[s] ?? s;
}

export function statusTone(s: string): "success" | "warning" | "neutral" {
  return s === "POSTED" ? "success" : s === "DRAFT" ? "warning" : "neutral";
}
