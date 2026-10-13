import { X } from "lucide-react";
import type { ReactNode } from "react";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

type DetailDialogProps = {
  open: boolean;
  title: string;
  description?: ReactNode;
  loading?: boolean;
  contentClassName?: string;
  headerClassName?: string;
  bodyClassName?: string;
  closeOnEscape?: boolean;
  closeOnOverlayClick?: boolean;
  trapFocus?: boolean;
  onCancel: () => void;
  children: ReactNode;
  footer?: ReactNode;
  fixedFooter?: boolean;
  documentLayout?: boolean;
};

export function DetailDialog({
  open,
  title,
  description,
  loading = false,
  contentClassName,
  headerClassName,
  bodyClassName,
  closeOnEscape = true,
  closeOnOverlayClick = true,
  trapFocus,
  onCancel,
  children,
  footer,
  documentLayout = false,
  fixedFooter = documentLayout,
}: DetailDialogProps) {
  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onCancel();
      }}
      closeOnEscape={closeOnEscape}
      closeOnOverlayClick={closeOnOverlayClick}
      trapFocus={trapFocus ?? documentLayout}
      restoreFocus={documentLayout}
    >
      <DialogOverlay />
      <DialogContent
        className={cn(
          "max-h-[calc(100vh-48px)] max-w-[860px]",
          fixedFooter && "flex flex-col",
          documentLayout && "max-h-[90dvh] max-w-[1100px] max-sm:h-dvh max-sm:max-h-dvh max-sm:w-full max-sm:rounded-none",
          contentClassName,
        )}
        aria-busy={loading || undefined}
      >
        <DialogHeader className={cn(fixedFooter && "shrink-0", headerClassName)}>
          <div className="min-w-0">
            <DialogTitle>{title}</DialogTitle>
            {description !== undefined && description !== null && (
              <DialogDescription>{description}</DialogDescription>
            )}
          </div>
          <DialogClose aria-label="关闭详情弹窗">
            <X className="h-4 w-4" aria-hidden />
          </DialogClose>
        </DialogHeader>

        <DialogBody
          className={cn(
            "max-h-[calc(100vh-150px)] px-card py-space-5",
            fixedFooter && "max-h-none flex-1",
            bodyClassName,
          )}
        >
          {children}
          {!fixedFooter && footer && <div className="mt-space-4">{footer}</div>}
        </DialogBody>
        {fixedFooter && footer && (
          <footer className="shrink-0 border-t border-border bg-surface px-card py-space-4">{footer}</footer>
        )}
      </DialogContent>
    </Dialog>
  );
}
