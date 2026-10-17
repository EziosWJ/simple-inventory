import { useCallback, useEffect, useState } from "react";
import { getUserDetail, getUserPage } from "@/api/user";
import { FormDialog } from "@/components/common/form-dialog";
import { PagedRecordSelect } from "@/components/common/paged-record-select";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { McpTokenCreateRequest } from "@/types/mcp-token";
import type { UserRecord } from "@/types/user";

type ExpiryMode = "permanent" | "7" | "30" | "90" | "custom";

const EXPIRY_OPTIONS: { value: ExpiryMode; label: string }[] = [
  { value: "permanent", label: "永久" },
  { value: "7", label: "7 天" },
  { value: "30", label: "30 天" },
  { value: "90", label: "90 天" },
  { value: "custom", label: "自定义日期" },
];

type TokenFormDialogProps = {
  open: boolean;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (payload: McpTokenCreateRequest) => void | Promise<void>;
};

export function TokenFormDialog({ open, submitting, onCancel, onSubmit }: TokenFormDialogProps) {
  const [name, setName] = useState("");
  const [userId, setUserId] = useState(0);
  const [expiryMode, setExpiryMode] = useState<ExpiryMode>("permanent");
  const [customDate, setCustomDate] = useState("");
  const [errors, setErrors] = useState<{ name?: string; userId?: string; customDate?: string }>({});

  useEffect(() => {
    if (open) {
      setName("");
      setUserId(0);
      setExpiryMode("permanent");
      setCustomDate("");
      setErrors({});
    }
  }, [open]);

  const loadPage = useCallback(
    (keyword: string, page: number, pageSize: number) =>
      getUserPage({ keyword, page, pageSize }),
    [],
  );

  const submit = () => {
    const nextErrors: typeof errors = {};
    const trimmedName = name.trim();

    if (!trimmedName) nextErrors.name = "请输入令牌名称";
    else if (trimmedName.length > 100) nextErrors.name = "令牌名称不能超过 100 个字符";

    if (!userId) nextErrors.userId = "请选择绑定用户";

    if (expiryMode === "custom" && !customDate) {
      nextErrors.customDate = "请选择到期日期";
    }

    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;

    if (expiryMode === "permanent") {
      void onSubmit({ name: trimmedName, userId, permanent: true });
      return;
    }

    if (expiryMode === "custom") {
      void onSubmit({ name: trimmedName, userId, expiresOn: customDate });
      return;
    }

    void onSubmit({ name: trimmedName, userId, expiresInDays: Number(expiryMode) });
  };

  return (
    <FormDialog
      open={open}
      title="新建 MCP 令牌"
      description="令牌绑定已有用户，工具权限动态继承该用户当前的业务权限。"
      loading={submitting}
      submitText="创建令牌"
      contentClassName="max-w-modal-md"
      onCancel={onCancel}
      onSubmit={submit}
    >
      <div className="space-y-space-4">
        <label className="block">
          <span className="mb-1.5 block text-sm font-medium text-text-primary">
            令牌名称<span className="ml-1 text-error">*</span>
          </span>
          <Input
            value={name}
            maxLength={100}
            placeholder="例如：手机客户端"
            disabled={submitting}
            onChange={(event) => setName(event.target.value)}
          />
          <span className="mt-1 block min-h-[18px] text-xs text-error">{errors.name ?? ""}</span>
        </label>

        <div>
          <PagedRecordSelect<UserRecord>
            label="绑定用户"
            value={userId}
            disabled={submitting}
            loadPage={loadPage}
            loadRecord={getUserDetail}
            describe={(user) => user.nickname || user.username || `用户 #${user.id}`}
            warning={() => ""}
            onChange={(user) => setUserId(user?.id ?? 0)}
          />
          <span className="mt-1 block min-h-[18px] text-xs text-error">{errors.userId ?? ""}</span>
        </div>

        <fieldset>
          <legend className="mb-space-2 text-form-label font-medium text-text-primary">
            有效期
          </legend>
          <div className="flex flex-wrap gap-space-2">
            {EXPIRY_OPTIONS.map((option) => (
              <Button
                key={option.value}
                size="sm"
                variant={expiryMode === option.value ? "primary" : "secondary"}
                disabled={submitting}
                onClick={() => setExpiryMode(option.value)}
              >
                {option.label}
              </Button>
            ))}
          </div>

          {expiryMode === "custom" && (
            <div className="mt-space-3">
              <Input
                type="date"
                className="w-48"
                value={customDate}
                disabled={submitting}
                aria-label="到期日期"
                onChange={(event) => setCustomDate(event.target.value)}
              />
              <span className="mt-1 block min-h-[18px] text-xs text-error">
                {errors.customDate ?? "按中国时区日历日解释，包含当天。"}
              </span>
            </div>
          )}
        </fieldset>

        <p className={cn("text-helper text-text-tertiary")}>
          默认永久有效；也可选择固定天数或指定到期日。
        </p>
      </div>
    </FormDialog>
  );
}
