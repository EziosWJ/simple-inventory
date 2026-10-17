import { Copy, KeyRound, Play, RotateCcw, Square } from "lucide-react";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types/table";
import type { McpTokenRecord } from "@/types/mcp-token";

type TokenStatusMeta = {
  label: string;
  tone: "success" | "warning" | "error";
};

/** 状态优先级：吊销 > 停用 > 过期 > 启用中。 */
export function tokenStatusMeta(record: McpTokenRecord): TokenStatusMeta {
  if (record.revoked) return { label: "已吊销", tone: "error" };
  if (!record.enabled) return { label: "已停用", tone: "warning" };
  if (record.expired) return { label: "已过期", tone: "warning" };
  return { label: "启用中", tone: "success" };
}

type TokenColumnActions = {
  onView: (record: McpTokenRecord) => void;
  onExtend: (record: McpTokenRecord) => void;
  onToggleStatus: (record: McpTokenRecord) => void;
  onReset: (record: McpTokenRecord) => void;
  /** 正在读取原值的令牌 id，用于禁用按钮避免重复请求。 */
  viewingId: number | null;
};

export function createTokenColumns({
  onView,
  onExtend,
  onToggleStatus,
  onReset,
  viewingId,
}: TokenColumnActions): DataTableColumn<McpTokenRecord>[] {
  return [
    {
      title: "名称",
      key: "name",
      width: 200,
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">{record.name}</div>
          <div className="text-xs text-text-tertiary">ID {record.id}</div>
        </div>
      ),
    },
    {
      title: "绑定用户",
      key: "user",
      width: 160,
      render: (_, record) => (
        <div>
          <div className="text-text-primary">{record.userName || "-"}</div>
          <div className="text-xs text-text-tertiary">用户 ID {record.userId}</div>
        </div>
      ),
    },
    {
      title: "状态",
      key: "state",
      width: 100,
      align: "center",
      render: (_, record) => {
        const meta = tokenStatusMeta(record);
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "有效期",
      key: "expiresAt",
      width: 220,
      render: (_, record) =>
        record.permanent ? (
          <span className="text-text-primary">永久</span>
        ) : (
          <div className="space-y-0.5">
            <div className="whitespace-nowrap tabular-nums text-text-primary">
              {formatDateTime(record.expiresAt)}
            </div>
            {record.expired && <div className="text-xs text-warning">已过期</div>}
          </div>
        ),
    },
    {
      title: "是否支持查看",
      key: "secretViewable",
      width: 130,
      align: "center",
      render: (_, record) =>
        record.secretViewable ? (
          <StatusTag tone="success">支持查看</StatusTag>
        ) : (
          <StatusTag tone="neutral">不支持</StatusTag>
        ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 340,
      render: (_, record) => (
        <div className="inline-flex flex-wrap items-center justify-center gap-1">
          <Button
            size="sm"
            variant="ghost"
            disabled={!record.secretViewable || viewingId === record.id}
            title={record.secretViewable ? undefined : "旧版令牌，重置后可查看"}
            onClick={() => onView(record)}
          >
            <Copy className="h-4 w-4" aria-hidden />
            查看原值
          </Button>
          <Button size="sm" variant="ghost" onClick={() => onExtend(record)}>
            <KeyRound className="h-4 w-4" aria-hidden />
            延期
          </Button>
          <Button size="sm" variant="ghost" onClick={() => onToggleStatus(record)}>
            {record.enabled ? (
              <>
                <Square className="h-4 w-4" aria-hidden />
                停用
              </>
            ) : (
              <>
                <Play className="h-4 w-4" aria-hidden />
                启用
              </>
            )}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => onReset(record)}>
            <RotateCcw className="h-4 w-4" aria-hidden />
            重置
          </Button>
        </div>
      ),
    },
  ];
}
