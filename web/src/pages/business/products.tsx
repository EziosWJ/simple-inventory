import { useEffect, useState } from "react";
import {
  productPage,
  productStatus,
  saveProduct,
  type ProductRecord,
} from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  BUSINESS_STATUS_VALUES,
  DICT_CODES,
  PRODUCT_TYPE_VALUES,
  type DictSelectOption,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import {
  businessDictLabel,
  missingBusinessDictValues,
  preserveCurrentDictOption,
} from "@/lib/business-dict-label";
import type { ApiStatus, DataTableColumn } from "@/types";

type Filters = { keyword: string; type: string; category: string; status: string };
type ProductDraft = Omit<ProductRecord, "id" | "status">;
const emptyFilters: Filters = { keyword: "", type: "", category: "", status: "" };

export function ProductsPage() {
  const productTypeDict = useDictOptions<ProductRecord["type"]>(DICT_CODES.PRODUCT_TYPE, {
    allowedValues: PRODUCT_TYPE_VALUES,
  });
  const businessStatusDict = useDictOptions<ApiStatus>(DICT_CODES.BUSINESS_STATUS, {
    allowedValues: BUSINESS_STATUS_VALUES,
    valueType: "number",
  });
  const [records, setRecords] = useState<ProductRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [draft, setDraft] = useState<Filters>(emptyFilters);
  const [filters, setFilters] = useState<Filters>(emptyFilters);
  const [queryRun, setQueryRun] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [editing, setEditing] = useState<ProductRecord | null>(null);
  const [detail, setDetail] = useState<ProductRecord | null>(null);
  const missingTypes = missingBusinessDictValues(productTypeDict.options, PRODUCT_TYPE_VALUES);
  const missingStatuses = missingBusinessDictValues(businessStatusDict.options, BUSINESS_STATUS_VALUES);
  const productTypeIssue = productTypeDict.error || (missingTypes.length ? `字典缺少可用值：${missingTypes.join("、")}` : "");
  const businessStatusIssue = businessStatusDict.error || (missingStatuses.length ? `字典缺少可用值：${missingStatuses.join("、")}` : "");

  useEffect(() => {
    let current = true;
    setLoading(true);
    void productPage({ page, pageSize: 10, ...filters })
      .then((result) => {
        if (!current) return;
        setRecords(result.records);
        setTotal(result.total);
        setError("");
      })
      .catch((reason: unknown) => {
        if (!current) return;
        setRecords([]);
        setTotal(0);
        setError(reason instanceof Error ? reason.message : "加载失败");
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [filters, page, queryRun]);

  const columns: DataTableColumn<ProductRecord>[] = [
    {
      title: "商品/服务",
      key: "name",
      render: (_, record) => (
        <div>
          <div className="font-medium">{record.name}</div>
          <div className="text-xs text-text-tertiary">{record.code}</div>
        </div>
      ),
    },
    {
      title: "型号 / 规格",
      key: "specification",
      render: (_, record) => `${record.model || "-"} / ${record.specification || "-"}`,
    },
    { title: "类型", dataIndex: "type", render: (value) => businessDictLabel(productTypeDict.options, value) },
    { title: "分类", dataIndex: "category", render: (value) => value || "-" },
    { title: "单位", dataIndex: "unit" },
    { title: "参考采购价", dataIndex: "purchasePrice", render: (value) => value ?? "-" },
    { title: "参考销售价", dataIndex: "salePrice", render: (value) => value ?? "-" },
    { title: "状态", dataIndex: "status", render: (value) => businessDictLabel(businessStatusDict.options, value) },
    {
      title: "操作",
      key: "actions",
      render: (_, record) => (
        <div className="flex gap-1">
          <Button size="sm" variant="secondary" onClick={() => setDetail(record)}>查看</Button>
          <Button size="sm" variant="secondary" onClick={() => setEditing(record)}>编辑</Button>
          <Button size="sm" variant="secondary" onClick={() => void changeStatus(record)}>
            {businessDictLabel(businessStatusDict.options, record.status === 1 ? 0 : 1)}
          </Button>
        </div>
      ),
    },
  ];

  async function changeStatus(record: ProductRecord) {
    setActionError("");
    try {
      await productStatus(record.id, record.status === 1 ? 0 : 1);
      setQueryRun((value) => value + 1);
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : "状态更新失败");
    }
  }

  function query() {
    setFilters({ ...draft });
    setPage(1);
    setQueryRun((value) => value + 1);
  }

  function reset() {
    setDraft(emptyFilters);
    setFilters(emptyFilters);
    setPage(1);
    setQueryRun((value) => value + 1);
  }

  async function save(draftValue: ProductDraft, id?: number) {
    await saveProduct(draftValue, id);
    setEditing(null);
    setQueryRun((value) => value + 1);
  }

  return (
    <>
      <PageHeader
        title="商品与服务"
        description="维护实物商品和不产生库存的服务项目。"
        actions={<Button onClick={() => setEditing(newProduct())}>新建档案</Button>}
      />
      <SearchFilterBar actions={
        <>
          <Button variant="secondary" onClick={reset}>重置</Button>
          <Button onClick={query}>查询</Button>
        </>
      }>
        <Input
          placeholder="编码、名称、品牌、型号或规格"
          value={draft.keyword}
          onChange={(event) => { setDraft({ ...draft, keyword: event.target.value }); setPage(1); }}
        />
        <Select value={draft.type} onChange={(event) => { setDraft({ ...draft, type: event.target.value }); setPage(1); }}>
          <option value="">全部类型</option>{productTypeDict.options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select>
        <Input placeholder="分类" value={draft.category} onChange={(event) => { setDraft({ ...draft, category: event.target.value }); setPage(1); }} />
        <Select value={draft.status} onChange={(event) => { setDraft({ ...draft, status: event.target.value }); setPage(1); }}>
          <option value="">全部状态</option>{businessStatusDict.options.map((option) => <option key={option.value} value={String(option.value)}>{option.label}</option>)}
        </Select>
      </SearchFilterBar>
      {(productTypeIssue || businessStatusIssue) && <div role="alert" className="mb-3 flex items-center justify-between gap-3 rounded-admin border border-danger/30 bg-danger/5 p-3 text-sm text-danger">
        <span>{[productTypeIssue && `${DICT_CODES.PRODUCT_TYPE}：${productTypeIssue}`, businessStatusIssue && `${DICT_CODES.BUSINESS_STATUS}：${businessStatusIssue}`].filter(Boolean).join("；")}</span>
        <Button size="sm" variant="secondary" onClick={() => { productTypeDict.reload(); businessStatusDict.reload(); }}>重试</Button>
      </div>}
      {actionError && <p role="alert" className="mb-3 text-sm text-danger">{actionError}</p>}
      <DataTableCard pagination={
        <Pagination page={page} pageSize={10} total={total} onPageChange={setPage} />
      }>
        <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} error={error} />
      </DataTableCard>
      {detail && (
        <dialog open className="fixed inset-0 z-50 m-auto max-h-[90vh] w-[min(640px,95vw)] overflow-auto rounded-admin border border-border bg-surface p-card shadow-admin">
          <h2 className="mb-4 text-lg font-semibold">档案详情</h2>
          <dl className="grid gap-3 sm:grid-cols-2">
            {productDetails(detail, productTypeDict.options, businessStatusDict.options).map(([label, value]) => <Detail key={label} label={label} value={value} />)}
          </dl>
          <div className="mt-4 flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setDetail(null)}>关闭</Button>
            <Button onClick={() => { setEditing(detail); setDetail(null); }}>编辑</Button>
          </div>
        </dialog>
      )}
      {editing && <ProductForm
        key={editing.id}
        record={editing}
        typeOptions={productTypeDict.options}
        typeIssue={productTypeIssue}
        retryTypes={productTypeDict.reload}
        onCancel={() => setEditing(null)}
        onSave={save}
      />}
    </>
  );
}

function newProduct(): ProductRecord {
  return { id: 0, code: "", name: "", type: "GOODS", unit: "", status: 1 };
}

function productDetails(
  record: ProductRecord,
  typeOptions: readonly DictSelectOption<ProductRecord["type"]>[],
  statusOptions: readonly DictSelectOption<ApiStatus>[],
): [string, string | null | undefined][] {
  return [
    ["编码", record.code], ["名称", record.name], ["类型", businessDictLabel(typeOptions, record.type)],
    ["品牌", record.brand], ["型号", record.model], ["规格", record.specification], ["分类", record.category],
    ["基本单位", record.unit], ["参考采购价", record.purchasePrice], ["参考销售价", record.salePrice],
    ["状态", businessDictLabel(statusOptions, record.status)], ["备注", record.remark],
  ];
}

function Detail({ label, value }: { label: string; value?: string | null }) {
  return <div><dt className="text-xs text-text-tertiary">{label}</dt><dd>{value || "-"}</dd></div>;
}

function ProductForm({ record, typeOptions, typeIssue, retryTypes, onCancel, onSave }: {
  record: ProductRecord;
  typeOptions: readonly DictSelectOption<ProductRecord["type"]>[];
  typeIssue: string;
  retryTypes: () => void;
  onCancel: () => void;
  onSave: (value: ProductDraft, id?: number) => Promise<void>;
}) {
  const [values, setValues] = useState<ProductDraft>(() => ({
    code: record.code, name: record.name, type: record.type, brand: record.brand,
    model: record.model, specification: record.specification, category: record.category,
    unit: record.unit, purchasePrice: record.purchasePrice, salePrice: record.salePrice, remark: record.remark,
  }));
  const [error, setError] = useState("");
  const textFields: [keyof ProductDraft, string, number][] = [
    ["code", "编码", 50], ["name", "名称", 200], ["brand", "品牌", 100], ["model", "型号", 100],
    ["specification", "规格", 200], ["category", "分类", 100], ["unit", "基本单位", 50], ["remark", "备注", 500],
  ];
  async function submit() {
    const invalidPrice = [values.purchasePrice, values.salePrice].some((price) => !validPrice(price));
    if (!values.name.trim() || !values.unit.trim() || invalidPrice) {
      setError("请填写名称和基本单位，参考价须为非负金额且最多两位小数。输入内容已保留。");
      return;
    }
    setError("");
    try {
      await onSave({ ...values, name: values.name.trim(), unit: values.unit.trim() }, record.id || undefined);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "保存失败，请检查输入后重试。");
    }
  }
  return (
    <FormDialog
      open
      title={record.id ? "编辑档案" : "新建档案"}
      onCancel={onCancel}
      onSubmit={submit}
      submitDisabled={!record.id && (Boolean(typeIssue) || !typeOptions.some((option) => option.value === values.type))}
      contentClassName="w-[min(720px,95vw)]"
      bodyClassName="overflow-auto"
    >
      <div className="grid gap-3 sm:grid-cols-2">
        {textFields.map(([key, label, maxLength]) => (
          <label key={key} className="grid gap-1 text-sm">{label}
            <Input maxLength={maxLength} required={key === "name"} value={String(values[key] ?? "")} onChange={(event) => setValues({ ...values, [key]: event.target.value })} />
          </label>
        ))}
        <label className="grid gap-1 text-sm">类型
          <Select value={values.type} onChange={(event) => setValues({ ...values, type: event.target.value as ProductRecord["type"] })}>
            {preserveCurrentDictOption(typeOptions, values.type).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
          </Select>
        </label>
        {(["purchasePrice", "salePrice"] as const).map((key) => (
          <label key={key} className="grid gap-1 text-sm">{key === "purchasePrice" ? "参考采购价（元）" : "参考销售价（元）"}
            <Input maxLength={20} inputMode="decimal" value={values[key] ?? ""} onChange={(event) => setValues({ ...values, [key]: event.target.value || null })} />
          </label>
        ))}
        {error && <p role="alert" className="col-span-full text-sm text-danger">{error}</p>}
        {typeIssue && <div role="alert" className="col-span-full flex items-center justify-between gap-3 text-sm text-danger"><span>{DICT_CODES.PRODUCT_TYPE}：{typeIssue}</span><Button type="button" size="sm" variant="secondary" onClick={retryTypes}>重试</Button></div>}
      </div>
    </FormDialog>
  );
}

function validPrice(value?: string | null): boolean {
  if (!value?.trim()) return true;
  if (!/^\d+(?:\.\d{1,2})?$/.test(value.trim())) return false;
  try {
    const [whole, fraction = ""] = value.trim().split(".");
    const cents = BigInt(whole) * 100n + BigInt((fraction + "00").slice(0, 2));
    return cents <= 9223372036854775807n;
  } catch {
    return false;
  }
}
