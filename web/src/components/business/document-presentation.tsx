import { ChevronDown } from "lucide-react";
import type { ReactNode } from "react";
import { DataTable } from "@/components/common/data-table";
import { StatusTag } from "@/components/common/status-tag";
import { formatDateTime } from "@/lib/datetime";

// Presentation only: amounts, impacts, snapshots and permissions belong to the caller.
export function DocumentSummary({ totals, hint, children }: {
  totals: { label: string; value: ReactNode }[];
  hint?: ReactNode;
  children?: ReactNode;
}) {
  return <section aria-label="单据概览" className="space-y-space-4 border-b border-border pb-space-5">
    <div className="flex flex-wrap items-end justify-between gap-space-4">
      <div className="flex flex-wrap gap-x-space-8 gap-y-space-4">{totals.map(total => <div key={total.label}>
        <p className="text-sm text-text-secondary">{total.label}</p>
        <p className="mt-space-1 text-page-title font-semibold tabular-nums">{total.value}</p>
      </div>)}</div>
      {hint && <div className="text-sm text-text-secondary">{hint}</div>}
    </div>
    {children && <div className="space-y-space-2 rounded-control bg-neutral-background px-space-4 py-space-3 text-sm text-text-secondary">{children}</div>}
  </section>;
}

export function DocumentDisclosure({ title, hint, children }: { title: string; hint?: ReactNode; children: ReactNode }) {
  return <details className="group border-t border-border py-space-4">
    <summary className="flex cursor-pointer list-none flex-wrap items-center gap-space-2 rounded-tight text-sm [&::-webkit-details-marker]:hidden">
      <ChevronDown className="h-4 w-4 shrink-0 text-text-tertiary group-open:rotate-180" aria-hidden />
      <span className="font-medium">{title}</span>
      {hint && <span className="ml-space-2 text-text-tertiary">{hint}</span>}
    </summary>
    <div className="pt-space-4">{children}</div>
  </details>;
}

export type DocumentItem = {
  productCode?: string; productName?: string; productModel?: string | null;
  productSpecification?: string | null; productType?: "GOODS" | "SERVICE";
  quantity: string; unit: string; unitPrice?: string; amount?: string; remark?: string | null;
};

export function DocumentItems({ items, title = "商品明细", empty = "暂无商品明细。" }: {
  items: readonly DocumentItem[]; title?: string; empty?: string;
}) {
  return <section aria-label={title} className="space-y-space-3">
    <h3 className="text-base font-semibold">{title}</h3>
    <DataTable columns={[
      { title: "商品 / 服务", key: "product", width: "48%", render: (_, item) => <div className="min-w-[12rem] space-y-space-1 py-space-1">
        <div className="flex flex-wrap items-center gap-space-2"><span className="font-medium">{item.productName || "商品资料待核对"}</span>{item.productType === "SERVICE" && <StatusTag tone="info">服务</StatusTag>}</div>
        <p className="break-words text-xs text-text-secondary">{[item.productCode, item.productModel, item.productSpecification].filter(Boolean).join(" · ")}</p>
        {item.remark?.trim() && <p className="whitespace-pre-wrap break-words text-sm text-text-secondary">备注：{item.remark.trim()}</p>}
      </div> },
      { title: "数量", key: "quantity", width: "16%", align: "right", nowrap: true, render: (_, item) => `${item.quantity} ${item.unit}` },
      { title: "成交单价", key: "price", width: "18%", align: "right", nowrap: true, render: (_, item) => item.unitPrice !== undefined ? `¥${item.unitPrice}` : "—" },
      { title: "金额", key: "amount", width: "18%", align: "right", nowrap: true, render: (_, item) => <span className="font-medium tabular-nums">{item.amount !== undefined ? `¥${item.amount}` : "—"}</span> },
    ]} dataSource={items.map((item, index) => ({ ...item, rowKey: index }))} rowKey="rowKey" minWidth={760} empty={empty} />
  </section>;
}

export function DocumentOperationRecords({ document }: { document: {
  createdByName?: string; createTime?: string; postedByName?: string; postedAt?: string | null;
  cancelledByName?: string; cancelledAt?: string | null;
} }) {
  const records = [
    { label: "创建单据", operator: document.createdByName, time: document.createTime },
    ...(document.postedAt ? [{ label: "过账生效", operator: document.postedByName, time: document.postedAt }] : []),
    ...(document.cancelledAt ? [{ label: document.postedAt ? "整单冲销取消" : "取消草稿", operator: document.cancelledByName, time: document.cancelledAt }] : []),
  ];
  return <DocumentDisclosure title="操作记录" hint={document.postedAt ? `过账于 ${formatDateTime(document.postedAt)}` : document.createTime ? `创建于 ${formatDateTime(document.createTime)}` : undefined}>
    <ol className="ml-space-2 border-l border-border">{records.map(record => <li key={record.label} className="relative pb-space-4 pl-space-5 last:pb-0">
      <span className="absolute -left-1 top-2 h-2 w-2 rounded-full bg-border" aria-hidden />
      <p className="text-sm font-medium">{record.label}</p>
      <p className="mt-space-1 flex flex-wrap gap-x-space-4 text-sm text-text-secondary"><span>{record.operator || "—"}</span><span>{record.time ? formatDateTime(record.time) : "—"}</span></p>
    </li>)}</ol>
  </DocumentDisclosure>;
}
