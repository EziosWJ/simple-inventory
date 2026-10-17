import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { CopyDocumentNoButton } from "./copy-document-no-button";

type Props = {
  documentNo: string;
  children: ReactNode;
  className?: string;
};

/**
 * 单据列表的行操作容器：保留原有操作按钮，并追加复制单号按钮。
 * sr-only 单号同时提供屏幕阅读器上下文和列表行文本标识。
 */
export function DocumentRowActions({ documentNo, children, className }: Props) {
  return (
    <div className={cn("flex flex-wrap items-center gap-2", className)}>
      <span className="sr-only">单据编号：{documentNo}</span>
      {children}
      <CopyDocumentNoButton documentNo={documentNo} />
    </div>
  );
}
