import type { SaleDraft } from "@/api/business";
import { DetailDialog } from "@/components/common/detail-dialog";
import { StatusTag } from "@/components/common/status-tag";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { DirectDeliveryTrace } from "@/components/business/direct-delivery-trace";
import { DataTable } from "@/components/common/data-table";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/datetime";
import { statusText, statusTone } from "./sale-query-status";
export function SaleDetail({ detail, saleId, loading, error, onRetry, onClose, goTo, onEdit }: {
    detail: SaleDraft | null;
    saleId: number;
    loading: boolean;
    error: string;
    onRetry: () => void;
    onClose: () => void;
    goTo: (path: string) => void;
    onEdit: (row: SaleDraft) => void;
}) {
    const deliveryInfo = [
        { label: "送货联系人", value: detail?.deliveryContact },
        { label: "联系电话", value: detail?.deliveryPhone },
        { label: "送货地址", value: detail?.deliveryAddress },
    ].filter(field => Boolean(field.value?.trim()));
    const ownerInfo = detail && detail.status !== "DRAFT"
        ? [
            { label: "经营者", value: detail.ownerName },
            { label: "联系电话", value: detail.ownerPhone },
            { label: "联系地址", value: detail.ownerAddress },
        ].filter(field => Boolean(field.value?.trim()))
        : [];
    return (<DetailDialog open={saleId > 0} loading={loading} title={detail ? `销售单 · ${detail.documentNo}` : "销售单详情"} description={detail ? (<div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span>{detail.partnerName}</span>
        <span aria-hidden="true">·</span>
        <span>{detail.businessDate}</span>
        <StatusTag tone={statusTone(detail.status)}>{statusText(detail.status)}</StatusTag>
        <span>{detail.directDelivery ? "直送销售" : "普通销售"}</span>
      </div>) : undefined} onCancel={onClose} footer={detail && (<div className="flex flex-col gap-space-3 border-t border-border pt-space-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <div className="[&>div]:mb-0"><BusinessReturnLink /></div>
          <Button variant="secondary" onClick={() => goTo(`/business/partner-ledger?partnerId=${detail.partnerId}&direction=CUSTOMER`)}>
            查看客户往来
          </Button>
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          {detail.status === "DRAFT" && (<Button onClick={() => onEdit(detail)}>编辑草稿</Button>)}
          {detail.status !== "CANCELLED" && (<Button variant="secondary" onClick={() => goTo(`/business/sales/${detail.id}/delivery-note`)}>
              打印送货单
            </Button>)}
          {detail.status === "POSTED" && (<Button onClick={() => goTo(`/business/sale-returns?saleId=${detail.id}`)}>
              办理退货
            </Button>)}
        </div>
      </div>)}>
    {loading && <p role="status">正在读取销售来源单…</p>}
    {error && <div role="alert" className="text-error">{error} <Button variant="secondary" onClick={onRetry}>重试读取来源单</Button></div>}
    {detail && (<div className="space-y-space-5">
        {detail.directDelivery && (<p className="rounded-control bg-neutral-background p-space-3 text-sm text-text-secondary">
            直送销售：采购与销售单据分别过账。
          </p>)}

        {(deliveryInfo.length > 0 || ownerInfo.length > 0) && (<section className="grid gap-x-space-6 gap-y-space-4 sm:grid-cols-2" aria-label="送货及经营者资料">
            {deliveryInfo.length > 0 && (<div className="space-y-space-2">
                <h3 className="font-medium">送货资料</h3>
                {deliveryInfo.map(field => (<InfoField key={field.label} label={field.label} value={field.value}/>))}
              </div>)}
            {ownerInfo.length > 0 && (<div className="space-y-space-2">
                <h3 className="font-medium">经营者资料</h3>
                {ownerInfo.map(field => (<InfoField key={field.label} label={field.label} value={field.value}/>))}
              </div>)}
          </section>)}

        {detail.directDocuments.length > 0 && (<section className="space-y-space-3 border-t border-border pt-space-4" aria-label="关联采购单">
            <h3 className="font-medium">关联采购单</h3>
            {detail.directDocuments.map(document => (<div key={document.id} className="flex flex-wrap items-center justify-between gap-2">
                <Button size="sm" variant="secondary" onClick={() => goTo(`/business/purchases?purchaseId=${document.id}`)}>
                  {document.documentNo}
                </Button>
                <div className="flex flex-wrap items-center gap-2 text-sm text-text-secondary">
                  <StatusTag tone={statusTone(document.status)}>{statusText(document.status)}</StatusTag>
                  {document.postedAt && (<span>过账：{document.postedByName ?? "—"} · {formatDateTime(document.postedAt)}</span>)}
                  {document.cancelReason && (<span>取消：{document.cancelledByName ?? "—"} · {document.cancelReason}</span>)}
                </div>
              </div>))}
          </section>)}

        <section className="space-y-space-3" aria-label="销售商品明细">
          <h3 className="font-medium">商品明细</h3>
          <DataTable columns={[
                {
                    title: "商品 / 服务",
                    key: "product",
                    width: "40%",
                    render: (_, item) => {
                        const description = [item.productModel, item.productSpecification]
                            .filter(value => Boolean(value?.trim()))
                            .join(" · ");
                        return (<div className="min-w-[12rem] space-y-1">
                      <div className="font-medium text-text-primary">
                        {[item.productCode, item.productName].filter(Boolean).join(" · ")}
                      </div>
                      {description && <div className="text-sm text-text-secondary">{description}</div>}
                      <div className="flex flex-wrap items-center gap-x-2 text-xs text-text-tertiary">
                        <span>{item.productType === "SERVICE" ? "服务" : "实物"}</span>
                        {item.remark?.trim() && <span>备注：{item.remark.trim()}</span>}
                      </div>
                    </div>);
                    },
                },
                { title: "数量", dataIndex: "quantity", align: "right", nowrap: true, width: "15%" },
                { title: "单位", dataIndex: "unit", align: "center", nowrap: true, width: "10%" },
                {
                    title: "成交单价",
                    dataIndex: "unitPrice",
                    align: "right",
                    nowrap: true,
                    width: "17.5%",
                    render: value => `¥${value ?? ""}`,
                },
                {
                    title: "金额",
                    dataIndex: "amount",
                    align: "right",
                    nowrap: true,
                    width: "17.5%",
                    render: value => `¥${value ?? ""}`,
                },
            ]} dataSource={detail.items} rowKey="id" minWidth={760} empty="销售单尚无商品明细。"/>
          <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-space-3">
            <span className="text-sm text-text-tertiary">共 {detail.items.length} 项</span>
            <p className="text-right font-semibold">
              合计 <span className="ml-2 text-lg tabular-nums">¥{detail.totalAmount}</span>
            </p>
          </div>
        </section>

        {detail.remark?.trim() && (<section className="space-y-1 border-t border-border pt-space-4" aria-label="单据备注">
            <h3 className="font-medium">单据备注</h3>
            <p className="whitespace-pre-wrap break-words text-sm text-text-secondary">{detail.remark.trim()}</p>
          </section>)}

        <section className="grid gap-x-space-6 gap-y-space-2 border-t border-border pt-space-4 text-sm text-text-tertiary sm:grid-cols-2" aria-label="单据操作记录">
          <p>创建人：{detail.createdByName}</p>
          <p>创建时间：{formatDateTime(detail.createTime)}</p>
          {detail.postedByName && <p>过账人：{detail.postedByName}</p>}
          {detail.postedAt && <p>过账时间：{formatDateTime(detail.postedAt)}</p>}
          {(detail.cancelledByName || detail.cancelledAt) && (<p>取消人：{detail.cancelledByName ?? "—"}</p>)}
          {detail.cancelledAt && <p>取消时间：{formatDateTime(detail.cancelledAt)}</p>}
          {detail.cancelReason && (<p className="sm:col-span-2">取消原因：{detail.cancelReason}</p>)}
        </section>

        <DirectDeliveryTrace trace={detail.directTrace}/>
      </div>)}
  </DetailDialog>);
}
function InfoField({ label, value }: {
    label: string;
    value?: string | null;
}) {
    return (<p className="text-sm">
      <span className="mr-2 text-text-tertiary">{label}</span>
      <span className="break-words">{value}</span>
    </p>);
}
