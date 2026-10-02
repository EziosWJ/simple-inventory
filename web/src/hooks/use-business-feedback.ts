import { useCallback, useRef, useState } from "react";
import { toast, type ToastType } from "@/components/common/toast-store";

export type BusinessFeedback = {
  type: ToastType;
  title: string;
  description: string;
  documentNo?: string;
  status?: string;
  detail?: string;
};

// Called by operation handlers, never by render or ordinary data loading.
export function useBusinessFeedback() {
  const [feedback, setFeedback] = useState<BusinessFeedback | null>(null);
  const activeToast = useRef<string | null>(null);
  const notify = useCallback((result: BusinessFeedback, duration = result.type === "success" ? 6000 : 8000) => {
    setFeedback(result);
    if (activeToast.current) toast.dismiss(activeToast.current);
    activeToast.current = toast.show(result.type, { title: result.title, description: result.description, duration });
  }, []);
  return { feedback, setFeedback, notify };
}
