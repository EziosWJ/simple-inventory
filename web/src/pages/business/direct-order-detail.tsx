import { DirectOrderActions } from "@/components/business/direct-order-actions";
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
    { title: "商品 / 服务", key: "product", render: (_, line) => <div>{line.productCode} · {line.productName}<span className="block text-text-tertiary">{line.productType === "SERVICE" ? "服务" : "实物"} · {line.unit}</span></div> },
    { title: "数量", dataIndex: "quantity", align: "right" },
    { title: "采购单价", key: "purchasePrice", align: "right", render: (_, line) => line.productType === "SERVICE" ? "—" : `¥${line.purchasePrice}` },
    { title: "销售单价", dataIndex: "salePrice", align: "right", render: value => `¥${value}` },
    { title: "采购金额", key: "purchaseAmount", align: "right", render: (_, line) => line.productType === "SERVICE" ? "—" : `¥${line.purchaseAmount}` },
    { title: "销售金额", dataIndex: "saleAmount", align: "right", render: value => `¥${value}` },
    { title: "备注", dataIndex: "remark" },
  ];
  return <div className="space-y-space-4">
    <PageHeader title={document ? `直送详情 · ${document.documentNo}` : "直送详情"} description="查询供应商直接交付客户的业务资料。" actions={<Link className="text-primary hover:underline" to="/business/direct-orders">返回列表</Link>} />
    {loading && <p role="status">加载直送详情…</p>}
    {error && <div role="alert" className="text-error">{error} <Button onClick={() => setReload(n => n + 1)}>重新加载</Button></div>}
    {document && <>
      <DirectOrderActions document={document} onChange={setDocument} />
      <ContentCard title="业务资料" extra={<div className="flex items-center gap-space-3"><StatusTag tone={document.status === "POSTED" ? "success" : document.status === "CANCELLED" ? "neutral" : "warning"}>{directStatus[document.status]}</StatusTag>{document.status === "DRAFT" && <Link className="text-primary hover:underline" to={`/business/direct-orders/${document.id}/edit`}>编辑草稿</Link>}</div>}>
        <div className="grid gap-space-4 md:grid-cols-3"><DetailItem label="供应商" value={document.supplierName} /><DetailItem label="客户" value={document.customerName} /><DetailItem label="业务日期" value={document.businessDate} /><DetailItem label="采购合计" value={`¥${document.purchaseTotal}`} /><DetailItem label="销售合计" value={`¥${document.saleTotal}`} /><DetailItem label="版本" value={document.version} /><DetailItem label="创建人" value={document.createdByName} /><DetailItem label="创建时间" value={formatDateTime(document.createTime)} /><DetailItem label="整单备注" value={document.remark} /><DetailItem label="确认人" value={document.postedByName} /><DetailItem label="确认时间" value={document.postedAt ? formatDateTime(document.postedAt) : undefined} /><DetailItem label="取消人" value={document.cancelledByName} /><DetailItem label="取消时间" value={document.cancelledAt ? formatDateTime(document.cancelledAt) : undefined} /><DetailItem label="取消原因" value={document.cancelReason} /></div>
      </ContentCard>
      <ContentCard title="送货资料"><div className="grid gap-space-4 md:grid-cols-3"><DetailItem label="送货联系人" value={document.deliveryContact} /><DetailItem label="送货电话" value={document.deliveryPhone} /><DetailItem label="送货地址" value={document.deliveryAddress} /></div></ContentCard>
      <ContentCard title="直送明细" bodyClassName="p-0"><DataTable columns={columns} dataSource={document.items} rowKey="id" /></ContentCard>
    </>}
  </div>;
}
