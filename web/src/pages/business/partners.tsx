import { useEffect, useState } from "react";
import { partnerPage, partnerStatus, savePartner, type PartnerRecord } from "@/api/business";
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
  PARTNER_IDENTITY_VALUES,
  PARTNER_TYPE_VALUES,
  type DictSelectOption,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import {
  businessDictLabel,
  missingBusinessDictValues,
  preserveCurrentDictOption,
} from "@/lib/business-dict-label";
import type { ApiStatus, DataTableColumn } from "@/types";

type Filters = { keyword: string; type: string; identity: string; status: string };
type PartnerDraft = Omit<PartnerRecord, "id" | "status">;
const emptyFilters: Filters = { keyword: "", type: "", identity: "", status: "" };

export function PartnersPage() {
  const partnerTypeDict = useDictOptions<PartnerRecord["type"]>(DICT_CODES.PARTNER_TYPE, {
    allowedValues: PARTNER_TYPE_VALUES,
  });
  const partnerIdentityDict = useDictOptions<(typeof PARTNER_IDENTITY_VALUES)[number]>(DICT_CODES.PARTNER_IDENTITY, {
    allowedValues: PARTNER_IDENTITY_VALUES,
  });
  const businessStatusDict = useDictOptions<ApiStatus>(DICT_CODES.BUSINESS_STATUS, {
    allowedValues: BUSINESS_STATUS_VALUES,
    valueType: "number",
  });
  const [records, setRecords] = useState<PartnerRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [draftFilters, setDraftFilters] = useState<Filters>(emptyFilters);
  const [filters, setFilters] = useState<Filters>(emptyFilters);
  const [queryRun, setQueryRun] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [editing, setEditing] = useState<PartnerRecord | null>(null);
  const [detail, setDetail] = useState<PartnerRecord | null>(null);
  const missingTypes = missingBusinessDictValues(partnerTypeDict.options, PARTNER_TYPE_VALUES);
  const missingIdentities = missingBusinessDictValues(partnerIdentityDict.options, PARTNER_IDENTITY_VALUES);
  const missingStatuses = missingBusinessDictValues(businessStatusDict.options, BUSINESS_STATUS_VALUES);
  const partnerTypeIssue = partnerTypeDict.error || (missingTypes.length ? `字典缺少可用值：${missingTypes.join("、")}` : "");
  const partnerIdentityIssue = partnerIdentityDict.error || (missingIdentities.length ? `字典缺少可用值：${missingIdentities.join("、")}` : "");
  const businessStatusIssue = businessStatusDict.error || (missingStatuses.length ? `字典缺少可用值：${missingStatuses.join("、")}` : "");

  useEffect(() => {
    let current = true;
    setLoading(true);
    void partnerPage({ page, pageSize: 10, ...filters })
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
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [filters, page, queryRun]);

  const columns: DataTableColumn<PartnerRecord>[] = [
    {
      title: "往来单位",
      key: "name",
      render: (_, record) => <div><div className="font-medium">{record.name}</div><div className="text-xs text-text-tertiary">{record.code}</div></div>,
    },
    { title: "分类", dataIndex: "type", render: (value) => businessDictLabel(partnerTypeDict.options, value) },
    { title: "身份", key: "identity", render: (_, record) => identities(record, partnerIdentityDict.options).join(" / ") },
    { title: "联系人", dataIndex: "contact", render: (value) => value || "-" },
    { title: "电话", dataIndex: "phone", render: (value) => value || "-" },
    { title: "状态", dataIndex: "status", render: (value) => businessDictLabel(businessStatusDict.options, value) },
    {
      title: "操作",
      key: "actions",
      render: (_, record) => <div className="flex gap-1">
        <Button size="sm" variant="secondary" onClick={() => setDetail(record)}>查看</Button>
        <Button size="sm" variant="secondary" onClick={() => setEditing(record)}>编辑</Button>
        <Button size="sm" variant="secondary" onClick={() => void changeStatus(record)}>{businessDictLabel(businessStatusDict.options, record.status ? 0 : 1)}</Button>
      </div>,
    },
  ];

  async function changeStatus(record: PartnerRecord) {
    setActionError("");
    try {
      await partnerStatus(record.id, record.status ? 0 : 1);
      setQueryRun((value) => value + 1);
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : "状态更新失败");
    }
  }

  function query() { setFilters({ ...draftFilters }); setPage(1); setQueryRun((value) => value + 1); }
  function reset() { setDraftFilters(emptyFilters); setFilters(emptyFilters); setPage(1); setQueryRun((value) => value + 1); }

  async function save(value: PartnerDraft, id?: number) {
    await savePartner(value, id);
    setEditing(null);
    setQueryRun((current) => current + 1);
  }

  return <>
    <PageHeader title="往来单位" description="客户与供应商共用一份档案，可同时拥有两种身份。" actions={<Button onClick={() => setEditing(newPartner())}>新建档案</Button>} />
    <SearchFilterBar actions={<><Button variant="secondary" onClick={reset}>重置</Button><Button onClick={query}>查询</Button></>}>
      <Input placeholder="编码、名称、联系人或电话" value={draftFilters.keyword} onChange={(event) => { setDraftFilters({ ...draftFilters, keyword: event.target.value }); setPage(1); }} />
      <Select value={draftFilters.type} onChange={(event) => { setDraftFilters({ ...draftFilters, type: event.target.value }); setPage(1); }}><option value="">全部分类</option>{partnerTypeDict.options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select>
      <Select value={draftFilters.identity} onChange={(event) => { setDraftFilters({ ...draftFilters, identity: event.target.value }); setPage(1); }}><option value="">全部身份</option>{partnerIdentityDict.options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select>
      <Select value={draftFilters.status} onChange={(event) => { setDraftFilters({ ...draftFilters, status: event.target.value }); setPage(1); }}><option value="">全部状态</option>{businessStatusDict.options.map((option) => <option key={option.value} value={String(option.value)}>{option.label}</option>)}</Select>
    </SearchFilterBar>
    {(partnerTypeIssue || partnerIdentityIssue || businessStatusIssue) && <div role="alert" className="mb-3 flex items-center justify-between gap-3 rounded-admin border border-danger/30 bg-danger/5 p-3 text-sm text-danger">
      <span>{[
        partnerTypeIssue && `${DICT_CODES.PARTNER_TYPE}：${partnerTypeIssue}`,
        partnerIdentityIssue && `${DICT_CODES.PARTNER_IDENTITY}：${partnerIdentityIssue}`,
        businessStatusIssue && `${DICT_CODES.BUSINESS_STATUS}：${businessStatusIssue}`,
      ].filter(Boolean).join("；")}</span>
      <Button size="sm" variant="secondary" onClick={() => { partnerTypeDict.reload(); partnerIdentityDict.reload(); businessStatusDict.reload(); }}>重试</Button>
    </div>}
    {actionError && <p role="alert" className="mb-3 text-sm text-danger">{actionError}</p>}
    <DataTableCard pagination={<Pagination page={page} pageSize={10} total={total} onPageChange={setPage} />}>
      <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} error={error} />
    </DataTableCard>
    {detail && <dialog open className="fixed inset-0 z-50 m-auto max-h-[90vh] w-[min(680px,95vw)] overflow-auto rounded-admin border border-border bg-surface p-card shadow-admin">
      <h2 className="mb-4 text-lg font-semibold">往来单位详情</h2>
      <dl className="grid gap-3 sm:grid-cols-2">{partnerDetails(detail, partnerTypeDict.options, partnerIdentityDict.options, businessStatusDict.options).map(([label, value]) => <Detail key={label} label={label} value={value} />)}</dl>
      <div className="mt-4 flex justify-end gap-2"><Button variant="secondary" onClick={() => setDetail(null)}>关闭</Button><Button onClick={() => { setEditing(detail); setDetail(null); }}>编辑</Button></div>
    </dialog>}
    {editing && <PartnerForm
      key={editing.id}
      record={editing}
      typeOptions={partnerTypeDict.options}
      typeIssue={partnerTypeIssue}
      retryTypes={partnerTypeDict.reload}
      identityOptions={partnerIdentityDict.options}
      identityIssue={partnerIdentityIssue}
      retryIdentities={partnerIdentityDict.reload}
      onCancel={() => setEditing(null)}
      onSave={save}
    />}
  </>;
}

function newPartner(): PartnerRecord { return { id: 0, code: "", name: "", type: "PERSON", isCustomer: true, isSupplier: false, status: 1 }; }
function identities(record: PartnerRecord, options: readonly DictSelectOption<(typeof PARTNER_IDENTITY_VALUES)[number]>[]) {
  const enabled = new Set<(typeof PARTNER_IDENTITY_VALUES)[number]>();
  if (record.isCustomer) enabled.add("CUSTOMER");
  if (record.isSupplier) enabled.add("SUPPLIER");
  return identityOptionOrder(options)
    .filter((value) => enabled.has(value))
    .map((value) => businessDictLabel(options, value));
}

function identityOptionOrder(options: readonly DictSelectOption<(typeof PARTNER_IDENTITY_VALUES)[number]>[]) {
  const configured = options.map((option) => option.value);
  const missing = PARTNER_IDENTITY_VALUES.filter((value) => !configured.includes(value));
  return [...configured, ...missing];
}
function partnerDetails(
  record: PartnerRecord,
  typeOptions: readonly DictSelectOption<PartnerRecord["type"]>[],
  identityOptions: readonly DictSelectOption<(typeof PARTNER_IDENTITY_VALUES)[number]>[],
  statusOptions: readonly DictSelectOption<ApiStatus>[],
): [string, string | null | undefined][] {
  return [
    ["编码", record.code], ["名称", record.name], ["分类", businessDictLabel(typeOptions, record.type)], ["身份", identities(record, identityOptions).join(" / ")],
    ["联系人", record.contact], ["电话", record.phone], ["地址", record.address], ["开票名称", record.invoiceName], ["税号", record.taxNumber],
    ["注册地址", record.registeredAddress], ["注册电话", record.registeredPhone], ["开户行", record.bankName], ["账号", record.bankAccount],
    ["状态", businessDictLabel(statusOptions, record.status)], ["备注", record.remark],
  ];
}
function Detail({ label, value }: { label: string; value?: string | null }) { return <div><dt className="text-xs text-text-tertiary">{label}</dt><dd>{value || "-"}</dd></div>; }

function PartnerForm({ record, typeOptions, typeIssue, retryTypes, identityOptions, identityIssue, retryIdentities, onCancel, onSave }: {
  record: PartnerRecord;
  typeOptions: readonly DictSelectOption<PartnerRecord["type"]>[];
  typeIssue: string;
  retryTypes: () => void;
  identityOptions: readonly DictSelectOption<(typeof PARTNER_IDENTITY_VALUES)[number]>[];
  identityIssue: string;
  retryIdentities: () => void;
  onCancel: () => void;
  onSave: (value: PartnerDraft, id?: number) => Promise<void>;
}) {
  const [values, setValues] = useState<PartnerDraft>(() => ({
    code: record.code, name: record.name, type: record.type, isCustomer: record.isCustomer, isSupplier: record.isSupplier,
    contact: record.contact, phone: record.phone, address: record.address, remark: record.remark, invoiceName: record.invoiceName,
    taxNumber: record.taxNumber, registeredAddress: record.registeredAddress, registeredPhone: record.registeredPhone,
    bankName: record.bankName, bankAccount: record.bankAccount,
  }));
  const [error, setError] = useState("");
  const fields: [keyof PartnerDraft, string, number][] = [
    ["code", "编码", 50], ["contact", "主要联系人", 100], ["phone", "电话", 50], ["address", "地址", 500],
    ["invoiceName", "开票名称", 200], ["taxNumber", "税号", 100], ["registeredAddress", "注册地址", 500],
    ["registeredPhone", "注册电话", 50], ["bankName", "开户行", 200], ["bankAccount", "账号", 100], ["remark", "备注", 500],
  ];
  async function submit() {
    if (!values.name.trim() || (!values.isCustomer && !values.isSupplier)) {
      setError("请填写名称，并至少选择客户或供应商身份。输入内容已保留。");
      return;
    }
    setError("");
    try { await onSave({ ...values, name: values.name.trim() }, record.id || undefined); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "保存失败，请检查输入后重试。"); }
  }
  return <FormDialog
    open
    title={record.id ? "编辑往来单位" : "新建往来单位"}
    onCancel={onCancel}
    onSubmit={submit}
    submitDisabled={!record.id && (
      Boolean(typeIssue || identityIssue)
      || !typeOptions.some((option) => option.value === values.type)
      || PARTNER_IDENTITY_VALUES.some((value) => !identityOptions.some((option) => option.value === value))
    )}
    contentClassName="w-[min(760px,95vw)]"
    bodyClassName="overflow-auto"
  >
    <div className="grid gap-3 sm:grid-cols-2">
      <label className="grid gap-1 text-sm">名称<Input required maxLength={200} value={values.name} onChange={(event) => setValues({ ...values, name: event.target.value })} /></label>
      <label className="grid gap-1 text-sm">编码<Input maxLength={50} value={values.code} onChange={(event) => setValues({ ...values, code: event.target.value })} /></label>
      <label className="grid gap-1 text-sm">分类<Select value={values.type} onChange={(event) => setValues({ ...values, type: event.target.value as PartnerRecord["type"] })}>{preserveCurrentDictOption(typeOptions, values.type).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select></label>
      {identityOptionOrder(identityOptions).map((value) => {
        const key = value === "CUSTOMER" ? "isCustomer" : "isSupplier";
        return <label key={value} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={values[key]} onChange={(event) => setValues({ ...values, [key]: event.target.checked })} />{businessDictLabel(identityOptions, value)}</label>;
      })}
      {fields.slice(1).map(([key, label, maxLength]) => <label key={key} className="grid gap-1 text-sm">{label}<Input maxLength={maxLength} value={String(values[key] ?? "")} onChange={(event) => setValues({ ...values, [key]: event.target.value || null })} /></label>)}
      {error && <p role="alert" className="col-span-full text-sm text-danger">{error}</p>}
      {typeIssue && <div role="alert" className="col-span-full flex items-center justify-between gap-3 text-sm text-danger"><span>{DICT_CODES.PARTNER_TYPE}：{typeIssue}</span><Button type="button" size="sm" variant="secondary" onClick={retryTypes}>重试</Button></div>}
      {identityIssue && <div role="alert" className="col-span-full flex items-center justify-between gap-3 text-sm text-danger"><span>{DICT_CODES.PARTNER_IDENTITY}：{identityIssue}</span><Button type="button" size="sm" variant="secondary" onClick={retryIdentities}>重试</Button></div>}
    </div>
  </FormDialog>;
}
