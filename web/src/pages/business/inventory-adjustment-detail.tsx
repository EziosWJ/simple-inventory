import { type ReactNode } from "react";
import { X } from "lucide-react";
import type { AdjustmentReason, AdjustmentStatus, InventoryAdjustment } from "@/api/business";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { StatusTag } from "@/components/common/status-tag";
import { DocumentSummary, DocumentOperationRecords } from "@/components/business/document-presentation";
import {
  DICT_CODES,
  INVENTORY_ADJUSTMENT_REASON_VALUES,
  INVENTORY_ADJUSTMENT_STATUS_VALUES,
  type DictSelectOption,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { businessDictLabel } from "@/lib/business-dict-label";

const reasonFallback: DictSelectOption<AdjustmentReason>[] = [
  { value: "OPENING", label: "期初录入" }, { value: "SURPLUS", label: "盘盈" },
  { value: "SHORTAGE", label: "盘亏" }, { value: "DAMAGE", label: "报损" }, { value: "OTHER", label: "其他" },
];
const statusFallback: DictSelectOption<AdjustmentStatus>[] = [
  { value: "DRAFT", label: "草稿" }, { value: "POSTED", label: "已过账" }, { value: "CANCELLED", label: "已取消" },
];

// Shared read-only body; the maintenance page supplies its own business actions.
export function InventoryAdjustmentDetail({ adjustment, actions, onClose }: {
  adjustment: InventoryAdjustment;
  // Business actions (edit, post, cancel) stay with the page that owns them.
  actions?: ReactNode;
  onClose: () => void;
}) {
  const statusDict = useDictOptions<AdjustmentStatus>(DICT_CODES.INVENTORY_ADJUSTMENT_STATUS, {
    allowedValues: INVENTORY_ADJUSTMENT_STATUS_VALUES, fallback: statusFallback,
  });
  const reasonDict = useDictOptions<AdjustmentReason>(DICT_CODES.INVENTORY_ADJUSTMENT_REASON, {
    allowedValues: INVENTORY_ADJUSTMENT_REASON_VALUES, fallback: reasonFallback,
  });
  const items = adjustment.items ?? [];
  const posted = items.some((item) => item.balanceBefore != null && item.balanceAfter != null);
  const increases = items.filter(item => !item.quantity.startsWith("-")).length;
  const wasPosted = adjustment.postedBy != null || Boolean(adjustment.postedAt);
  return <Dialog open onOpenChange={(open) => { if (!open) onClose(); }} trapFocus restoreFocus lockScroll>
    <DialogOverlay />
    <DialogContent className="flex max-h-[90dvh] max-w-[1100px] flex-col p-0 max-sm:h-dvh max-sm:max-h-dvh max-sm:w-full max-sm:rounded-none">
      <DialogHeader className="shrink-0 py-space-5">
        <div className="min-w-0"><DialogTitle>库存调整单 · {adjustment.documentNo}</DialogTitle>
          <DialogDescription className="mt-space-2"><StatusTag tone={adjustment.status === "DRAFT" ? "warning" : adjustment.status === "POSTED" ? "success" : "neutral"}>{businessDictLabel(statusDict.options, adjustment.status)}</StatusTag></DialogDescription>
        </div>
        <DialogClose aria-label="关闭来源调整单"><X className="h-4 w-4" aria-hidden /></DialogClose>
      </DialogHeader>
      <DialogBody className="min-h-0 flex-1 space-y-space-6 sm:px-space-6">
        <DocumentSummary totals={[{ label: "调整明细", value: `共 ${items.length} 项` }]} hint={`增加 ${increases} 项 · 减少 ${items.length - increases} 项`}>
          <p>{adjustment.status === "DRAFT" ? "尚未生效，库存未变化；本单不产生往来金额。" : adjustment.status === "CANCELLED" ? wasPosted ? "该单据曾过账并已整单冲销：下表为过账时的库存影响，冲销记录保留在库存流水中。" : "草稿已取消，库存未变化。" : "库存调整已生效，本单不产生往来金额。"}</p>
          {adjustment.cancelReason && <p className="whitespace-pre-wrap break-words text-text-primary">取消原因：{adjustment.cancelReason}</p>}
        </DocumentSummary>
        <h3 className="font-semibold">{wasPosted ? "过账明细" : "调整明细"}</h3>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] border-collapse text-sm">
            <thead><tr className="border-b border-border bg-neutral-background text-left"><th className="px-space-5 py-space-3">商品</th><th className="px-space-5 py-space-3 text-right">调整数量</th><th className="px-space-5 py-space-3">原因</th>{posted && <th className="px-space-5 py-space-3 text-right">调整前 → 调整后</th>}<th className="px-space-5 py-space-3">说明</th></tr></thead>
            <tbody>{items.map((item) => <tr key={item.id} className="border-b border-border">
              <td className="px-space-5 py-space-3"><div className="font-medium">{item.productName}</div><div className="text-xs text-text-secondary">{[item.productCode, item.productModel, item.productSpecification].filter(Boolean).join(" · ")}</div></td>
              <td className="whitespace-nowrap px-space-5 py-space-3 text-right tabular-nums">{item.quantity.startsWith("-") ? `减少 ${item.quantity.slice(1)}` : `增加 ${item.quantity}`} {item.unit}</td>
              <td className="px-space-5 py-space-3">{businessDictLabel(reasonDict.options, item.reason)}</td>
              {posted && <td className="whitespace-nowrap px-space-5 py-space-3 text-right tabular-nums">{item.balanceBefore != null && item.balanceAfter != null ? `${item.balanceBefore} → ${item.balanceAfter} ${item.unit}` : "—"}</td>}
              <td className="max-w-56 whitespace-pre-wrap break-words px-space-5 py-space-3">{item.remark || "—"}</td>
            </tr>)}</tbody>
          </table>
        </div>
        <DocumentOperationRecords document={{ ...adjustment, createdByName: adjustment.createdByName || `用户 ${adjustment.createdBy}`, postedByName: adjustment.postedByName || (adjustment.postedBy != null ? `用户 ${adjustment.postedBy}` : undefined), cancelledByName: adjustment.cancelledByName || (adjustment.cancelledBy != null ? `用户 ${adjustment.cancelledBy}` : undefined) }} />
      </DialogBody>
      <DialogFooter className="shrink-0 flex-wrap">
        {actions}
        <Button variant="secondary" onClick={onClose}>关闭</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
