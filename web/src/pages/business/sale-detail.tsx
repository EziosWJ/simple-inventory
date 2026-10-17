import { ChevronDown, Copy } from "lucide-react";
import { useState, type ReactNode } from "react";
import type { SaleDraft } from "@/api/business";
import { SalePurchaseSources } from "@/components/business/sale-purchase-sources";
import { sourceQuantityMilli } from "@/components/business/purchase-source-validation";
import { DetailDialog } from "@/components/common/detail-dialog";
import { StatusTag } from "@/components/common/status-tag";
import { BusinessReturnLink } from "@/components/business/business-return-link";
import { DirectDeliveryTrace } from "@/components/business/direct-delivery-trace";
import { DataTable } from "@/components/common/data-table";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/datetime";
import { copyText } from "@/lib/clipboard";
import { statusText, statusTone } from "./sale-query-status";

export function SaleDetail({ detail, saleId, loading, error, onRetry, onClose, goTo, onEdit, onChange }: {
  detail: SaleDraft | null;
  saleId: number;
  loading: boolean;
  error: string;
  onRetry: () => void;
  onClose: () => void;
  goTo: (path: string) => void;
  onEdit: (row: SaleDraft) => void;
  onChange?: (row: SaleDraft) => void;
}) {
  const deliveryInfo = [
    { label: "送货联系人", value: detail?.deliveryContact },
    { label: "联系电话", value: detail?.deliveryPhone },
    { label: "送货地址", value: detail?.deliveryAddress },
  ].filter(field => Boolean(field.value?.trim()));
  const ownerInfo = detail && detail.status !== "DRAFT" ? [
    { label: "经营者", value: detail.ownerName },
    { label: "联系电话", value: detail.ownerPhone },
    { label: "联系地址", value: detail.ownerAddress },
  ].filter(field => Boolean(field.value?.trim())) : [];
  const goods = detail?.items.filter(item => item.productType === "GOODS") ?? [];
  const services = (detail?.items.length ?? 0) - goods.length;
  const sourcedCount = goods.filter(item => {
    const quantity = sourceQuantityMilli(item.quantity);
    const sourced = (item.purchaseSources ?? []).reduce((sum, source) => sum + (sourceQuantityMilli(source.quantity) ?? 0n), 0n);
    return quantity !== null && sourced === quantity;
  }).length;
  const sourceWarnings = goods.filter(item => item.purchaseSources?.some(source =>
    source.status === "CANCELLED" || (sourceQuantityMilli(source.returnedQuantity ?? "0") ?? 0n) > 0n));
  const zeroAmount = /^0(?:\.0+)?$/.test(detail?.totalAmount ?? "");
  const effect = !detail ? "" : detail.status === "DRAFT"
    ? "尚未生效，库存与客户往来余额未变化。"
    : detail.status === "CANCELLED"
      ? detail.postedAt ? "已整单冲销，原过账记录保留。" : "草稿已取消，库存与客户往来余额未变化。"
      : `${goods.length ? "实物已出库" : "仅含服务，不产生库存变动"}；${zeroAmount ? "零金额，客户往来余额未变化。" : "销售金额已计入客户往来。"}`;

  return (
    <DetailDialog
      open={saleId > 0}
      loading={loading}
      title={detail ? `销售单 · ${detail.documentNo}` : "销售单详情"}
      contentClassName="max-w-[1100px] max-h-[90dvh] max-sm:h-dvh max-sm:max-h-dvh max-sm:w-full max-sm:rounded-none"
      headerClassName="py-space-5"
      bodyClassName="space-y-space-6 sm:px-space-6"
      fixedFooter
      description={detail ? (
        <div className="mt-space-2 flex flex-wrap items-center gap-x-space-6 gap-y-space-2 text-sm text-text-secondary">
          <span className="break-words"><span className="mr-2 text-text-tertiary">客户</span>{detail.partnerName}</span>
          <span><span className="mr-2 text-text-tertiary">业务日期</span>{detail.businessDate}</span>
          <StatusTag tone={statusTone(detail.status)}>{statusText(detail.status)}</StatusTag>
          <span>{detail.sourceOrderId ? "直送单生成" : detail.directDelivery ? "旧版直送销售" : "普通销售"}</span>
          <CopyDocumentNo key={detail.documentNo} documentNo={detail.documentNo} />
        </div>
      ) : undefined}
      onCancel={onClose}
      footer={detail && (
        <div className="flex flex-wrap items-center justify-between gap-space-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="[&>div]:mb-0"><BusinessReturnLink /></div>
            <Button variant="secondary" onClick={() => goTo(`/business/partner-ledger?partnerId=${detail.partnerId}&direction=CUSTOMER`)}>查看客户往来</Button>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {detail.sourceOrderId && <Button onClick={() => goTo(`/business/direct-orders/${detail.sourceOrderId}`)}>查看来源直送单</Button>}
            {detail.status === "DRAFT" && !detail.sourceOrderId && <Button variant="primary" onClick={() => onEdit(detail)}>编辑草稿</Button>}
            {detail.status === "POSTED" && goods.length > 0 && <Button variant="secondary" onClick={() => goTo(`/business/sale-returns?saleId=${detail.id}`)}>办理退货</Button>}
            {detail.status !== "CANCELLED" && <Button variant={detail.status === "POSTED" ? "primary" : "secondary"} onClick={() => goTo(`/business/sales/${detail.id}/delivery-note`)}>打印送货单</Button>}
          </div>
        </div>
      )}
    >
      {loading && <p role="status">正在读取销售单…</p>}
      {error && <div role="alert" className="text-error">{error} <Button variant="secondary" onClick={onRetry}>重试读取来源单</Button></div>}
      {detail && <div key={detail.id} className="space-y-space-6">
        <section aria-label="销售单概览" className="border-b border-border pb-space-5">
          <div className="flex flex-wrap items-end justify-between gap-space-4">
            <div>
              <p className="text-sm text-text-secondary">单据金额{detail.status === "CANCELLED" && "（原单）"}</p>
              <p className="mt-space-1 text-page-title font-semibold tabular-nums">¥{detail.totalAmount}</p>
            </div>
            <p className="text-sm text-text-secondary">共 {detail.items.length} 项<span className="ml-space-3">实物 {goods.length} 项 · 服务 {services} 项</span></p>
          </div>
          <div className="mt-space-4 rounded-control bg-neutral-background px-space-4 py-space-3 text-sm text-text-secondary">
            <p>{effect}</p>
            {detail.directDelivery && <p className="mt-space-1">{detail.sourceOrderId ? "由直送单统一确认和取消，采购与销售同时生效。" : "旧版直送销售：采购与销售单据分别过账。"}</p>}
            {detail.cancelReason && <p className="mt-space-2 whitespace-pre-wrap break-words text-text-primary">取消原因：{detail.cancelReason}</p>}
          </div>
        </section>

        <section aria-label="销售商品明细" className="space-y-space-3">
          <h3 className="text-base font-semibold">商品明细</h3>
          <DataTable columns={[
            { title: "商品 / 服务", key: "product", width: "48%", render: (_, item) => (
              <div className="min-w-[12rem] space-y-space-1 py-space-1">
                <div className="flex flex-wrap items-center gap-space-2">
                  <span className="font-medium text-text-primary">{item.productName}</span>
                  {item.productType === "SERVICE" && <StatusTag tone="info">服务</StatusTag>}
                </div>
                <div className="break-words text-xs text-text-secondary">{[item.productCode, item.productModel, item.productSpecification].filter(value => Boolean(value?.trim())).join(" · ")}</div>
                {item.remark?.trim() && <p className="whitespace-pre-wrap break-words text-sm text-text-secondary">备注：{item.remark.trim()}</p>}
              </div>
            ) },
            { title: "数量", key: "quantity", align: "right", nowrap: true, width: "16%", render: (_, item) => <span className="tabular-nums">{item.quantity} <span className="text-text-secondary">{item.unit}</span></span> },
            { title: "成交单价", dataIndex: "unitPrice", align: "right", nowrap: true, width: "18%", render: value => `¥${value}` },
            { title: "金额", dataIndex: "amount", align: "right", nowrap: true, width: "18%", render: value => <span className="font-medium tabular-nums">¥{String(value ?? "")}</span> },
          ]} dataSource={detail.items} rowKey="id" minWidth={760} empty="销售单尚无商品明细。" />
          <div className="flex justify-end gap-space-4 pt-space-2 text-sm">
            <span className="text-text-secondary">单据合计</span><span className="font-semibold tabular-nums">¥{detail.totalAmount}</span>
          </div>
        </section>

        {detail.remark?.trim() && <section aria-label="单据备注" className="border-l-2 border-border pl-space-4">
          <h3 className="font-medium">单据备注</h3>
          <p className="mt-space-2 whitespace-pre-wrap break-words text-sm text-text-secondary">{detail.remark.trim()}</p>
        </section>}

        <div className="divide-y divide-border border-t border-border">
          {(deliveryInfo.length > 0 || ownerInfo.length > 0) && <DetailDisclosure title="送货及经营者资料" hint={detail.deliveryContact ? `联系人：${detail.deliveryContact}` : "查看联系人、地址和打印抬头"}>
            <section aria-label="送货及经营者资料" className="grid gap-space-6 sm:grid-cols-2">
              {deliveryInfo.length > 0 && <div><h3 className="mb-space-3 font-medium">送货资料</h3><dl className="space-y-space-3">{deliveryInfo.map(field => <InfoField key={field.label} {...field} />)}</dl></div>}
              {ownerInfo.length > 0 && <div><h3 className="mb-space-3 font-medium">经营者资料</h3><dl className="space-y-space-3">{ownerInfo.map(field => <InfoField key={field.label} {...field} />)}</dl></div>}
            </section>
          </DetailDisclosure>}

          {!detail.directDelivery && goods.length > 0 && <div>
            {sourceWarnings.length > 0 && <p role="status" className="pt-space-4 text-sm text-warning">{sourceWarnings.map(item => item.productName).join("、")} 的来源采购已取消或发生退货，请核对采购来源。</p>}
            <DetailDisclosure title="销售采购来源" hint={`已完整记录 ${sourcedCount} / ${goods.length} 项实物来源`}>
              <div className="[&>section]:mt-0 [&>section]:border-t-0 [&>section]:pt-0"><SalePurchaseSources document={detail} onChange={onChange ?? (() => onRetry())} /></div>
            </DetailDisclosure>
          </div>}

          {detail.directDocuments.length > 0 && <DetailDisclosure title="关联单据与直送追溯" hint={`${detail.directDocuments.length} 张关联采购单`}>
            {detail.directDocuments.length > 0 && <section aria-label="关联采购单" className="space-y-space-3">
              {detail.directDocuments.map(document => <div key={document.id} className="flex flex-wrap items-center justify-between gap-space-3">
                <Button size="sm" variant="secondary" onClick={() => goTo(`/business/purchases?purchaseId=${document.id}`)}>{document.documentNo}</Button>
                <div className="flex flex-wrap items-center gap-space-2 text-sm text-text-secondary">
                  <StatusTag tone={statusTone(document.status)}>{statusText(document.status)}</StatusTag>
                  {document.postedAt && <span>过账：{document.postedByName ?? "—"} · {formatDateTime(document.postedAt)}</span>}
                  {document.cancelReason && <span>取消原因：{document.cancelReason}</span>}
                </div>
              </div>)}
            </section>}
          </DetailDisclosure>}
          <DirectDeliveryTrace trace={detail.directTrace} />

          <DetailDisclosure title="操作记录" hint={detail.postedAt ? `过账于 ${formatDateTime(detail.postedAt)}` : `创建于 ${formatDateTime(detail.createTime)}`}>
            <section aria-label="单据操作记录">
              <ol className="ml-space-2 border-l border-border">
                <OperationRecord title="创建单据" operator={detail.createdByName} time={detail.createTime} />
                {detail.postedAt && <OperationRecord title="过账生效" operator={detail.postedByName} time={detail.postedAt} />}
                {detail.status === "CANCELLED" && <OperationRecord title={detail.postedAt ? "整单冲销取消" : "取消草稿"} operator={detail.cancelledByName} time={detail.cancelledAt} />}
              </ol>
            </section>
          </DetailDisclosure>
        </div>
      </div>}
    </DetailDialog>
  );
}

