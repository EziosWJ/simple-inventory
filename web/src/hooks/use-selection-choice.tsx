import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogOverlay, DialogTitle } from "@/components/ui/dialog";
type Choice = {
    title: string;
    description: string;
    keepText?: string;
    replaceText: string;
};
// Resolve only after a business choice. Cancelling never changes the form.
export function useSelectionChoice() {
    const [choice, setChoice] = useState<Choice | null>(null);
    const resolve = useRef<((value: boolean | null) => void) | null>(null);
    const finish = useCallback((value: boolean | null) => { resolve.current?.(value); resolve.current = null; setChoice(null); }, []);
    const choose = useCallback((next: Choice) => new Promise<boolean | null>(done => { resolve.current?.(null); resolve.current = done; setChoice(next); }), []);
    useEffect(() => () => { resolve.current?.(null); }, []);
    const dialog = <Dialog open={Boolean(choice)} onOpenChange={open => { if (!open)
        finish(null); }} trapFocus lockScroll>
  <DialogOverlay /><DialogContent className="max-w-modal-sm" onKeyDown={e => { if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        finish(null);
    } }}>
   <DialogHeader><DialogTitle>{choice?.title}</DialogTitle></DialogHeader>
   <DialogBody><p className="text-sm text-text-secondary">{choice?.description}</p></DialogBody>
   <DialogFooter className="flex-wrap"><Button autoFocus variant="secondary" onClick={() => finish(null)}>取消换选</Button>{choice?.keepText && <Button variant="secondary" onClick={() => finish(false)}>{choice.keepText}</Button>}<Button onClick={() => finish(true)}>{choice?.replaceText}</Button></DialogFooter>
  </DialogContent>
 </Dialog>;
    return { choose, dialog, busy: Boolean(choice) };
}
