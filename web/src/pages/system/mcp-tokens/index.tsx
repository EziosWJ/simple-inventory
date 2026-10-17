import { Plus, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import {
  createMcpToken,
  extendMcpToken,
  getMcpTokenPage,
  getMcpTokenSecret,
  resetMcpToken,
  updateMcpTokenStatus,
} from "@/api/mcp-token";
import { getUserDetail, getUserPage } from "@/api/user";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { PagedRecordSelect } from "@/components/common/paged-record-select";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { StatusTag } from "@/components/common/status-tag";
import { useListPage } from "@/hooks/use-list-page";
import { getErrorMessage } from "@/lib/api-error";
import type {
  McpTokenCreateRequest,
  McpTokenExtendRequest,
  McpTokenListQuery,
  McpTokenRecord,
  McpTokenState,
} from "@/types/mcp-token";
import type { UserRecord } from "@/types/user";
import { createTokenColumns } from "./columns";
import { ExtendDialog } from "./extend-dialog";
import { TokenFormDialog } from "./token-form-dialog";
import { TokenValueDialog, type TokenValueResult } from "./token-value-dialog";

type TokenFilters = {
  keyword: string;
  userId: number | null;
  state: McpTokenState | "";
};

const DEFAULT_FILTERS: TokenFilters = { keyword: "", userId: null, state: "" };

const STATE_OPTIONS: { value: McpTokenState | ""; label: string }[] = [
  { value: "", label: "全部状态" },
  { value: "ENABLED", label: "启用中" },
  { value: "DISABLED", label: "已停用" },
  { value: "EXPIRED", label: "已过期" },
];

type ConfirmAction =
  | { type: "status"; record: McpTokenRecord }
  | { type: "reset"; record: McpTokenRecord };

export function McpTokensPage() {
  const {
    data: tokens,
    total,
    loading,
    error,
    page,
    pageSize,
    setPage,
    setPageSize,
    filters,
    setFilter,
    submitFilters,
    resetFilters,
    reload: loadTokens,
  } = useListPage<TokenFilters, McpTokenRecord, McpTokenListQuery>({
    fetch: getMcpTokenPage,
    defaultFilters: DEFAULT_FILTERS,
    toQuery: (f, p, ps) => ({
      page: p,
      pageSize: ps,
      keyword: f.keyword.trim() || undefined,
      userId: f.userId ?? undefined,
      state: f.state || undefined,
    }),
    onError: (err) =>
      toast.error({
        title: "加载失败",
        description: getErrorMessage(err, "令牌列表加载失败"),
      }),
  });

  const [formOpen, setFormOpen] = useState(false);
  const [formSubmitting, setFormSubmitting] = useState(false);
  const [viewingId, setViewingId] = useState<number | null>(null);
  const [tokenValue, setTokenValue] = useState<TokenValueResult | null>(null);
  const [extendTarget, setExtendTarget] = useState<McpTokenRecord | null>(null);
  const [extendSubmitting, setExtendSubmitting] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);

  const loadUserPage = useCallback(
    (keyword: string, pageNumber: number, pageSizeNumber: number) =>
      getUserPage({ keyword, page: pageNumber, pageSize: pageSizeNumber }),
    [],
  );

  // 明文只在内存中流转：关闭弹窗立即清空，绝不写入 localStorage 或 console。
  const closeTokenValue = useCallback(() => setTokenValue(null), []);

  const submitCreate = async (payload: McpTokenCreateRequest) => {
    setFormSubmitting(true);
    try {
      const result = await createMcpToken(payload);
      setFormOpen(false);
      await loadTokens();
      setTokenValue({
        token: result.token,
        permanent: result.permanent,
        expiresAt: result.expiresAt,
        notice: "请立即复制，之后可在列表中查看。",
      });
      toast.success("令牌已创建");
    } catch (submitError) {
      toast.error({
        title: "创建失败",
        description: getErrorMessage(submitError, "请检查表单后重试"),
      });
    } finally {
      setFormSubmitting(false);
    }
  };

  const viewSecret = async (record: McpTokenRecord) => {
    if (!record.secretViewable) return;

    setViewingId(record.id);
    try {
      const result = await getMcpTokenSecret(record.id);
      setTokenValue({
        token: result.token,
        permanent: result.permanent,
        expiresAt: result.expiresAt,
      });
    } catch (viewError) {
      // 解密失败时原样展示后端 message，不覆盖为通用文案。
      toast.error({
        title: "查看原值失败",
        description: getErrorMessage(viewError, "无法查看令牌原值"),
      });
    } finally {
      setViewingId(null);
    }
  };

  const submitExtend = async (payload: McpTokenExtendRequest) => {
    if (!extendTarget) return;

    setExtendSubmitting(true);
    try {
      await extendMcpToken(extendTarget.id, payload);
      toast.success("令牌有效期已更新");
      setExtendTarget(null);
      await loadTokens();
    } catch (extendError) {
      toast.error({
        title: "延期失败",
        description: getErrorMessage(extendError, "请稍后重试"),
      });
    } finally {
      setExtendSubmitting(false);
    }
  };

  const runConfirmAction = async () => {
    if (!confirmAction) return;

    setConfirmLoading(true);
    try {
      if (confirmAction.type === "status") {
        const enabled = !confirmAction.record.enabled;
        await updateMcpTokenStatus(confirmAction.record.id, { enabled });
        toast.success(enabled ? "令牌已启用" : "令牌已停用");
        setConfirmAction(null);
        await loadTokens();
      } else {
        const result = await resetMcpToken(confirmAction.record.id);
        setConfirmAction(null);
        await loadTokens();
        setTokenValue({
          token: result.token,
          permanent: result.permanent,
          expiresAt: result.expiresAt,
          notice: "旧令牌已立即失效，请复制新值并更新客户端配置。",
        });
        toast.success("令牌已重置");
      }
    } catch (actionError) {
      toast.error({
        title: "操作失败",
        description: getErrorMessage(actionError, "请稍后重试"),
      });
    } finally {
      setConfirmLoading(false);
    }
  };

  const confirmMeta = useMemo(() => {
    if (!confirmAction) return null;

    if (confirmAction.type === "status") {
      const enabling = !confirmAction.record.enabled;
      return {
        title: enabling ? "启用令牌" : "停用令牌",
        description: enabling
          ? `确认启用令牌「${confirmAction.record.name}」吗？启用后该令牌的请求将立即生效。`
          : `停用后该令牌的请求会立即失败。确认停用令牌「${confirmAction.record.name}」吗？`,
        confirmText: enabling ? "启用" : "停用",
        danger: !enabling,
      };
    }

    return {
      title: "重置令牌",
      description: `重置后旧令牌立即失效，客户端需要重新配置。确认重置令牌「${confirmAction.record.name}」吗？`,
      confirmText: "重置",
      danger: true,
    };
  }, [confirmAction]);

  const columns = createTokenColumns({
    onView: (record) => void viewSecret(record),
    onExtend: (record) => setExtendTarget(record),
    onToggleStatus: (record) => setConfirmAction({ type: "status", record }),
    onReset: (record) => setConfirmAction({ type: "reset", record }),
    viewingId,
  });

  return (
    <>
      <PageHeader
        title="MCP令牌"
        description="管理 MCP 访问令牌：绑定用户、有效期与启停状态，工具权限动态继承绑定用户。"
        actions={
          <Button variant="primary" onClick={() => setFormOpen(true)}>
            <Plus className="h-4 w-4" aria-hidden />
            新建令牌
          </Button>
        }
      />

      <SearchFilterBar
        onSubmit={() => submitFilters()}
        actions={
          <>
            <Button variant="secondary" onClick={resetFilters}>
              <RotateCcw className="h-4 w-4" aria-hidden />
              重置
            </Button>
            <Button variant="primary" onClick={() => submitFilters()}>
              <Search className="h-4 w-4" aria-hidden />
              查询
            </Button>
          </>
        }
      >
        <Input
          value={filters.keyword}
          placeholder="令牌名称"
          aria-label="令牌名称"
          onChange={(event) => setFilter("keyword", event.target.value)}
        />
        <PagedRecordSelect<UserRecord>
          label="绑定用户"
          inlineBrowse
          value={filters.userId ?? 0}
          fallback={filters.userId ? `已选用户 #${filters.userId}` : undefined}
          loadPage={loadUserPage}
          loadRecord={getUserDetail}
          describe={(user) => user.nickname || user.username || `用户 #${user.id}`}
          warning={() => ""}
          onChange={(user) => setFilter("userId", user?.id ?? null)}
        />
        <Select
          value={filters.state}
          aria-label="令牌状态"
          onChange={(event) =>
            setFilter("state", event.target.value as McpTokenState | "")
          }
        >
          {STATE_OPTIONS.map((option) => (
            <option key={option.value || "all"} value={option.value}>
              {option.label}
            </option>
          ))}
        </Select>
      </SearchFilterBar>

      <DataTableCard
        toolbar={
          <TableToolbar
            title="令牌列表"
            description={`共 ${total} 条数据，当前显示 ${tokens.length} 条。`}
            actions={
              <>
                <StatusTag tone={loading ? "warning" : error ? "error" : "info"}>
                  {loading ? "加载中" : error ? "加载失败" : "已同步"}
                </StatusTag>
                <Button size="sm" variant="secondary" onClick={loadTokens}>
                  <RefreshCw className="h-4 w-4" aria-hidden />
                  刷新
                </Button>
              </>
            }
          />
        }
        pagination={
          <Pagination
            page={page}
            pageSize={pageSize}
            total={total}
            disabled={loading}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        }
      >
        <DataTable<McpTokenRecord>
          columns={columns}
          dataSource={tokens}
          rowKey="id"
          loading={loading}
          error={error}
          minWidth={1200}
          empty={
            <EmptyState
              title="暂无令牌数据"
              description="调整筛选条件后重新查询，或新建一枚 MCP 令牌。"
              actionText="新建令牌"
              onAction={() => setFormOpen(true)}
            />
          }
        />
      </DataTableCard>

      <TokenFormDialog
        open={formOpen}
        submitting={formSubmitting}
        onCancel={() => setFormOpen(false)}
        onSubmit={submitCreate}
      />

      <ExtendDialog
        record={extendTarget}
        submitting={extendSubmitting}
        onCancel={() => setExtendTarget(null)}
        onSubmit={submitExtend}
      />

      <TokenValueDialog result={tokenValue} onClose={closeTokenValue} />

      {confirmMeta && (
        <ConfirmDialog
          open={!!confirmAction}
          title={confirmMeta.title}
          description={confirmMeta.description}
          confirmText={confirmMeta.confirmText}
          danger={confirmMeta.danger}
          loading={confirmLoading}
          onConfirm={runConfirmAction}
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </>
  );
}
