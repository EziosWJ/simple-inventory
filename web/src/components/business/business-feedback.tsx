import { AlertCircle, CheckCircle2, Info, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";
import type { BusinessFeedback as Feedback } from "@/hooks/use-business-feedback";

const variants = {
  success: { icon: CheckCircle2, style: "border-success-border bg-success-background", iconStyle: "text-success" },
  warning: { icon: TriangleAlert, style: "border-warning-border bg-warning-background", iconStyle: "text-warning" },
  error: { icon: AlertCircle, style: "border-error-border bg-error-background", iconStyle: "text-error" },
  info: { icon: Info, style: "border-info-border bg-info-background", iconStyle: "text-info" },
};

export function BusinessFeedback({ feedback, children }: { feedback: Feedback | null; children?: ReactNode }) {
  if (!feedback) return null;
  const { icon: Icon, style, iconStyle } = variants[feedback.type];
  return <section aria-label="操作结果" role={feedback.type === "error" ? "alert" : "status"}
    className={`flex min-w-0 gap-space-3 rounded-admin border p-space-4 ${style}`}>
    <Icon className={`mt-0.5 h-5 w-5 shrink-0 ${iconStyle}`} aria-hidden />
    <div className="min-w-0 flex-1 space-y-space-2">
      <p className="font-semibold text-text-primary">{feedback.title}</p>
      <p className="break-words text-sm text-text-secondary">{feedback.description}</p>
      {(feedback.documentNo || feedback.status) && <p className="break-all text-sm text-text-secondary">
        {feedback.documentNo && `单号：${feedback.documentNo}`}{feedback.documentNo && feedback.status && " · "}{feedback.status}
      </p>}
      {feedback.detail && <p className="break-words text-xs text-text-tertiary">{feedback.detail}</p>}
      {children && <div className="flex flex-wrap gap-space-2">{children}</div>}
    </div>
  </section>;
}
