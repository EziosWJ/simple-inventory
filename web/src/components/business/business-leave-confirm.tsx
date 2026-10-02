import { ConfirmDialog } from "@/components/common/confirm-dialog";
import type { useBusinessLeaveGuard } from "@/hooks/use-business-leave-guard";

export function BusinessLeaveConfirm({ guard, title = "放弃未保存修改？", description, stayText = "继续编辑", leaveText = "放弃并离开" }: {
  guard: ReturnType<typeof useBusinessLeaveGuard>;
  title?: string;
  description?: string;
  stayText?: string;
  leaveText?: string;
}) {
  return <ConfirmDialog open={guard.open} title={title}
    description={description ?? "有未保存修改，离开将丢弃当前输入。"}
    cancelText={stayText} confirmText={leaveText} loading={guard.busy}
    onCancel={guard.stay} onConfirm={guard.leave}
    role="alertdialog" focusCancel restoreFocus />;
}