function DetailDisclosure({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
  return <details className="group py-space-4">
    <summary className="flex cursor-pointer list-none flex-wrap items-center gap-space-2 rounded-tight text-sm [&::-webkit-details-marker]:hidden">
      <ChevronDown className="h-4 w-4 shrink-0 text-text-tertiary group-open:rotate-180" aria-hidden />
      <span className="font-medium">{title}</span>
      {hint && <span className="ml-space-2 text-text-tertiary">{hint}</span>}
    </summary>
    <div className="pt-space-4">{children}</div>
  </details>;
}

function InfoField({ label, value }: { label: string; value?: string | null }) {
  return <div className="grid grid-cols-[6rem_minmax(0,1fr)] gap-space-3 text-sm">
    <dt className="text-text-tertiary">{label}</dt><dd className="whitespace-pre-wrap break-words">{value}</dd>
  </div>;
}

function OperationRecord({ title, operator, time }: { title: string; operator?: string; time?: string | null }) {
  return <li className="relative pb-space-4 pl-space-5 last:pb-0">
    <span className="absolute -left-1 top-2 h-2 w-2 rounded-full bg-border" aria-hidden />
    <p className="text-sm font-medium">{title}</p>
    <p className="mt-space-1 flex flex-wrap gap-x-space-4 text-sm text-text-secondary"><span>{operator || "—"}</span><span>{time ? formatDateTime(time) : "—"}</span></p>
  </li>;
}

function CopyDocumentNo({ documentNo }: { documentNo: string }) {
  const [message, setMessage] = useState("");
  return <span className="inline-flex items-center gap-space-2">
    <Button size="sm" variant="ghost" aria-label="复制销售单号" onClick={async () => {
      setMessage(await copyText(documentNo) ? "已复制" : "复制失败，请手动复制单号");
    }}><Copy className="h-3.5 w-3.5" aria-hidden />复制单号</Button>
    {message && <span role="status" className="text-xs">{message}</span>}
  </span>;
}
