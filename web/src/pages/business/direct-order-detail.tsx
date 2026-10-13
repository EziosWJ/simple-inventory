import { DirectOrderActions } from "@/components/business/direct-order-actions";
import { DocumentDisclosure, DocumentOperationRecords, DocumentSummary } from "@/components/business/document-presentation";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { getDirectOrder, directStatus, type DirectOrder } from "@/api/direct-orders";
import { PageHeader } from "@/components/common/page-header";
import { ContentCard } from "@/components/common/content-card";
import { DetailItem } from "@/components/common/detail-item";
import { DataTable } from "@/components/common/data-table";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";

export function DirectOrderDetailPage() {
  const { id } = useParams();
  const [document, setDocument] = useState<DirectOrder | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true); setError(""); setDocument(null);
    const target = Number(id);
    if (!Number.isSafeInteger(target) || target < 1) { setError("直送单 ID 无效"); setLoading(false); return; }
    void getDirectOrder(target).then(d => { if (active) setDocument(d); })
      .catch(e => { if (active) setError(e instanceof Error ? e.message : "直送详情加载失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [id, reload]);

  const columns: DataTableColumn<DirectOrder["items"][number]>[] = [
    { title: "商品 / 服务", key: "product", width: "28%", render: (_, line) => <div className="min-w-[12rem] space-y-1"><div className="font-medium">{line.productName}</div><div className="break-words text-xs text-text-secondary">{line.productCode}{line.productType === "SERVICE" && " · 服务项目"}</div>{line.remark?.trim() && <div className="whitespace-pre-wrap break-words text-sm text-text-secondary">备注：{line.remark.trim()}</div>}</div> },
    { title: "数量", dataIndex: "quantity", align: "right", nowrap: true, render: (value, line) => `${String(value)} ${line.unit}` },
    { title: "采购单价", dataIndex: "purchasePrice", align: "right", nowrap: true, render: (value, line) => line.productType === "SERVICE" ? "—" : `¥${String(value)}` },
    { title: "采购金额", dataIndex: "purchaseAmount", align: "right", nowrap: true, render: (value, line) => line.productType === "SERVICE" ? "—" : `¥${String(value)}` },
    { title: "销售单价", dataIndex: "salePrice", align: "right", nowrap: true, render: value => `¥${String(value)}` },
    { title: "销售金额", dataIndex: "saleAmount", align: "right", nowrap: true, render: value => <span className="font-medium tabular-nums">¥{String(value)}</span> },
  ];

  const statusTone = document?.status === "POSTED" ? "success" : document?.status === "CANCELLED" ? "neutral" : "warning";
  const effectiveHint = document?.status === "POSTED"
    ? "已确认：采购与销售同时生效；实物库存保留成对流水，净变化为零；双方往来分别记录。"
    : document?.status === "CANCELLED"
      ? document.postedAt ? "直送已冲销取消，原单与流水保留。" : "草稿已取消，库存与双方往来未变化。"
      : "草稿尚未生效，库存与双方往来余额均未变化。";

  return <div className="space-y-space-4">
    <PageHeader title={document ? `直送详情 · ${document.documentNo}` : "直送详情"} description="供应商直接交付客户的业务单据。" actions={<div className="flex flex-wrap items-center gap-space-4">{document?.status === "DRAFT" && <Link className="text-primary hover:underline" to={`/business/direct-orders/${document.id}/edit`}>编辑草稿</Link>}<Link className="text-primary hover:underline" to="/business/direct-orders">返回列表</Link></div>} />
    {loading && <p role="status">加载直送详情…</p>}
    {error && <div role="alert" className="text-error">{error} <Button onClick={() => setReload(n => n + 1)}>重新加载</Button></div>}
    {document && <>
      <ContentCard title="单据概览" extra={<StatusTag tone={statusTone}>{directStatus[document.status]}</StatusTag>}>
        <DocumentSummary totals={[{ label: "采购金额", value: `¥${document.purchaseTotal}` }, { label: "销售金额", value: `¥${document.saleTotal}` }]} hint={`${document.items.length} 行 · ${document.items.filter(item => item.productType === "GOODS").length} 行实物 · ${document.items.filter(item => item.productType === "SERVICE").length} 行服务`}>
          <p>{effectiveHint}</p>
        </DocumentSummary>
        <dl className="mt-space-4 grid gap-space-4 sm:grid-cols-2 lg:grid-cols-3">
          <DetailItem label="直送单号" value={document.documentNo} />
          <DetailItem label="供应商" value={document.supplierName} />
          <DetailItem label="客户" value={document.customerName} />
          <DetailItem label="业务日期" value={document.businessDate} />
          <DetailItem label="单据状态" value={directStatus[document.status]} />
        </dl>
        {document.cancelReason && <p role="alert" className="mt-space-4 rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">取消原因：{document.cancelReason}</p>}
        {document.remark?.trim() && <div aria-label="单据备注" className="mt-space-4 border-l-2 border-border pl-space-4"><p className="text-sm font-medium">单据备注</p><p className="mt-space-1 whitespace-pre-wrap break-words text-sm text-text-secondary">{document.remark.trim()}</p></div>}
      </ContentCard>

      <ContentCard title="直送商品明细" bodyClassName="p-0">
        <DataTable columns={columns} dataSource={document.items} rowKey="id" minWidth={900} />
        <p className="border-t border-border px-card py-space-3 text-sm text-text-secondary">服务项目不计采购金额，不产生采购入库或库存流水；单据两侧金额分别用于供应商与客户方向往来。</p>
      </ContentCard>

      <ContentCard title="现有操作与关联入口"><DirectOrderActions document={document} onChange={setDocument} /></ContentCard>

      {(document.deliveryContact || document.deliveryPhone || document.deliveryAddress) && <DocumentDisclosure title="送货资料" hint="查看联系人、电话与地址">
        <dl className="grid gap-space-4 sm:grid-cols-2"><DetailItem label="送货联系人" value={document.deliveryContact || "—"} /><DetailItem label="送货电话" value={document.deliveryPhone || "—"} /><div className="sm:col-span-2"><DetailItem label="送货地址" value={document.deliveryAddress || "—"} /></div></dl>
      </DocumentDisclosure>}

      <DocumentOperationRecords document={{ createdByName: document.createdByName, createTime: document.createTime, postedByName: document.postedByName, postedAt: document.postedAt, cancelledByName: document.cancelledByName, cancelledAt: document.cancelledAt }} />
      {document.postedAt && <p className="text-xs text-text-tertiary">实际确认时间：{formatDateTime(document.postedAt)}</p>}
    </>}
  </div>;
}
