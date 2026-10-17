import { Check, Copy } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { copyText } from "@/lib/clipboard";

type Props = {
  documentNo: string;
  className?: string;
  variant?: "ghost" | "secondary";
};

/** 列表行内的紧凑复制按钮；单号本身只在行内以 sr-only 文本保留。 */
export function CopyDocumentNoButton({ documentNo, className, variant = "ghost" }: Props) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<number | null>(null);

  useEffect(() => () => {
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
  }, []);

  async function copy() {
    if (!documentNo) return;

    if (await copyText(documentNo)) {
      setCopied(true);
      toast.success({ title: "已复制单据编号", description: documentNo, duration: 2000 });
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => setCopied(false), 1500);
      return;
    }

    toast.error({
      title: "复制失败",
      description: `请手动复制单据编号：${documentNo}`,
      duration: 0,
    });
  }

  return (
    <Button
      size="sm"
      variant={variant}
      className={cn("w-8 px-0", className)}
      aria-label="复制单据编号"
      title="复制单据编号"
      onClick={() => void copy()}
    >
      {copied ? <Check className="h-4 w-4" aria-hidden /> : <Copy className="h-4 w-4" aria-hidden />}
    </Button>
  );
}
