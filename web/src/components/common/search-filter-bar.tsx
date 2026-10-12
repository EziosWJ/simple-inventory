import type { KeyboardEvent, PropsWithChildren, ReactNode } from "react";
import { ContentCard } from "@/components/common/content-card";
import { cn } from "@/lib/utils";

type SearchFilterBarProps = PropsWithChildren<{
  actions?: ReactNode;
  /** Shown at the left of the action row in the stacked layout, e.g. shortcuts. */
  leadingActions?: ReactNode;
  /**
   * Fields use the whole available width and the actions move to their own row
   * below them. This is the standard layout for business query panels.
   */
  stacked?: boolean;
  className?: string;
  onSubmit?: () => void;
}>;

export function SearchFilterBar({
  actions,
  leadingActions,
  stacked = true,
  className,
  onSubmit,
  children,
}: SearchFilterBarProps) {
  const fields = (
    <div className="grid min-w-0 flex-1 grid-cols-1 gap-space-3 sm:grid-cols-[repeat(auto-fit,minmax(0,240px))] [&>*]:min-w-0 [&>form>*]:min-w-0" onKeyDown={(event: KeyboardEvent<HTMLDivElement>) => {
      if (event.key === "Enter" && !event.nativeEvent.isComposing && event.keyCode !== 229 && onSubmit) {
        event.preventDefault();
        onSubmit();
      }
    }}>
      {children}
    </div>
  );

  return (
    <ContentCard className={cn("mb-space-4", className)} bodyClassName="p-card-compact">
      {stacked ? (
        <div className="flex flex-col gap-space-3">
          {fields}
          {(leadingActions || actions) && (
              <div className={cn("flex flex-wrap items-center gap-space-2 border-t border-border pt-space-3", leadingActions ? "justify-between" : "justify-end")}>
              {leadingActions && <div className="flex flex-wrap items-center gap-space-2">{leadingActions}</div>}
              <div className="flex flex-wrap items-center justify-end gap-space-2">{actions}</div>
            </div>
          )}
        </div>
      ) : (
        <div className="flex flex-col gap-space-3 lg:flex-row lg:items-end">
          {fields}
          {actions && <div className="flex shrink-0 flex-wrap items-center justify-end gap-space-2">{actions}</div>}
        </div>
      )}
    </ContentCard>
  );
}
