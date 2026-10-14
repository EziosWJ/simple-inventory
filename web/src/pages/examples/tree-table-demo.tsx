import { Plus, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { ContentCard } from "@/components/common/content-card";
import { DataTable } from "@/components/common/data-table";
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

type MemberStatus = "enabled" | "disabled";

type Category = {
  id: number;
  name: string;
  code: string;
};

type MemberRecord = {
  id: number;
  name: string;
  account: string;
  categoryId: number;
  role: string;
  status: MemberStatus;
  lastActive: string;
};

type MemberDraft = {
  name: string;
  account: string;
  categoryId: number;
  role: string;
  status: MemberStatus;
};

const categories: Category[] = [
  { id: 1, name: "全部组织", code: "all" },
  { id: 11, name: "产品研发中心", code: "rd" },
  { id: 12, name: "客户运营中心", code: "ops" },
  { id: 13, name: "系统运维部", code: "it" },
];

const assignableCategories = categories.filter((category) => category.id !== 1);

const initialMembers: MemberRecord[] = [
  {
    id: 1,
    name: "周宁",
    account: "zhouning",
    categoryId: 11,
    role: "产品负责人",
    status: "enabled",
    lastActive: "2026-05-02 18:20:00",
  },
  {
    id: 2,
    name: "陈晨",
    account: "chenchen",
    categoryId: 12,
    role: "运营专员",
    status: "enabled",
    lastActive: "2026-05-02 16:45:00",
  },
  {
    id: 3,
    name: "林远",
    account: "linyuan",
    categoryId: 13,
    role: "运维工程师",
    status: "disabled",
    lastActive: "2026-04-29 09:12:00",
  },
];

function emptyDraft(categoryId: number): MemberDraft {
  return {
    name: "",
    account: "",
    categoryId,
    role: "",
    status: "enabled",
  };
}

export function TreeTableDemoPage() {
  const [members, setMembers] = useState(initialMembers);
  const [selectedCategoryId, setSelectedCategoryId] = useState(1);
  const [keyword, setKeyword] = useState("");
  const [appliedKeyword, setAppliedKeyword] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [creating, setCreating] = useState(false);
  const [draft, setDraft] = useState<MemberDraft>(() => emptyDraft(11));
  const currentCategory = categories.find((item) => item.id === selectedCategoryId);

  const filteredMembers = useMemo(
    () =>
      members.filter((member) => {
        const matchedCategory =
          selectedCategoryId === 1 || member.categoryId === selectedCategoryId;
        const matchedKeyword =
          !appliedKeyword.trim() ||
          member.name.includes(appliedKeyword.trim()) ||
          member.account.includes(appliedKeyword.trim());

        return matchedCategory && matchedKeyword;
      }),
    [appliedKeyword, members, selectedCategoryId],
  );
  const pageCount = Math.max(Math.ceil(filteredMembers.length / pageSize), 1);
  const currentPage = Math.min(page, pageCount);
  const pagedMembers = useMemo(
    () => filteredMembers.slice((currentPage - 1) * pageSize, currentPage * pageSize),
    [currentPage, filteredMembers, pageSize],
  );

  const selectCategory = (categoryId: number) => {
    setSelectedCategoryId(categoryId);
    setPage(1);
  };

  const applyQuery = () => {
    setAppliedKeyword(keyword);
    setPage(1);
  };

  const resetQuery = () => {
    setKeyword("");
    setAppliedKeyword("");
    selectCategory(1);
  };

  const refresh = () => {
    applyQuery();
    toast.info({
      title: "演示数据已重新载入",
      description: "左树右表 Demo 的数据来自页面内存，不请求后端接口。",
    });
  };

  const openCreate = () => {
    setDraft(emptyDraft(selectedCategoryId === 1 ? 11 : selectedCategoryId));
    setCreating(true);
  };

  const submitCreate = () => {
    const nextId = members.reduce((max, member) => Math.max(max, member.id), 0) + 1;
    const created: MemberRecord = {
      ...draft,
      id: nextId,
      lastActive: formatDateTime(new Date().toISOString()),
    };

    setMembers((current) => [...current, created]);
    setCreating(false);
    selectCategory(draft.categoryId);
    toast.success({
      title: `已新增成员「${created.name}」`,
      description: "示例页只写入页面内存数据，不请求后端接口。",
    });
  };

  const columns: DataTableColumn<MemberRecord>[] = [
    {
      title: "成员",
      key: "member",
      width: 220,
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">{record.name}</div>
          <div className="text-xs text-text-tertiary">{record.account}</div>
        </div>
      ),
    },
    { title: "角色", dataIndex: "role", width: 160 },
    {
      title: "状态",
      dataIndex: "status",
      width: 100,
      render: (value) =>
        value === "enabled" ? (
          <StatusTag tone="success">启用</StatusTag>
        ) : (
          <StatusTag tone="neutral">停用</StatusTag>
        ),
    },
    {
      title: "最近活跃",
      dataIndex: "lastActive",
      width: 180,
      render: (value) => (
        <span className="whitespace-nowrap tabular-nums">
          {String(value || "-")}
        </span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title="左树右表 Demo"
        description="展示左侧组织树与右侧列表联动的常见后台页面结构。"
        actions={
          <Button variant="primary" onClick={openCreate}>
            <Plus className="h-4 w-4" aria-hidden />
            新增成员
          </Button>
        }
      />

      <div className="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
        <ContentCard title="组织分类" bodyClassName="p-2">
          <div className="space-y-1">
            {categories.map((category) => (
              <button
                key={category.id}
                type="button"
                className={`flex h-9 w-full items-center justify-between rounded-lg px-3 text-left text-sm transition-colors ${
                  selectedCategoryId === category.id
                    ? "bg-blue-50 text-primary"
                    : "text-text-secondary hover:bg-slate-50 hover:text-text-primary"
                }`}
                onClick={() => selectCategory(category.id)}
              >
                <span>{category.name}</span>
                <span className="text-xs text-text-tertiary">{category.code}</span>
              </button>
            ))}
          </div>
        </ContentCard>

        <div>
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
              placeholder="搜索姓名或账号"
            />
          </SearchFilterBar>

          <section className="rounded-admin border border-border bg-surface shadow-admin">
            <TableToolbar
              title={currentCategory?.name ?? "成员列表"}
              description={`当前显示 ${filteredMembers.length} 条成员数据。`}
              actions={
                <Button size="sm" variant="secondary" onClick={refresh}>
                  <RefreshCw className="h-4 w-4" aria-hidden />
                  刷新
                </Button>
              }
            />
            <DataTable<MemberRecord>
              columns={columns}
              dataSource={pagedMembers}
              rowKey="id"
              minWidth={760}
            />
            <Pagination
              page={currentPage}
              pageSize={pageSize}
              total={filteredMembers.length}
              onPageChange={setPage}
              onPageSizeChange={(nextPageSize) => {
                setPageSize(nextPageSize);
                setPage(1);
              }}
            />
          </section>
        </div>
      </div>

      <FormDialog
        open={creating}
        title="新增成员"
        description="示例页只写入页面内存数据，不请求后端接口。"
        submitText="新增"
        onCancel={() => setCreating(false)}
        onSubmit={submitCreate}
        submitDisabled={!draft.name.trim() || !draft.account.trim()}
      >
        <div className="grid gap-space-4 md:grid-cols-2">
          <Field label="姓名" htmlFor="demo-member-name" required>
            <Input
              id="demo-member-name"
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.target.value })}
              placeholder="请输入姓名"
            />
          </Field>
          <Field label="账号" htmlFor="demo-member-account" required>
            <Input
              id="demo-member-account"
              value={draft.account}
              onChange={(event) =>
                setDraft({ ...draft, account: event.target.value })
              }
              placeholder="请输入账号"
            />
          </Field>
          <Field label="组织" htmlFor="demo-member-category">
            <Select
              id="demo-member-category"
              value={String(draft.categoryId)}
              onChange={(event) =>
                setDraft({ ...draft, categoryId: Number(event.target.value) })
              }
            >
              {assignableCategories.map((category) => (
                <option key={category.id} value={category.id}>
                  {category.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="角色" htmlFor="demo-member-role">
            <Input
              id="demo-member-role"
              value={draft.role}
              onChange={(event) => setDraft({ ...draft, role: event.target.value })}
              placeholder="请输入角色"
            />
          </Field>
          <Field label="状态" htmlFor="demo-member-status">
            <Select
              id="demo-member-status"
              value={draft.status}
              onChange={(event) =>
                setDraft({ ...draft, status: event.target.value as MemberStatus })
              }
            >
              <option value="enabled">启用</option>
              <option value="disabled">停用</option>
            </Select>
          </Field>
        </div>
      </FormDialog>
    </>
  );
}
