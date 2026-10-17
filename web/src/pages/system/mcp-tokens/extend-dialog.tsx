import { useState } from "react";
import { FormDialog } from "@/components/common/form-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { formatDateTime } from "@/lib/datetime";
import { cn } from "@/lib/utils";
import type { McpTokenExtendRequest, McpTokenRecord } from "@/types/mcp-token";

type ExtendMode = "days" | "permanent";

const QUICK_DAYS = [7, 30, 90];

type ExtendDialogProps = {
  record: McpTokenRecord | null;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (payload: McpTokenExtendRequest) => void | Promise<void>;
};

export function ExtendDialog({ record, submitting, onCancel, onSubmit }: ExtendDialogProps) {
  const [mode, setMode] = useState<ExtendMode>("days");
  const [days, setDays] = useState("30");
  const [error, setError] = useState("");

  if (!record) return null;

  const currentExpiry = record.permanent
    ? "永久"
    : `${formatDateTime(record.expiresAt)}${record.expired ? "（已过期）" : ""}`;

  const submit = () => {
    if (mode === "permanent") {
      setError("");
      void onSubmit({ permanent: true });
      return;
    }

    const value = Number(days);
    if (!Number.isInteger(value) || value < 1 || value > 3650) {
      setError("请输入 1 到 3650 之间的整数天数");
      return;
    }

    setError("");
    void onSubmit({ days: value });
  };

  return (
    <FormDialog
      open
      title="延期令牌"
      description={`令牌「${record.name}」当前有效期：${currentExpiry}`}
      loading={submitting}
      submitText="确定延期"
      contentClassName="max-w-modal-md"
      onCancel={onCancel}
      onSubmit={submit}
    >
      <div className="space-y-space-4">
        <p className="rounded-control border border-info-border bg-info-background px-space-3 py-space-2 text-helper text-text-secondary">
          未过期将从原到期时间延长，已过期将从当前时间计算；延期不会自动启用已停用的令牌。
        </p>

        <fieldset className="space-y-space-3">
          <legend className="mb-space-2 text-form-label font-medium text-text-primary">延期方式</legend>

          <label className="flex cursor-pointer items-center gap-space-2 text-sm text-text-primary">
            <input
              type="radio"
              name="extend-mode"
              className="h-4 w-4 accent-primary"
              checked={mode === "days"}
              onChange={() => setMode("days")}
            />
            增加天数
          </label>

          <div className={cn("flex flex-wrap items-center gap-space-2 pl-6", mode !== "days" && "opacity-50")}>
            {QUICK_DAYS.map((quick) => (
              <Button
                key={quick}
                size="sm"
                variant={days === String(quick) && mode === "days" ? "primary" : "secondary"}
                disabled={mode !== "days" || submitting}
                onClick={() => { setMode("days"); setDays(String(quick)); setError(""); }}
              >
                {quick} 天
              </Button>
            ))}
            <Input
              type="number"
              min={1}
              max={3650}
              className="w-28"
              value={days}
              disabled={mode !== "days" || submitting}
              aria-label="增加天数"
              onChange={(event) => { setMode("days"); setDays(event.target.value); setError(""); }}
            />
          </div>

          <label className="flex cursor-pointer items-center gap-space-2 text-sm text-text-primary">
            <input
              type="radio"
              name="extend-mode"
              className="h-4 w-4 accent-primary"
              checked={mode === "permanent"}
              onChange={() => { setMode("permanent"); setError(""); }}
            />
            改为永久有效
          </label>
        </fieldset>

        {error && <p role="alert" className="text-sm text-error">{error}</p>}
      </div>
    </FormDialog>
  );
}
