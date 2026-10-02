import { useEffect, useState } from "react";
import { useBlocker } from "react-router-dom";

// Business editors opt in; query pages and scaffold dialogs keep their defaults.
export function useBusinessLeaveGuard({ dirty, busy = false, uncertain = false }: {
  dirty: boolean;
  busy?: boolean;
  uncertain?: boolean;
}) {
  const [closeAction, setCloseAction] = useState<(() => void) | null>(null);
  const shouldWarn = dirty || busy || uncertain;
  const blocker = useBlocker(shouldWarn);

  useEffect(() => {
    if (!shouldWarn) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [shouldWarn]);

  return {
    open: closeAction !== null || blocker.state === "blocked",
    busy,
    requestClose(action: () => void) {
      if (busy) return;
      if (shouldWarn) setCloseAction(() => action);
      else action();
    },
    stay() {
      setCloseAction(null);
      if (blocker.state === "blocked") blocker.reset();
    },
    leave() {
      if (busy) return;
      setCloseAction(null);
      if (blocker.state === "blocked") blocker.proceed();
      else closeAction?.();
    },
  };
}
