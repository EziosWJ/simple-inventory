import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";

type PaginationProps = {
  page: number;
  pageSize: number;
  total: number;
  pageSizeOptions?: number[];
  disabled?: boolean;
  className?: string;
  compact?: boolean;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
};

function clampPage(page: number, pageCount: number) {
  return Math.min(Math.max(page, 1), pageCount);
}

export function Pagination({
  page,
  pageSize,
  total,
  pageSizeOptions = [10, 20, 50, 100],
  disabled = false,
  className,
  compact = false,
  onPageChange,
  onPageSizeChange,
}: PaginationProps) {
  const safeTotal = Math.max(total, 0);
  const safePageSize = Math.max(pageSize, 1);
  const pageCount = Math.max(Math.ceil(safeTotal / safePageSize), 1);
  const currentPage = clampPage(page, pageCount);
  const start = safeTotal === 0 ? 0 : (currentPage - 1) * safePageSize + 1;
  const end = Math.min(currentPage * safePageSize, safeTotal);
  const canPrevious = currentPage > 1 && !disabled;
  const canNext = currentPage < pageCount && !disabled;

  return (
    <div
      className={cn(
        compact
          ? "flex w-full min-w-0 flex-col gap-space-2 text-sm text-text-secondary"
          : "flex flex-col gap-space-3 border-t border-border px-card py-space-3 text-sm text-text-secondary md:flex-row md:items-center md:justify-between",
        className,
      )}
    >
      <div className={cn("tabular-nums", compact && "whitespace-nowrap")}>
        共 <span className="font-medium text-text-primary">{safeTotal}</span>{" "}
        条
        {safeTotal > 0 && (
          <>
            ，当前{" "}
            <span className="font-medium text-text-primary">
              {start}-{end}
            </span>
          </>
        )}
      </div>

      <div className={cn("flex flex-wrap items-center gap-space-2", compact && "w-full justify-between")}>
        {onPageSizeChange && (
          <label className="flex items-center gap-space-2">
            <span>每页</span>
            <Select
              className="h-control-sm w-[84px]"
              value={String(safePageSize)}
              disabled={disabled}
              onChange={(event) => onPageSizeChange(Number(event.target.value))}
              aria-label="每页条数"
            >
              {pageSizeOptions.map((option) => (
                <option key={option} value={String(option)}>
                  {option} 条
                </option>
              ))}
            </Select>
          </label>
        )}

        <div className={cn("flex items-center gap-space-2 tabular-nums", compact && "w-full justify-between")}>
          <Button
            size="sm"
            variant="secondary"
            className={compact ? "shrink-0 px-space-2" : undefined}
            disabled={!canPrevious}
            onClick={() => onPageChange(currentPage - 1)}
            aria-label="上一页"
          >
            <ChevronLeft className="h-4 w-4" aria-hidden />
            上一页
          </Button>
          <span className={cn("min-w-16 text-center", compact && "min-w-0 px-1")}>
            {currentPage} / {pageCount}
          </span>
          <Button
            size="sm"
            variant="secondary"
            className={compact ? "shrink-0 px-space-2" : undefined}
            disabled={!canNext}
            onClick={() => onPageChange(currentPage + 1)}
            aria-label="下一页"
          >
            下一页
            <ChevronRight className="h-4 w-4" aria-hidden />
          </Button>
        </div>
      </div>
    </div>
  );
}
