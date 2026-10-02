import { useEffect, useId, useRef, useState } from "react";
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

  useEffect(() => {
    if (!open) return;
    searchRef.current?.focus();
  }, [open]);

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
      <Button ref={triggerRef} disabled={disabled} aria-expanded={open} aria-controls={id} className="!h-auto min-h-9 min-w-0 flex-1 justify-start whitespace-normal break-words py-[7px] text-left" onClick={() => setOpen(!open)}>
        {value > 0 ? (current ? describe(current) : fallback ?? `已选资料 #${value}`) : `选择${label}`}
      </Button>
      {value > 0 && <Button disabled={disabled} size="sm" aria-label={`清除${label}`} onClick={() => { onChange(null); setOpen(false); }}>清除</Button>}
    </div>
    {selectionLoading && <p role="status" className="text-sm text-text-tertiary">读取已选资料…</p>}
    {currentWarning && <p className="text-sm text-warning">{currentWarning}</p>}
    {selectionError && <div role="alert" className="text-sm text-error">{selectionError} · 保留原选择 <Button size="sm" onClick={() => setRetry(retry + 1)}>重试读取</Button></div>}
    {open && <div id={id} className="space-y-space-2 rounded-control border border-border bg-surface p-space-2" onKeyDown={event => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); closeSearch(); }
      // Searching inside a draft must not submit its parent form on Enter.
      if (event.key === "Enter" && event.target === searchRef.current) event.preventDefault();
    }}>
      <Input ref={searchRef} disabled={disabled} value={keyword} aria-label={`搜索${label}`} placeholder="输入关键词搜索" onChange={event => { setKeyword(event.target.value); setPage(1); }} />
      {loading && <p role="status" className="text-sm text-text-tertiary">搜索中…</p>}
      {error && <div role="alert" className="text-sm text-error">{error} <Button size="sm" onClick={() => setRetry(retry + 1)}>重试搜索</Button></div>}
      {!loading && !error && records.length === 0 && <p role="status" className="text-sm text-text-tertiary">没有匹配资料</p>}
      <div className="max-h-64 space-y-space-1 overflow-y-auto">
        {!loading && !error && records.map(record => <Button key={record.id} disabled={disabled || Boolean(warning(record))} aria-pressed={record.id === value} className="!h-auto min-h-9 w-full justify-start whitespace-normal break-words py-space-2 text-left" onClick={() => { setSelected(record); onChange(record); closeSearch(); }}>
          {describe(record)}
        </Button>)}
      </div>
      <Pagination compact page={page} pageSize={PAGE_SIZE} total={total} disabled={loading || Boolean(error) || disabled} onPageChange={setPage} />
      <Button size="sm" onClick={closeSearch}>收起搜索</Button>
    </div>}
  </div>;
}
