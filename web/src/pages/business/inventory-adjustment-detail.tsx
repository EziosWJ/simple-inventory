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
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import {
  DICT_CODES,
  INVENTORY_ADJUSTMENT_REASON_VALUES,
  INVENTORY_ADJUSTMENT_STATUS_VALUES,
  type DictSelectOption,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { businessDictLabel } from "@/lib/business-dict-label";
import { formatDateTime } from "@/lib/datetime";

const reasonFallback: DictSelectOption<AdjustmentReason>[] = [
  { value: "OPENING", label: "期初录入" }, { value: "SURPLUS", label: "盘盈" },
  { value: "SHORTAGE", label: "盘亏" }, { value: "DAMAGE", label: "报损" }, { value: "OTHER", label: "其他" },
];
const statusFallback: DictSelectOption<AdjustmentStatus>[] = [
  { value: "DRAFT", label: "草稿" }, { value: "POSTED", label: "已过账" }, { value: "CANCELLED", label: "已取消" },
];

// Read-only view of one adjustment, used where a document is opened from another
// page (for example as the source of a ledger line). The maintenance page keeps
// its own dialog because it also offers edit, post and cancel actions.
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
  return <Dialog open onOpenChange={(open) => { if (!open) onClose(); }} trapFocus restoreFocus lockScroll>
    <DialogOverlay />
    <DialogContent className="flex max-h-[90vh] w-[min(900px,96vw)] flex-col p-0">
      <DialogHeader>
        <DialogTitle>来源调整单 {adjustment.documentNo}</DialogTitle>
        <DialogClose aria-label="关闭来源调整单"><X className="h-4 w-4" aria-hidden /></DialogClose>
      </DialogHeader>
      <DialogBody className="min-h-0 overflow-auto">
        <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Detail label="调整单号" value={adjustment.documentNo} />
          <Detail label="状态" value={businessDictLabel(statusDict.options, adjustment.status)} />
          <Detail label="版本" value={String(adjustment.version)} />
          <Detail label="创建人" value={adjustment.createdByName || `用户 ${adjustment.createdBy}`} />
          <Detail label="创建时间" value={formatDateTime(adjustment.createTime)} />
          {adjustment.postedBy !== null && <>
            <Detail label="过账人" value={adjustment.postedByName || `用户 ${adjustment.postedBy}`} />
            <Detail label="过账时间" value={adjustment.postedAt ? formatDateTime(adjustment.postedAt) : "-"} />
          </>}
          {adjustment.status === "CANCELLED" && <>
            <Detail label="取消人" value={adjustment.cancelledBy === null ? "-" : adjustment.cancelledByName || `用户 ${adjustment.cancelledBy}`} />
            <Detail label="取消时间" value={adjustment.cancelledAt ? formatDateTime(adjustment.cancelledAt) : "-"} />
            <Detail label="取消原因" value={adjustment.cancelReason || "-"} />
          </>}
        </dl>
        {adjustment.status === "CANCELLED" && adjustment.postedBy !== null && <p className="mt-4 text-sm text-text-secondary">该单据曾过账并已整单冲销：下表为过账时的库存影响，冲销记录保留在库存流水中。</p>}
        <h3 className="mb-2 mt-5 font-medium">{adjustment.postedBy !== null ? "过账明细" : "调整明细"}（{items.length}）</h3>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] border-collapse text-sm">
            <thead><tr className="border-b border-border bg-background text-left"><th className="p-2">商品</th><th className="p-2">型号 / 规格</th><th className="p-2">单位</th><th className="p-2">调整数量</th><th className="p-2">原因</th>{posted && <th className="p-2">调整前 → 调整后</th>}<th className="p-2">说明</th></tr></thead>
            <tbody>{items.map((item) => <tr key={item.id} className="border-b border-border">
              <td className="p-2"><div>{item.productName}</div><div className="text-xs text-text-tertiary">{item.productCode}</div></td>
              <td className="p-2">{item.productModel || "-"} / {item.productSpecification || "-"}</td>
              <td className="p-2">{item.unit}</td>
              <td className="p-2 tabular-nums">{item.quantity}</td>
              <td className="p-2">{businessDictLabel(reasonDict.options, item.reason)}</td>
              {posted && <td className="p-2 tabular-nums">{item.balanceBefore != null && item.balanceAfter != null ? `${item.balanceBefore} → ${item.balanceAfter}` : "-"}</td>}
              <td className="max-w-56 whitespace-pre-wrap p-2">{item.remark || "-"}</td>
            </tr>)}</tbody>
          </table>
        </div>
      </DialogBody>
      <DialogFooter>
        {actions}
        <Button variant="secondary" onClick={onClose}>关闭</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}

function Detail({ label, value }: { label: string; value: string }) {
  return <div><dt className="text-xs text-text-tertiary">{label}</dt><dd className="break-words">{value}</dd></div>;
}
