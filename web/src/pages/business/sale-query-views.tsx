import { useState } from "react";
import type { SaleItemSummary } from "@/api/business";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";
import { statusText, statusTone } from "./sale-query-status";

export function ViewSwitch({
  view,
  onChange,
}: {
  view: "document" | "item";
  onChange: (next: "document" | "item") => void;
}) {
  const options = [
    { value: "document", label: "按单据" },
    { value: "item", label: "按明细" },
  ] as const;

  return (
    <div
      role="group"
      aria-label="销售浏览视图"
      className="flex shrink-0 gap-1 rounded-control border border-border p-0.5"
    >
      {options.map((option) => (
        <Button
          key={option.value}
          size="sm"
          variant={view === option.value ? "primary" : "ghost"}
          aria-pressed={view === option.value}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  );
}

export function ItemSummaryPanel({
  summary,
  loading,
  error,
}: {
  summary?: SaleItemSummary;
  loading: boolean;
  error: string;
}) {
  const [quantitiesOpen, setQuantitiesOpen] = useState(false);

  return (
    <section
      className="space-y-3 border-b border-border px-card py-4"
      aria-label="销售明细全结果合计"
      aria-busy={loading}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h3 className="text-sm font-semibold">全部匹配明细合计</h3>
          <p className="text-xs text-text-tertiary">
            按销售业务日期筛选，扣除截至当前有效退货；草稿与已取消销售不计入净销售。
          </p>
        </div>
        {!loading && !error && summary && (
          <Button
            size="sm"
            variant="secondary"
            className="shrink-0"
            aria-label={`查看数量合计，${summary.quantities.length} 组`}
            onClick={() => setQuantitiesOpen(true)}
          >
            查看数量合计
            <span className="ml-1 text-text-tertiary">
              {summary.quantities.length} 组
            </span>
          </Button>
        )}
      </div>

      {loading ? (
        <p role="status" className="py-2 text-sm text-text-secondary">
          合计加载中…
        </p>
      ) : error || !summary ? (
        <p className="py-2 text-sm text-error">合计不可用，请重试查询。</p>
      ) : (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
          {summary.amounts.map((amount) => {
            const isPosted = amount.status === "POSTED";
            return (
              <article
                key={amount.status}
                data-testid="sale-summary-card"
                className={`min-w-0 rounded-control border p-3 ${
                  isPosted
                    ? "border-primary/30 bg-primary/5"
                    : "border-border bg-surface"
                }`}
              >
                <div className="flex items-center justify-between gap-2">
                  <StatusTag tone={statusTone(amount.status)}>
                    {statusText(amount.status)}
                  </StatusTag>
                  <span className="text-xs text-text-tertiary">
                    {amount.itemCount} 条明细
                  </span>
                </div>

                {isPosted && amount.netAmount != null ? (
                  <div className="mt-3">
                    <p className="text-xs text-text-secondary">净销售额</p>
                    <p className="text-xl font-semibold tabular-nums text-primary">
                      ¥{amount.netAmount}
                    </p>
                  </div>
                ) : (
                  <p className="mt-3 text-sm text-text-secondary">
                    不计入净销售额
                  </p>
                )}

                <dl className="mt-3 grid grid-cols-2 gap-x-3 border-t border-border/70 pt-2 text-xs">
                  <div>
                    <dt className="text-text-tertiary">原金额</dt>
                    <dd className="mt-0.5 font-medium tabular-nums text-text-primary">
                      ¥{amount.originalAmount}
                    </dd>
                  </div>
                  {amount.returnedAmount != null && (
                    <div>
                      <dt className="text-text-tertiary">有效退货</dt>
                      <dd className="mt-0.5 font-medium tabular-nums text-text-primary">
                        ¥{amount.returnedAmount}
                      </dd>
                    </div>
                  )}
                </dl>
              </article>
            );
          })}
        </div>
      )}

      {summary && (
        <Dialog
          open={quantitiesOpen}
          onOpenChange={setQuantitiesOpen}
          trapFocus
          restoreFocus
          lockScroll
        >
          <DialogOverlay />
          <DialogContent className="flex max-h-[calc(100dvh-32px)] w-[calc(100%-24px)] max-w-4xl flex-col p-0">
            <DialogHeader className="items-start">
              <div>
                <DialogTitle>数量合计</DialogTitle>
                <p className="mt-1 text-sm text-text-tertiary">
                  按商品身份、状态和单位汇总，共 {summary.quantities.length} 组
                </p>
              </div>
              <DialogClose
                className="w-auto px-3"
                aria-label="关闭数量合计"
              >
                关闭
              </DialogClose>
            </DialogHeader>
            <DialogBody className="min-h-0 p-0">
              {summary.quantities.length === 0 ? (
                <p className="p-card text-sm text-text-secondary">
                  当前筛选没有数量明细。
                </p>
              ) : (
                <ul
                  aria-label="数量合计明细"
                  className="space-y-2 p-3 md:hidden"
                >
                  {summary.quantities.map((group) => (
                    <li
                      key={`${group.status}-${group.productId}-${group.unit}`}
                      className="rounded-control border border-border p-3"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <p className="break-words font-medium text-text-primary">
                            {group.productName}
                          </p>
                          <p className="text-xs text-text-tertiary">
                            {group.productCode || "无编码"}
                          </p>
                        </div>
                        <StatusTag tone={statusTone(group.status)}>
                          {statusText(group.status)}
                        </StatusTag>
                      </div>
                      <dl className="mt-3 grid grid-cols-3 gap-2 border-t border-border pt-2 text-xs">
                        <div className="min-w-0">
                          <dt className="text-text-tertiary">销售数量</dt>
                          <dd className="mt-1 break-words font-medium tabular-nums">
                            {group.quantity} {group.unit}
                          </dd>
                        </div>
                        <div className="min-w-0">
                          <dt className="text-text-tertiary">已退数量</dt>
                          <dd className="mt-1 break-words tabular-nums text-text-secondary">
                            {group.returnsApplicable
                              ? `${group.returnedQuantity ?? "0"} ${group.unit}`
                              : "不适用"}
                          </dd>
                        </div>
                        <div className="min-w-0">
                          <dt className="text-text-tertiary">净数量</dt>
                          <dd className="mt-1 break-words font-medium tabular-nums">
                            {group.netQuantity != null
                              ? `${group.netQuantity} ${group.unit}`
                              : "不适用"}
                          </dd>
                        </div>
                      </dl>
                    </li>
                  ))}
                </ul>
              )}
              {summary.quantities.length > 0 && (
                <div className="hidden max-h-[min(60vh,560px)] overflow-auto md:block">
                  <table
                    aria-label="数量合计明细"
                    className="w-full min-w-[680px] border-collapse text-left text-sm"
                  >
                    <thead className="sticky top-0 z-10 bg-neutral-background text-xs text-text-secondary">
                      <tr>
                        <th scope="col" className="px-4 py-3 font-medium">
                          状态
                        </th>
                        <th scope="col" className="px-4 py-3 font-medium">
                          商品或服务
                        </th>
                        <th scope="col" className="px-4 py-3 font-medium">
                          单位
                        </th>
                        <th scope="col" className="px-4 py-3 text-right font-medium">
                          销售数量
                        </th>
                        <th scope="col" className="px-4 py-3 text-right font-medium">
                          已退数量
                        </th>
                        <th scope="col" className="px-4 py-3 text-right font-medium">
                          净数量
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {summary.quantities.map((group) => (
                        <tr
                          key={`${group.status}-${group.productId}-${group.unit}`}
                          className="border-t border-border"
                        >
                          <td className="whitespace-nowrap px-4 py-3">
                            <StatusTag tone={statusTone(group.status)}>
                              {statusText(group.status)}
                            </StatusTag>
                          </td>
                          <td className="px-4 py-3">
                            <span className="block font-medium text-text-primary">
                              {group.productName}
                            </span>
                            <span className="text-xs text-text-tertiary">
                              {group.productCode || "无编码"}
                            </span>
                          </td>
                          <td className="whitespace-nowrap px-4 py-3 text-text-secondary">
                            {group.unit}
                          </td>
                          <td className="whitespace-nowrap px-4 py-3 text-right tabular-nums">
                            {group.quantity} {group.unit}
                          </td>
                          <td className="whitespace-nowrap px-4 py-3 text-right tabular-nums text-text-secondary">
                            {group.returnsApplicable
                              ? `${group.returnedQuantity ?? "0"} ${group.unit}`
                              : "不适用"}
                          </td>
                          <td className="whitespace-nowrap px-4 py-3 text-right font-medium tabular-nums">
                            {group.netQuantity != null
                              ? `${group.netQuantity} ${group.unit}`
                              : "不适用"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </DialogBody>
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
