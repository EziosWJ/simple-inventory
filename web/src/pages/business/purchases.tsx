import { useCallback, useEffect, useState } from "react";
import {
  cancelPurchase,
  createPurchase,
  getPurchase,
  partnerPage,
  postPurchase,
  productPage,
  purchasePage,
  updatePurchase,
  type PartnerRecord,
  type ProductRecord,
  type PurchaseDraft,
  type PurchaseInput,
  type PurchaseLineInput,
} from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { useNavigate, useSearchParams } from "react-router-dom";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { DataTableColumn } from "@/types";

type Filters = {
  documentNo: string;
  partnerId: string;
  productId: string;
  status: string;
  businessFrom: string;
  businessTo: string;
};
type LineForm = {
  productId: number;
  quantity: string;
  unitPrice: string;
  remark: string;
};
const blankFilters: Filters = {
  documentNo: "",
  partnerId: "",
  productId: "",
  status: "",
  businessFrom: "",
  businessTo: "",
};
const blankLine = (): LineForm => ({
  productId: 0,
  quantity: "1",
  unitPrice: "0.00",
  remark: "",
});
const PAGE_SIZE = 10;

export function PurchasesPage() {
  const navigate=useNavigate();
  const [searchParams] = useSearchParams();
  const [records, setRecords] = useState<PurchaseDraft[]>([]);
  const [partners, setPartners] = useState<PartnerRecord[]>([]);
  const [products, setProducts] = useState<ProductRecord[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [filters, setFilters] = useState<Filters>(blankFilters);
  const [query, setQuery] = useState<Filters>(blankFilters);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [formOpen, setFormOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [editing, setEditing] = useState<PurchaseDraft | null>(null);
  const [partnerId, setPartnerId] = useState(0);
  const [businessDate, setBusinessDate] = useState(today());
  const [directDelivery, setDirectDelivery] = useState(false);
  const [remark, setRemark] = useState("");
  const [lines, setLines] = useState<LineForm[]>([blankLine()]);
  const [detail, setDetail] = useState<PurchaseDraft | null>(null);
  const [cancelTarget, setCancelTarget] = useState<PurchaseDraft | null>(null);
  const [cancelReason, setCancelReason] = useState("");
  const [postTarget, setPostTarget] = useState<PurchaseDraft | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    void purchasePage({
      page,
      pageSize: PAGE_SIZE,
      documentNo: query.documentNo,
      partnerId: query.partnerId,
      productId: query.productId,
      status: query.status,
      businessFrom: query.businessFrom,
      businessTo: query.businessTo,
    })
      .then((result) => {
        setRecords(result.records);
        setTotal(result.total);
        setError("");
      })
      .catch((reason: unknown) => {
        setRecords([]);
        setTotal(0);
        setError(reason instanceof Error ? reason.message : "采购单加载失败");
      })
      .finally(() => setLoading(false));
  }, [page, query]);

  useEffect(() => {
    void partnerPage({ page: 1, pageSize: 500, identity: "SUPPLIER", status: 1 })
      .then((result) => setPartners(result.records))
      .catch(() => setPartners([]));
    void productPage({ page: 1, pageSize: 500, type: "GOODS", status: 1 })
      .then((result) =>
        setProducts(result.records.filter((product) => product.type === "GOODS" && product.status === 1)),
      )
      .catch(() => setProducts([]));
  }, []);

  useEffect(() => load(), [load, reload]);
  useEffect(() => { const id=Number(searchParams.get("purchaseId")); if(id>0) void getPurchase(id).then(setDetail).catch((reason:unknown)=>setError(reason instanceof Error?reason.message:"读取采购单失败")); }, [searchParams]);

  function startNew() {
    setEditing(null);
    setDirectDelivery(false);
    setPartnerId(0);
    setBusinessDate(today());
    setRemark("");
    setLines([blankLine()]);
    setError("");
    setFormOpen(true);
  }

  async function startEdit(record: PurchaseDraft) {
    try {
      const current = await getPurchase(record.id);
      setEditing(current);
      setDirectDelivery(current.directDelivery);
      setPartnerId(current.partnerId);
      setBusinessDate(current.businessDate);
      setRemark(current.remark ?? "");
      setLines(
        current.items.map((item) => ({
          productId: item.productId,
          quantity: item.quantity,
          unitPrice: item.unitPrice,
          remark: item.remark ?? "",
        })),
      );
      setError("");
      setFormOpen(true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "读取采购单失败");
    }
  }

  async function openDetail(record: PurchaseDraft) {
    try {
      setDetail(await getPurchase(record.id));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "读取采购单失败");
    }
  }

  async function saveDraft() {
    const body: PurchaseInput = {
      directDelivery,
      partnerId,
      businessDate,
      remark: remark.trim() || undefined,
      items: lines.map((line) => {
        const product = products.find((item) => item.id === line.productId);
        return {
          productId: line.productId,
          productType: "GOODS",
          unit: product?.unit ?? "",
          quantity: line.quantity,
          unitPrice: line.unitPrice,
          remark: line.remark.trim() || undefined,
        } satisfies PurchaseLineInput;
      }),
    };
    setSaving(true);
    try {
      if (editing) {
        await updatePurchase(editing.id, { ...body, version: editing.version });
      } else {
        await createPurchase(body);
      }
      setFormOpen(false);
      setReload((value) => value + 1);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "保存失败，表单输入已保留");
    } finally {
      setSaving(false);
    }
  }

  async function confirmCancel() {
    if (!cancelTarget) return;
    setSaving(true);
    try {
      await cancelPurchase(cancelTarget.id, {
        version: cancelTarget.version,
        reason: cancelReason,
      });
      setCancelTarget(null);
      setReload((value) => value + 1);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "取消失败，采购草稿仍保留");
    } finally {
      setSaving(false);
    }
  }
  async function confirmPost(){if(!postTarget)return;setSaving(true);try{await postPurchase(postTarget.id,postTarget.version);setPostTarget(null);setDetail(null);setReload(v=>v+1);}catch(reason){setError(reason instanceof Error?reason.message:"过账失败，单据仍保留");}finally{setSaving(false);}}

  function applyFilters() {
    if (filters.businessFrom && filters.businessTo && filters.businessFrom > filters.businessTo) {
      setError("业务日期起始不能晚于结束日期。");
      return;
    }
    setPage(1);
    setQuery({ ...filters });
  }

  function resetFilters() {
    setFilters(blankFilters);
    setQuery(blankFilters);
    setPage(1);
  }

  const columns: DataTableColumn<PurchaseDraft>[] = [
    {
      title: "采购单号",
      dataIndex: "documentNo",
      nowrap: true,
      render: (value, record) => (
        <button className="text-primary hover:underline" onClick={() => void openDetail(record)}>
          {String(value ?? "")}
        </button>
      ),
    },
    { title: "供应商", dataIndex: "partnerName" },
    { title: "业务日期", dataIndex: "businessDate", nowrap: true },
    { title: "金额", dataIndex: "totalAmount", align: "right", render: (value) => `¥${String(value ?? "0.00")}` },
    { title: "状态", dataIndex: "status", render: (value) => ({DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[String(value)] ?? String(value)) },
    { title: "创建人", dataIndex: "createdByName" },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      render: (_, record) => (
        <div className="flex gap-2">
          <Button size="sm" variant="secondary" onClick={() => void openDetail(record)}>查看</Button>
          {record.status === "DRAFT" && (
            <>
              <Button size="sm" variant="secondary" onClick={() => void startEdit(record)}>编辑</Button>
              <Button size="sm" onClick={() => setPostTarget(record)}>过账</Button>
              <Button size="sm" variant="secondary" onClick={() => { setCancelTarget(record); setCancelReason(""); }}>取消</Button>
            </>
          )}
          {record.status === "POSTED" && <><Button size="sm" variant="secondary" onClick={() => { setCancelTarget(record); setCancelReason(""); }}>整单取消</Button><Button size="sm" onClick={() => navigate(`/business/purchase-returns?purchaseId=${record.id}`)}>办理退货</Button></>}
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-space-4">
      <PageHeader
        title="采购入库"
        description="记录实际收货；草稿保存和取消不改变库存或应付款。"
        actions={<Button onClick={startNew}>新建采购单</Button>}
      />
      <SearchFilterBar actions={(
        <>
          <Button onClick={applyFilters}>查询</Button>
          <Button variant="secondary" onClick={resetFilters}>重置</Button>
        </>
      )}>
        <Field label="采购单号">
          <Input value={filters.documentNo} onChange={(event) => setFilters({ ...filters, documentNo: event.target.value })} placeholder="输入单号" />
        </Field>
        <Field label="供应商">
          <select className="h-control w-full rounded-control border border-border bg-surface px-space-3" value={filters.partnerId} onChange={(event) => setFilters({ ...filters, partnerId: event.target.value })}>
            <option value="">全部供应商</option>
            {partners.map((partner) => <option key={partner.id} value={partner.id}>{partner.name}</option>)}
          </select>
        </Field>
        <Field label="状态">
          <select className="h-control w-full rounded-control border border-border bg-surface px-space-3" value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}>
            <option value="">全部状态</option><option value="DRAFT">草稿</option><option value="POSTED">已过账</option><option value="CANCELLED">已取消</option>
          </select>
        </Field>
        <Field label="商品">
          <select className="h-control w-full rounded-control border border-border bg-surface px-space-3" value={filters.productId} onChange={(event) => setFilters({ ...filters, productId: event.target.value })}>
            <option value="">全部商品</option>
            {products.map((product) => <option key={product.id} value={product.id}>{product.code} · {product.name}</option>)}
          </select>
        </Field>
        <Field label="业务日期起">
          <Input type="date" value={filters.businessFrom} onChange={(event) => setFilters({ ...filters, businessFrom: event.target.value })} />
        </Field>
        <Field label="业务日期止">
          <Input type="date" value={filters.businessTo} onChange={(event) => setFilters({ ...filters, businessTo: event.target.value })} />
        </Field>
      </SearchFilterBar>
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
      <DataTableCard
        toolbar={<TableToolbar title="采购入库单" description="支持按单号、供应商、状态、商品与业务日期查询。" />}
        pagination={<Pagination page={page} pageSize={PAGE_SIZE} total={total} disabled={loading} onPageChange={setPage} />}
      >
        <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} error={error || undefined} minWidth={960} empty="暂无采购单。" />
      </DataTableCard>

      <FormDialog
        open={formOpen}
        title={editing ? "编辑采购入库草稿" : "新建采购入库草稿"}
        description={editing ? `${editing.documentNo} · 版本 ${editing.version}` : "单据保存后生成唯一单号。"}
        loading={saving}
        onCancel={() => setFormOpen(false)}
        onSubmit={saveDraft}
      >
        {error && <p role="alert" className="mb-3 text-sm text-error">{error}</p>}
        <div className="grid gap-space-3 md:grid-cols-2">
          <Field label="供应商" required>
            <select className="h-control w-full rounded-control border border-border bg-surface px-space-3" value={partnerId || ""} onChange={(event) => setPartnerId(Number(event.target.value))}>
              <option value="">选择启用供应商</option>
              {partners.map((partner) => <option key={partner.id} value={partner.id}>{partner.name}</option>)}
            </select>
          </Field>
          <Field label="业务日期" required>
            <Input type="date" value={businessDate} onChange={(event) => setBusinessDate(event.target.value)} />
          </Field>
          <Field label="交付方式"><label className="flex items-center gap-2"><input type="checkbox" checked={directDelivery} onChange={e=>setDirectDelivery(e.target.checked)}/>供应商直接送达客户</label></Field>
          <Field label="备注">
            <Input value={remark} onChange={(event) => setRemark(event.target.value)} maxLength={500} />
          </Field>
        </div>
        <div className="mt-space-4 space-y-space-3">
          <div className="flex items-center justify-between">
            <h3 className="font-medium">采购明细</h3>
            <Button size="sm" variant="secondary" onClick={() => setLines([...lines, blankLine()])}>添加明细</Button>
          </div>
          {lines.map((line, index) => {
            const selected = products.find((product) => product.id === line.productId);
            return (
              <div key={index} className="grid gap-space-2 rounded-control border border-border p-space-3 md:grid-cols-[2fr_1fr_1fr_auto]">
                <Field label={`第 ${index + 1} 行商品`} required>
                  <select className="h-control w-full rounded-control border border-border bg-surface px-space-3" value={line.productId || ""} onChange={(event) => {
                    const productId = Number(event.target.value);
                    const product = products.find((item) => item.id === productId);
                    updateLine(setLines, lines, index, {
                      productId,
                      unitPrice: product?.purchasePrice ?? "0.00",
                    });
                  }}>
                    <option value="">选择启用实物商品</option>
                    {products.map((product) => <option key={product.id} value={product.id}>{product.code} · {product.name}（{product.unit}）</option>)}
                  </select>
                  {selected && <span className="text-helper text-text-tertiary">基本单位：{selected.unit}</span>}
                </Field>
                <Field label="数量" required>
                  <Input inputMode="decimal" value={line.quantity} onChange={(event) => updateLine(setLines, lines, index, { quantity: event.target.value })} />
                </Field>
                <Field label="成交单价（元）" required>
                  <Input inputMode="decimal" value={line.unitPrice} onChange={(event) => updateLine(setLines, lines, index, { unitPrice: event.target.value })} />
                </Field>
                <div className="flex items-end pb-5">
                  <Button variant="secondary" disabled={lines.length === 1} onClick={() => setLines(lines.filter((_, lineIndex) => lineIndex !== index))}>删除</Button>
                </div>
              </div>
            );
          })}
        </div>
      </FormDialog>

      <DetailDialog
        open={detail !== null}
        title={`采购入库单${detail ? ` · ${detail.documentNo}` : "详情"}`}
        description={detail ? `${detail.partnerName} · ${detail.businessDate} · ${{DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[detail.status]}` : undefined}
        onCancel={() => setDetail(null)}
      >
        {detail && (
          <>
            <p className="mb-3">{detail.directDelivery ? "直送：先单独过账采购，再单独过账销售；取消时先取消关联销售。" : "普通备货采购"}</p>
            {detail.directDelivery && <div className="mb-3 space-y-2">{detail.directDocuments.map(d=><p key={d.id}><Button variant="secondary" onClick={()=>navigate(`/business/sales?saleId=${d.id}`)}>{d.documentNo} · {statusText(d.status)}</Button> {d.postedAt&&`过账：${d.postedByName} ${new Date(d.postedAt).toLocaleString()}`} {d.cancelReason&&`取消：${d.cancelledByName} ${d.cancelReason}`}</p>)}{detail.status!=="CANCELLED" && !detail.directDocuments.some(d=>d.status!=="CANCELLED") && <Button onClick={()=>navigate(`/business/sales?directPurchaseId=${detail.id}`)}>关联新建直送销售</Button>}</div>}
            <DataTable
              columns={[
                { title: "商品", key: "product", render: (_, item) => `${item.productCode} ${item.productName}` },
                { title: "数量", dataIndex: "quantity" },
                { title: "单位", dataIndex: "unit" },
                { title: "成交单价", dataIndex: "unitPrice", align: "right" },
                { title: "金额", dataIndex: "amount", align: "right" },
              ]}
              dataSource={detail.items}
              rowKey="id"
              minWidth={720}
            />
            <p className="mt-space-4 text-right font-medium">合计：¥{detail.totalAmount}</p>
            <p className="mt-space-2 text-sm text-text-tertiary">
              创建人：{detail.createdByName} · 创建时间：{new Date(detail.createTime).toLocaleString()}
              {detail.postedByName && ` · 过账人：${detail.postedByName}`}
              {detail.cancelledByName && ` · 取消人：${detail.cancelledByName}`}
            </p>
            {detail.cancelReason && <p className="mt-space-2 text-sm">取消原因：{detail.cancelReason}</p>}
            <Button className="mt-space-3" variant="secondary" onClick={() => navigate(`/business/partner-balances?partnerId=${detail.partnerId}&direction=SUPPLIER`)}>查看供应商往来</Button>
          </>
        )}
      </DetailDialog>

      <FormDialog
        open={cancelTarget !== null}
        title={cancelTarget?.status === "POSTED" ? "整单取消采购入库" : "取消采购草稿"}
        description={cancelTarget ? `单号 ${cancelTarget.documentNo}。已付款保留；若库存不足，整单取消将被拒绝。` : undefined}
        submitText="确认取消"
        loading={saving}
        submitDisabled={!cancelReason.trim()}
        onCancel={() => setCancelTarget(null)}
        onSubmit={confirmCancel}
      >
        <Field label="取消原因" required>
          <Textarea value={cancelReason} onChange={(event) => setCancelReason(event.target.value)} maxLength={500} placeholder="请说明取消原因" />
        </Field>
      </FormDialog>
      <FormDialog open={postTarget !== null} title="过账采购入库单" description={postTarget ? `${postTarget.documentNo} · ${postTarget.partnerName} · 合计 ¥${postTarget.totalAmount}。确认后库存与应付同时生效。` : undefined} submitText="确认过账" loading={saving} onCancel={() => setPostTarget(null)} onSubmit={confirmPost}>
        {postTarget && <p className="text-sm">明细 {postTarget.items.length} 行 · 当前版本 {postTarget.version}</p>}
      </FormDialog>
    </div>
  );
}

function updateLine(
  setLines: (lines: LineForm[]) => void,
  lines: LineForm[],
  index: number,
  patch: Partial<LineForm>,
) {
  setLines(lines.map((line, lineIndex) => lineIndex === index ? { ...line, ...patch } : line));
}

function today() {
  const date = new Date();
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 10);
}

function statusText(s:string){return {DRAFT:"草稿",POSTED:"已过账",CANCELLED:"已取消"}[s]??s}
