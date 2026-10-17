import { Check, Copy, Eye, EyeOff } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";

export type TokenValueResult = {
  /** 令牌原值；仅存在于内存 state，关闭后立即清除。 */
  token: string;
  permanent: boolean;
  expiresAt: string | null;
  /** 附加提示，例如创建/重置后的说明。 */
  notice?: string;
};

type TokenValueDialogProps = {
  result: TokenValueResult | null;
  onClose: () => void;
};

export function TokenValueDialog({ result, onClose }: TokenValueDialogProps) {
  const [visible, setVisible] = useState(false);
  const [copied, setCopied] = useState(false);

  // 每次打开都以隐藏状态开始，避免残留明文。
  useEffect(() => {
    if (result) {
      setVisible(false);
      setCopied(false);
    }
  }, [result]);

  if (!result) return null;

  const mask = "•".repeat(Math.min(Math.max(result.token.length, 12), 48));

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(result.token);
      setCopied(true);
      toast.success("令牌已复制到剪贴板");
    } catch {
      toast.error({
        title: "复制失败",
        description: "浏览器拒绝了剪贴板访问，请手动选择并复制。",
      });
    }
  };

  return (
    <Dialog
      open={Boolean(result)}
      onOpenChange={(nextOpen) => { if (!nextOpen) onClose(); }}
      closeOnEscape={false}
      closeOnOverlayClick={false}
    >
      <DialogOverlay />
      <DialogContent className="max-w-modal-md">
        <DialogHeader>
          <DialogTitle>令牌原值</DialogTitle>
          <DialogDescription className="text-sm">
            {result.notice ?? "请立即复制并妥善保存，关闭后页面不再显示该原值。"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-none space-y-space-3 overflow-visible px-5 py-4">
          <div className="break-all rounded-control border border-border bg-neutral-background px-space-4 py-space-3 font-mono text-sm tabular-nums text-text-primary">
            {visible ? result.token : mask}
          </div>
          <div className="flex flex-wrap items-center gap-space-2">
            <Button size="sm" variant="secondary" onClick={() => setVisible((value) => !value)}>
              {visible ? <EyeOff className="h-4 w-4" aria-hidden /> : <Eye className="h-4 w-4" aria-hidden />}
              {visible ? "隐藏" : "显示"}
            </Button>
            <Button size="sm" variant="primary" onClick={() => void copy()}>
              {copied ? <Check className="h-4 w-4" aria-hidden /> : <Copy className="h-4 w-4" aria-hidden />}
              {copied ? "已复制" : "复制"}
            </Button>
          </div>
        </DialogBody>
        <DialogFooter className="border-t-0 px-5 py-4">
          <Button variant="primary" onClick={onClose}>
            知道了
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
