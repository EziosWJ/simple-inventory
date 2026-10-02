import { AlertTriangle, X } from "lucide-react";
import { useEffect, useRef, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

type ConfirmDialogProps = {
  open: boolean;
  title: string;
  description?: ReactNode;
  confirmText?: string;
  cancelText?: string;
  danger?: boolean;
  loading?: boolean;
  role?: "dialog" | "alertdialog";
  focusCancel?: boolean;
  restoreFocus?: boolean;
  suspended?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

export function ConfirmDialog({
  open,
  title,
  description,
  confirmText = "确认",
  cancelText = "取消",
  danger = false,
  loading = false,
  role = "dialog",
  focusCancel = false,
  restoreFocus = false,
  suspended = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open || !focusCancel || loading || suspended) return;
    // Overlay mousedown can blur focus after its close handler renders this
    // confirmation. Focus after that native event has finished.
    const frame = window.requestAnimationFrame(() => cancelRef.current?.focus({ preventScroll: true }));
    return () => window.cancelAnimationFrame(frame);
  }, [open, focusCancel, loading, suspended]);
  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onCancel();
      }}
      closeOnEscape={!loading && !suspended}
      closeOnOverlayClick={!loading && !suspended}
      trapFocus={!suspended}
      restoreFocus={restoreFocus}
    >
      <DialogOverlay />
      <DialogContent role={role} className="max-w-modal-sm">
        <DialogHeader>
          <div className="flex min-w-0 items-start gap-space-3">
            {danger && (
              <span className="mt-0.5 flex h-control-sm w-8 shrink-0 items-center justify-center rounded-control bg-error-background text-error">
                <AlertTriangle className="h-4 w-4" aria-hidden />
              </span>
            )}
            <div className="min-w-0">
              <DialogTitle>{title}</DialogTitle>
              {description && (
                <DialogDescription className="text-sm">
                  {description}
                </DialogDescription>
              )}
            </div>
          </div>
          <DialogClose disabled={loading} aria-label="关闭确认弹窗">
            <X className="h-4 w-4" aria-hidden />
          </DialogClose>
        </DialogHeader>
        <DialogFooter className="border-t-0">
          <Button ref={cancelRef} variant="secondary" disabled={loading} onClick={onCancel}>
            {cancelText}
          </Button>
          <Button
            variant={danger ? "danger" : "primary"}
            disabled={loading}
            onClick={onConfirm}
            className={cn(loading && "cursor-wait")}
          >
            {loading ? "处理中..." : confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
