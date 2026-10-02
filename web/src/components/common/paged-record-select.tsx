import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import type { ApiPageResult } from "@/types/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Pagination } from "@/components/common/pagination";

type RecordWithID = { id: number };
type Props<T extends RecordWithID> = {
  label: string;
  value: number;
  fallback?: string;
  disabled?: boolean;
  loadPage: (keyword: string, page: number, pageSize: number) => Promise<ApiPageResult<T>>;
  loadRecord: (id: number) => Promise<T>;
  describe: (record: T) => string;
  warning: (record: T) => string;
  onChange: (record: T | null) => void;
};
const PAGE_SIZE = 10;

// Selection is independent of search results: a new page never drops the ID.
// Each effect cancels its ownership of responses as soon as input changes,
// including during debounce, so delayed successes and failures are ignored.
export function PagedRecordSelect<T extends RecordWithID>({
  label, value, fallback, disabled, loadPage, loadRecord, describe, warning, onChange,
}: Props<T>) {
  const id = useId();
  const searchRef = useRef<HTMLInputElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const positionPanelRef = useRef<(() => void) | null>(null);
  const [open, setOpen] = useState(false);
  const [keyword, setKeyword] = useState("");
  const [page, setPage] = useState(1);
  const [retry, setRetry] = useState(0);
  const [selected, setSelected] = useState<T | null>(null);
  const [selectionError, setSelectionError] = useState("");
  const [selectionLoading, setSelectionLoading] = useState(false);
  const [records, setRecords] = useState<T[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    setSelectionError("");
    setSelectionLoading(value > 0);
    if (value > 0) {
      void loadRecord(value).then(record => {
        if (active) setSelected(record);
      }).catch((reason: unknown) => {
        if (active) setSelectionError(reason instanceof Error ? reason.message : "已选资料加载失败");
      }).finally(() => { if (active) setSelectionLoading(false); });
    }
    return () => { active = false; };
  }, [value, loadRecord, retry]);

  useLayoutEffect(() => {
    const panel = panelRef.current;
    const trigger = triggerRef.current;
    if (!open || disabled || !panel || !trigger) return;

    // The native top layer escapes card/dialog clipping while keeping the
    // panel in DOM order for keyboard navigation and parent focus handling.
    panel.showPopover();
    function positionPanel() {
      if (!panel || !trigger) return;
      const viewport = window.visualViewport;
      const viewportLeft = viewport?.offsetLeft ?? 0;
      const viewportTop = viewport?.offsetTop ?? 0;
      const viewportWidth = viewport?.width ?? document.documentElement.clientWidth;
      const viewportHeight = viewport?.height ?? window.innerHeight;
      const edge = 8;
      const gap = 4;
      const rect = trigger.getBoundingClientRect();
      if (rect.bottom <= viewportTop || rect.top >= viewportTop + viewportHeight) {
        setOpen(false);
        return;
      }

      const width = Math.min(Math.max(rect.width, 320), 480, viewportWidth - edge * 2);
      panel.style.width = `${width}px`;
      panel.style.maxHeight = "480px";
      const below = Math.max(0, viewportTop + viewportHeight - rect.bottom - gap - edge);
      const above = Math.max(0, rect.top - viewportTop - gap - edge);
      const preferredHeight = Math.min(panel.scrollHeight, 480);
      const upwards = below < preferredHeight && above > below;
      panel.style.maxHeight = `${Math.min(480, upwards ? above : below)}px`;
      const height = panel.getBoundingClientRect().height;
      panel.style.left = `${Math.max(viewportLeft + edge, Math.min(rect.left, viewportLeft + viewportWidth - width - edge))}px`;
      panel.style.top = `${upwards ? rect.top - gap - height : rect.bottom + gap}px`;
    }

    positionPanelRef.current = positionPanel;
    positionPanel();
    searchRef.current?.focus({ preventScroll: true });
    const observer = new ResizeObserver(positionPanel);
    observer.observe(panel);
    observer.observe(trigger);
    // Capture scrolls in the app's content area and in form dialogs as well.
    function onScroll(event: Event) {
      if (event.target instanceof Node && panel?.contains(event.target)) return;
      positionPanel();
    }
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", positionPanel);
    window.visualViewport?.addEventListener("resize", positionPanel);
    window.visualViewport?.addEventListener("scroll", positionPanel);
    return () => {
      positionPanelRef.current = null;
      observer.disconnect();
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", positionPanel);
      window.visualViewport?.removeEventListener("resize", positionPanel);
      window.visualViewport?.removeEventListener("scroll", positionPanel);
      if (panel.matches(":popover-open")) panel.hidePopover();
    };
  }, [open, disabled]);

  useLayoutEffect(() => {
    positionPanelRef.current?.();
  }, [records, loading, error, total]);

  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setError("");
    setRecords([]);
    setTotal(0);
    const timer = window.setTimeout(() => {
      void loadPage(keyword.trim(), page, PAGE_SIZE).then(result => {
        if (active) { setRecords(result.records); setTotal(result.total); }
      }).catch((reason: unknown) => {
        if (active) setError(reason instanceof Error ? reason.message : "搜索失败");
      }).finally(() => { if (active) setLoading(false); });
    }, 250);
    return () => { active = false; window.clearTimeout(timer); };
  }, [open, keyword, page, loadPage, retry]);

  const current = selected?.id === value ? selected : null;
  const currentWarning = current ? warning(current) : "";
  function closeSearch() {
    setOpen(false);
    triggerRef.current?.focus();
  }
  return <div role="group" aria-label={label} className="min-w-0 space-y-space-2">
    <div className="flex gap-space-2">
      <Button ref={triggerRef} disabled={disabled} aria-expanded={open} aria-controls={id} className="!h-auto min-h-[var(--control-height-md)] min-w-0 flex-1 justify-start whitespace-normal break-words py-[calc((var(--control-height-md)_-_1.25rem_-_2px)/2)] text-left" onClick={() => setOpen(!open)}>
        {value > 0 ? (current ? describe(current) : fallback ?? `已选资料 #${value}`) : `选择${label}`}
      </Button>
      {value > 0 && <Button disabled={disabled} size="sm" aria-label={`清除${label}`} onClick={() => { onChange(null); setOpen(false); }}>清除</Button>}
    </div>
    {selectionLoading && <p role="status" className="text-sm text-text-tertiary">读取已选资料…</p>}
    {currentWarning && <p className="text-sm text-warning">{currentWarning}</p>}
    {selectionError && <div role="alert" className="text-sm text-error">{selectionError} · 保留原选择 <Button size="sm" onClick={() => setRetry(retry + 1)}>重试读取</Button></div>}
    {open && !disabled && <div ref={panelRef} id={id} popover="auto" role="region" aria-label={`${label}搜索面板`} className="fixed inset-auto !m-0 flex flex-col gap-space-2 overflow-hidden rounded-control border border-border bg-surface p-space-2 text-text-primary shadow-floating" onToggle={event => {
      if (event.newState === "closed") setOpen(false);
    }} onBlur={event => {
      if (event.relatedTarget instanceof Node && !event.currentTarget.contains(event.relatedTarget)) setOpen(false);
    }} onKeyDown={event => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); closeSearch(); }
      // Searching inside a draft must not submit its parent form on Enter.
      if (event.key === "Enter" && event.target === searchRef.current) event.preventDefault();
    }}>
      <Input ref={searchRef} className="shrink-0" disabled={disabled} value={keyword} aria-label={`搜索${label}`} placeholder="输入关键词搜索" onChange={event => { setKeyword(event.target.value); setPage(1); }} />
      {loading && <p role="status" className="text-sm text-text-tertiary">搜索中…</p>}
      {error && <div role="alert" className="text-sm text-error">{error} <Button size="sm" onClick={() => setRetry(retry + 1)}>重试搜索</Button></div>}
      {!loading && !error && records.length === 0 && <p role="status" className="text-sm text-text-tertiary">没有匹配资料</p>}
      <div className="min-h-0 space-y-space-1 overflow-y-auto overscroll-contain">
        {!loading && !error && records.map(record => <Button key={record.id} disabled={disabled || Boolean(warning(record))} aria-pressed={record.id === value} className="!h-auto min-h-9 w-full justify-start whitespace-normal break-words py-space-2 text-left" onClick={() => { setSelected(record); onChange(record); closeSearch(); }}>
          {describe(record)}
        </Button>)}
      </div>
      <Pagination compact className="shrink-0" page={page} pageSize={PAGE_SIZE} total={total} disabled={loading || Boolean(error) || disabled} onPageChange={setPage} />
      <Button size="sm" className="self-start" onClick={closeSearch}>收起搜索</Button>
    </div>}
  </div>;
}
