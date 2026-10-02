import { useCallback, useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { getDeliveryNote, type DeliveryNote } from "@/api/business";
import { PageHeader } from "@/components/common/page-header";
import { DeliveryNoteSheet } from "@/components/common/delivery-note-sheet";
import { Button } from "@/components/ui/button";
import "./delivery-note.css";

export function DeliveryNotePage() {
  const { id } = useParams();
  const [params] = useSearchParams();
  const [note, setNote] = useState<DeliveryNote | null>(null);
  const [error, setError] = useState("");
  const [showAmount, setShowAmount] = useState(params.get("showAmount") !== "false");
  const [loading, setLoading] = useState(false);
  const [pageCount, setPageCount] = useState(0);
  const saleId = Number(id);

  const load = useCallback(() => {
    if (!(saleId > 0)) {
      setError("销售单ID无效");
      return;
    }
    setLoading(true);
    void getDeliveryNote(saleId)
      .then((v) => { setNote(v); setError(""); })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : "送货单加载失败"))
      .finally(() => setLoading(false));
  }, [saleId]);
  useEffect(() => load(), [load]);

  return (
    <div className="space-y-space-4">
      <div className="print-hide">
        <PageHeader
          title="送货单打印"
          description="A4 纵向打印数据来自已保存单据：已过账使用当时的客户、送货和经营者快照（含空值），草稿使用当前已保存内容并标记未过账。"
          actions={
            <>
              <Button variant="secondary" onClick={() => setShowAmount((v) => !v)}>
                {showAmount ? "隐藏金额" : "显示金额"}
              </Button>
              <Button variant="secondary" onClick={load} disabled={loading}>刷新</Button>
              <Button onClick={() => window.print()} disabled={!note}>打印</Button>
            </>
          }
        />
        {error && <p role="alert" className="text-sm text-error">{error}</p>}
        {note && (
          <p className="text-sm text-text-tertiary">
            共 {pageCount} 页 · {note.posted ? "已过账正式送货单" : note.status === "CANCELLED" ? "已取消单，禁止作为有效正式单" : "未过账草稿"}
          </p>
        )}
      </div>
      {note && <DeliveryNoteSheet note={note} showAmount={showAmount} onPageCountChange={setPageCount} />}
    </div>
  );
}
