import { Eye, Pencil, Plus, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { EmptyState } from "@/components/common/empty-state";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";

type DemoListStatus = "enabled" | "disabled" | "pending";

type DemoListRecord = {
  id: number;
  name: string;
  code: string;
  owner: string;
  department: string;
  status: DemoListStatus;
  updatedAt: string;
};

type DemoListDraft = {
  name: string;
  code: string;
  owner: string;
  department: string;
  status: DemoListStatus;
};

const initialRecords: DemoListRecord[] = [
  {
    id: 1001,
    name: "客户资料同步",
    code: "customer_sync",
    owner: "陈晨",
    department: "客户运营部",
    status: "enabled",
    updatedAt: "2026-05-01 09:30:00",
  },
  {
    id: 1002,
    name: "审批时效提醒",
    code: "approval_sla_notice",
    owner: "李响",
    department: "流程平台部",
    status: "pending",
    updatedAt: "2026-04-30 16:42:00",
  },
  {
    id: 1003,
    name: "归档任务清理",
    code: "archive_cleanup",
    owner: "周宁",
    department: "系统运维部",
    status: "disabled",
    updatedAt: "2026-04-28 11:18:00",
  },
];

const emptyDraft: DemoListDraft = {
  name: "",
  code: "",
  owner: "",
  department: "",
  status: "enabled",
};

const statusMeta: Record<
  DemoListStatus,
  { label: string; tone: "success" | "warning" | "neutral" }
> = {
  enabled: { label: "启用", tone: "success" },
  pending: { label: "待确认", tone: "warning" },
  disabled: { label: "停用", tone: "neutral" },
};

export function ListDemoPage() {
  const [records, setRecords] = useState(initialRecords);
  const [keyword, setKeyword] = useState("");
  const [status, setStatus] = useState<DemoListStatus | "all">("all");
  const [appliedKeyword, setAppliedKeyword] = useState("");
  const [appliedStatus, setAppliedStatus] =
    useState<DemoListStatus | "all">("all");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [viewing, setViewing] = useState<DemoListRecord | null>(null);
  const [editing, setEditing] = useState<DemoListRecord | null>(null);
  const [creating, setCreating] = useState(false);
  const [draft, setDraft] = useState<DemoListDraft>(emptyDraft);

  const filteredRecords = useMemo(
    () =>
      records.filter((record) => {
        const matchedKeyword =
          !appliedKeyword.trim() ||
          record.name.includes(appliedKeyword.trim()) ||
          record.code.includes(appliedKeyword.trim());
        const matchedStatus =
          appliedStatus === "all" || record.status === appliedStatus;

        return matchedKeyword && matchedStatus;
      }),
    [appliedKeyword, appliedStatus, records],
  );
  const pageCount = Math.max(Math.ceil(filteredRecords.length / pageSize), 1);
  const currentPage = Math.min(page, pageCount);
  const pagedRecords = useMemo(
    () =>
      filteredRecords.slice((currentPage - 1) * pageSize, currentPage * pageSize),
    [currentPage, filteredRecords, pageSize],
  );

  const applyQuery = () => {
    setAppliedKeyword(keyword);
    setAppliedStatus(status);
    setPage(1);
  };

  const resetQuery = () => {
    setKeyword("");
    setStatus("all");
    setAppliedKeyword("");
    setAppliedStatus("all");
    setPage(1);
  };

  const refresh = () => {
    applyQuery();
    toast.info({
      title: "演示数据已重新载入",
      description: "标准列表页 Demo 的数据来自页面内存，不请求后端接口。",
    });
  };

  const openCreate = () => {
    setDraft(emptyDraft);
    setCreating(true);
  };

  const openEdit = (record: DemoListRecord) => {
    setDraft({
      name: record.name,
      code: record.code,
      owner: record.owner,
      department: record.department,
      status: record.status,
    });
    setEditing(record);
  };

  const closeForm = () => {
    setCreating(false);
    setEditing(null);
  };

  const submitCreate = () => {
    const nextId =
      records.reduce((max, record) => Math.max(max, record.id), 1000) + 1;
    const created: DemoListRecord = {
      ...draft,
      id: nextId,
      updatedAt: formatDateTime(new Date().toISOString()),
    };

    setRecords((current) => [...current, created]);
    closeForm();
    setPage(1);
    toast.success({
      title: `已新建「${created.name}」`,
      description: "示例页只写入页面内存数据，不请求后端接口。",
    });
  };

  const submitEdit = () => {
    if (!editing) return;

    const updated: DemoListRecord = {
      ...editing,
      ...draft,
      updatedAt: formatDateTime(new Date().toISOString()),
    };

    setRecords((current) =>
      current.map((record) => (record.id === updated.id ? updated : record)),
    );
    closeForm();
    toast.success({
      title: `已保存「${updated.name}」`,
      description: "示例页只写入页面内存数据，不请求后端接口。",
    });
  };

  const columns: DataTableColumn<DemoListRecord>[] = [
    {
      title: "业务名称",
      key: "name",
      width: 260,
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">{record.name}</div>
          <div className="text-xs text-text-tertiary">
            {record.code} · ID {record.id}
          </div>
        </div>
      ),
    },
    { title: "负责人", dataIndex: "owner", width: 120 },
    { title: "所属部门", dataIndex: "department", width: 180 },
    {
      title: "状态",
      dataIndex: "status",
      width: 100,
      render: (value) => {
        const meta = statusMeta[value as DemoListStatus];
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "更新时间",
      dataIndex: "updatedAt",
      width: 180,
      render: (value) => (
        <span className="whitespace-nowrap tabular-nums">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 180,
      render: (_, record) => (
        <div className="inline-flex items-center gap-1">
          <Button size="sm" variant="ghost" onClick={() => setViewing(record)}>
            <Eye className="h-4 w-4" aria-hidden />
            查看
          </Button>
          <Button size="sm" variant="ghost" onClick={() => openEdit(record)}>
            <Pencil className="h-4 w-4" aria-hidden />
            编辑
          </Button>
        </div>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title="标准列表页 Demo"
        description="标准后台列表页结构示例，包含筛选区、工具栏、表格、状态标签、行操作和分页。"
        actions={
          <Button variant="primary" onClick={openCreate}>
            <Plus className="h-4 w-4" aria-hidden />
            新建记录
          </Button>
        }
      />

      <SearchFilterBar
        actions={
          <>
            <Button variant="secondary" onClick={resetQuery}>
              <RotateCcw className="h-4 w-4" aria-hidden />
              重置
            </Button>
            <Button variant="primary" onClick={applyQuery}>
              <Search className="h-4 w-4" aria-hidden />
              查询
            </Button>
          </>
        }
      >
        <Input
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          placeholder="搜索名称或编码"
        />
        <Select
          value={status}
          onChange={(event) =>
            setStatus(event.target.value as DemoListStatus | "all")
          }
          aria-label="筛选状态"
        >
          <option value="all">全部状态</option>
          <option value="enabled">启用</option>
          <option value="pending">待确认</option>
          <option value="disabled">停用</option>
        </Select>
      </SearchFilterBar>

      <DataTableCard>
        <TableToolbar
          title="标准列表"
          description={`共 ${filteredRecords.length} 条数据。`}
          actions={
            <Button size="sm" variant="secondary" onClick={refresh}>
              <RefreshCw className="h-4 w-4" aria-hidden />
              刷新
            </Button>
          }
        />
        <DataTable<DemoListRecord>
          columns={columns}
          dataSource={pagedRecords}
          rowKey="id"
          minWidth={1040}
          empty={<EmptyState title="暂无数据" description="调整筛选条件后重试。" />}
        />
        <Pagination
          page={currentPage}
          pageSize={pageSize}
          total={filteredRecords.length}
          onPageChange={setPage}
          onPageSizeChange={(nextPageSize) => {
            setPageSize(nextPageSize);
            setPage(1);
          }}
        />
      </DataTableCard>

      <DetailDialog
        open={Boolean(viewing)}
        title={viewing?.name ?? ""}
        description={viewing ? `${viewing.code} · ID ${viewing.id}` : undefined}
        onCancel={() => setViewing(null)}
        footer={
          <Button variant="secondary" onClick={() => setViewing(null)}>
            关闭
          </Button>
        }
      >
        {viewing && (
          <dl className="grid gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-[13px] text-text-tertiary">业务名称</dt>
              <dd className="mt-1 text-sm text-text-primary">{viewing.name}</dd>
            </div>
            <div>
              <dt className="text-[13px] text-text-tertiary">业务编码</dt>
              <dd className="mt-1 text-sm text-text-primary">{viewing.code}</dd>
            </div>
            <div>
              <dt className="text-[13px] text-text-tertiary">负责人</dt>
              <dd className="mt-1 text-sm text-text-primary">{viewing.owner}</dd>
            </div>
            <div>
              <dt className="text-[13px] text-text-tertiary">所属部门</dt>
              <dd className="mt-1 text-sm text-text-primary">
                {viewing.department}
              </dd>
            </div>
            <div>
              <dt className="text-[13px] text-text-tertiary">状态</dt>
              <dd className="mt-1 text-sm text-text-primary">
                <StatusTag tone={statusMeta[viewing.status].tone}>
                  {statusMeta[viewing.status].label}
                </StatusTag>
              </dd>
            </div>
            <div>
              <dt className="text-[13px] text-text-tertiary">更新时间</dt>
              <dd className="mt-1 text-sm tabular-nums text-text-primary">
                {viewing.updatedAt}
              </dd>
            </div>
          </dl>
        )}
      </DetailDialog>

      <FormDialog
        open={creating || editing !== null}
        title={creating ? "新建记录" : "编辑记录"}
        description="示例页只写入页面内存数据，不请求后端接口。"
        submitText={creating ? "创建" : "保存"}
        onCancel={closeForm}
        onSubmit={creating ? submitCreate : submitEdit}
        submitDisabled={!draft.name.trim() || !draft.code.trim()}
      >
        <div className="grid gap-space-4 md:grid-cols-2">
          <Field label="业务名称" htmlFor="demo-record-name" required>
            <Input
              id="demo-record-name"
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.target.value })}
              placeholder="请输入业务名称"
            />
          </Field>
          <Field label="业务编码" htmlFor="demo-record-code" required>
            <Input
              id="demo-record-code"
              value={draft.code}
              onChange={(event) => setDraft({ ...draft, code: event.target.value })}
              placeholder="例如 customer_sync"
            />
          </Field>
          <Field label="负责人" htmlFor="demo-record-owner">
            <Input
              id="demo-record-owner"
              value={draft.owner}
              onChange={(event) => setDraft({ ...draft, owner: event.target.value })}
              placeholder="请输入负责人"
            />
          </Field>
          <Field label="所属部门" htmlFor="demo-record-department">
            <Input
              id="demo-record-department"
              value={draft.department}
              onChange={(event) =>
                setDraft({ ...draft, department: event.target.value })
              }
              placeholder="请输入所属部门"
            />
          </Field>
          <Field label="状态" htmlFor="demo-record-status">
            <Select
              id="demo-record-status"
              value={draft.status}
              onChange={(event) =>
                setDraft({ ...draft, status: event.target.value as DemoListStatus })
              }
            >
              <option value="enabled">启用</option>
              <option value="pending">待确认</option>
              <option value="disabled">停用</option>
            </Select>
          </Field>
        </div>
      </FormDialog>
    </>
  );
}
