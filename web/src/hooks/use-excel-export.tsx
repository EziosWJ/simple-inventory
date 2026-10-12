import { useRef, useState } from "react";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import type { BusinessFeedback } from "@/hooks/use-business-feedback";

type ExportOptions = {
  count: () => Promise<{ total: number }>;
  download: () => Promise<{ blob: Blob; filename: string }>;
};

// Each click captures its filters. The same snapshot is used for both the
// count request and download, even if the inputs change during confirmation.
export function useExcelExport(notify: (feedback: BusinessFeedback) => void) {
  const locked = useRef(false);
  const [exporting, setExporting] = useState(false);
  const [pending, setPending] = useState<{ total: number; options: ExportOptions } | null>(null);

  function finish() { locked.current = false; setExporting(false); }
  function failed(reason: unknown) {
    notify({ type: "error", title: "导出失败", description: reason instanceof Error ? reason.message : "导出明细失败，请稍后重试" });
  }
  async function download(options: ExportOptions) {
    try {
      const { blob, filename } = await options.download();
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      try {
        link.href = url; link.download = filename; document.body.appendChild(link); link.click();
      } finally {
        link.remove(); window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      }
    } catch (reason) { failed(reason); }
    finally { finish(); }
  }
  async function start(options: ExportOptions) {
    if (locked.current) return;
    locked.current = true; setExporting(true);
    try {
      const { total } = await options.count();
      if (total > 10000) { setPending({ total, options }); return; }
      await download(options);
    } catch (reason) { failed(reason); finish(); }
  }
  const confirmation = <ConfirmDialog open={pending !== null} title="确认导出明细" description={`本次将导出 ${pending?.total ?? 0} 行明细，可能需要几秒，是否继续？`} confirmText="继续导出" onConfirm={() => { if (!pending) return; const options = pending.options; setPending(null); void download(options); }} onCancel={() => { setPending(null); finish(); }} />;
  return { exporting, start, confirmation };
}
