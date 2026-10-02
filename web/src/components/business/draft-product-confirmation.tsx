import type { ProductRecord } from "@/api/business";
import { Button } from "@/components/ui/button";

export function DraftProductConfirmation({ type, unit, current, goodsOnly = false, onConfirm }: {
  type: "GOODS" | "SERVICE";
  unit: string;
  current?: ProductRecord;
  goodsOnly?: boolean;
  onConfirm: () => void;
}) {
  const changed = current && (current.type !== type || current.unit !== unit);
  const typeText = (value: string) => value === "GOODS" ? "实物" : "服务";
  return <div className="text-sm md:col-span-full">
    <p>草稿类型 / 单位：{typeText(type)} / {unit || "未选择"}</p>
    {changed && <div role="alert" className="mt-2 space-y-2 text-error">
      <p>商品档案已变化：{typeText(type)} / {unit} → {typeText(current.type)} / {current.unit}。确认前保留草稿原值；数量不会自动换算，请核对数量后确认。</p>
      {goodsOnly && current.type !== "GOODS" ? <p>采购只支持实物商品，请移除该行或重新选择实物商品。</p> : <Button type="button" variant="secondary" onClick={onConfirm}>确认使用新类型和单位</Button>}
    </div>}
  </div>;
}
