import { useEffect, useState } from "react";
import { getPurchase, getSale, getPurchaseReturn, getSaleReturn, partnerBalanceEntry, type PartnerBalanceEntry } from "@/api/business";
import { DocumentItems, DocumentOperationRecords, type DocumentItem } from "@/components/business/document-presentation";
import { Button } from "@/components/ui/button";
import { StatusTag } from "@/components/common/status-tag";
import { balanceDescription, cents, entryTypeLabel, money, paymentMethodLabel } from "@/lib/partner-ledger";
import { formatDateTime } from "@/lib/datetime";

type SourceDocument = {
  documentNo: string; partnerName: string; businessDate: string; status: "DRAFT" | "POSTED" | "CANCELLED";
  totalAmount: string; items: DocumentItem[]; sourceNo?: string; remark?: string | null; cancelReason?: string | null;
  createdByName?: string; createTime?: string; postedByName?: string; postedAt?: string | null;
  cancelledByName?: string; cancelledAt?: string | null;
};
type Source = { entry: PartnerBalanceEntry; document?: SourceDocument };
const statusLabel = { DRAFT: "草稿", POSTED: "已过账", CANCELLED: "已取消" };

async function loadSource(entry: PartnerBalanceEntry): Promise<Source> {
  const original = entry.entryType === "REVERSAL" && entry.reversesId ? await partnerBalanceEntry(entry.reversesId) : entry;
  if (original.saleReturnId) {
    const document = await getSaleReturn(original.saleReturnId);
    return { entry: original, document: { ...document, sourceNo: document.saleNo } };
  }
  if (original.purchaseReturnId) {
    const document = await getPurchaseReturn(original.purchaseReturnId);
    return { entry: original, document: { ...document, sourceNo: document.purchaseNo } };
  }
  if (original.saleId) return { entry: original, document: await getSale(original.saleId) };
  if (original.purchaseId) return { entry: original, document: await getPurchase(original.purchaseId) };
  if (["OPENING", "RECEIPT", "PAYMENT", "CUSTOMER_REFUND", "SUPPLIER_REFUND"].includes(original.entryType)) {
    return { entry: await partnerBalanceEntry(original.id) };
  }
  throw new Error("这笔记录缺少来源关联，无法读取单据细节。");
}

export function LedgerSourceDetails({ entry }: { entry: PartnerBalanceEntry }) {
  const [source, setSource] = useState<Source | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    setError(""); setSource(null);
    void loadSource(entry).then(value => { if (active) setSource(value); })
      .catch(e => { if (active) setError(e instanceof Error ? e.message : "读取来源细节失败"); });
    return () => { active = false; };
  }, [entry, revision]);
  if (error) return <div role="alert" className="space-y-2 p-3 text-sm"><p className="text-error">{error}</p><Button size="sm" onClick={() => setRevision(v => v + 1)}>重试细节</Button></div>;
  if (!source) return <p role="status" className="p-3 text-sm text-text-secondary">正在读取单据细节…</p>;
  const document = source.document, record = source.entry, amount = cents(record.amount);
  return <section aria-label="来源内容细节" className="space-y-4 rounded-control border border-border bg-surface p-4 text-sm">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div><h3 className="font-semibold">{entry.entryType === "REVERSAL" ? "被冲销的原业务" : entryTypeLabel[record.entryType]}</h3><p className="mt-1 break-all text-text-secondary">{document?.documentNo ?? record.documentNo}</p></div>
      <StatusTag tone="neutral">{document ? statusLabel[document.status] : record.reversedById ? "已冲销" : "已生效"}</StatusTag>
    </div>
    {document ? <>
      <div className="flex flex-wrap gap-x-6 gap-y-2"><span>往来单位：{document.partnerName}</span><span>业务日期：{document.businessDate}</span><strong className="tabular-nums">单据金额：¥{document.totalAmount}</strong></div>
      <p className="text-xs text-text-tertiary">此处展示来源单据当前状态；往来记录的金额与前后余额保留本笔生效时的数据。</p>
      {document.sourceNo && <p>原单：{document.sourceNo}</p>}
      <DocumentItems items={document.items} title={document.sourceNo ? "本次退货商品明细" : "商品与服务明细"} />
      {document.remark && <p className="whitespace-pre-wrap break-words">单据备注：{document.remark}</p>}
      {document.cancelReason && <p className="whitespace-pre-wrap break-words text-error">取消原因：{document.cancelReason}</p>}
      <DocumentOperationRecords document={document} />
    </> : <>
      <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-[auto_1fr]">
        <dt>往来单位</dt><dd>{record.partnerName}</dd>
        <dt>本次金额</dt><dd className="font-semibold tabular-nums">¥{amount === null ? "—" : money(amount < 0n ? -amount : amount)}</dd>
        <dt>本笔生效前</dt><dd>{balanceDescription(record.direction, record.balanceBefore)}</dd>
        <dt>本笔生效后</dt><dd>{balanceDescription(record.direction, record.balanceAfter)}</dd>
        <dt>业务日期</dt><dd>{record.businessDate}</dd>
        <dt>实际生效</dt><dd>{formatDateTime(record.effectiveAt)}</dd>
        <dt>操作人</dt><dd>{record.operatorName || record.operatorId}</dd>
        {record.paymentMethod && <><dt>方式</dt><dd>{paymentMethodLabel[record.paymentMethod] ?? record.paymentMethod}</dd></>}
        {record.transactionNo && <><dt>交易流水号</dt><dd className="break-all">{record.transactionNo}</dd></>}
        {record.description && <><dt>说明</dt><dd className="whitespace-pre-wrap break-words">{record.description}</dd></>}
      </dl>
    </>}
  </section>;
}

export function LedgerSourceDisclosure({ entry }: { entry: PartnerBalanceEntry }) {
  const [expanded, setExpanded] = useState(false);
  return <div className="space-y-3">
    <Button size="sm" aria-expanded={expanded} onClick={() => setExpanded(v => !v)}>{expanded ? "收起细节" : "展示细节"}</Button>
    {expanded && <LedgerSourceDetails entry={entry} />}
  </div>;
}
