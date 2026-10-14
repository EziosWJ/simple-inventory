import { useEffect, useState } from "react";
import { partnerBalanceEntries, type PartnerBalanceEntry } from "@/api/business";
import { DataTable } from "@/components/common/data-table";
import { Pagination } from "@/components/common/pagination";
import { Button } from "@/components/ui/button";
import { balanceDescription, cents, entryTypeLabel, money } from "@/lib/partner-ledger";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";

// Mounted only after the user expands the selected partner's ledger.
export function PartnerEntryLedger({ partnerId, direction }: { partnerId: string; direction: "CUSTOMER" | "SUPPLIER" }) {
  const [page, setPage] = useState(1);
  const [records, setRecords] = useState<PartnerBalanceEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true); setError("");
    void partnerBalanceEntries({ partnerId, direction, page, pageSize: 10 }).then(result => {
      if (active) { setRecords(result.records); setTotal(result.total); }
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : "读取往来明细失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [partnerId, direction, page, revision]);
  const columns: DataTableColumn<PartnerBalanceEntry>[] = [
    { title: "业务／单号", key: "source", render: (_, row) => <div><span>{entryTypeLabel[row.entryType] ?? row.entryType}</span><span className="mt-1 block text-xs text-text-tertiary">{row.documentNo}</span></div> },
    { title: "业务日期／生效时间", key: "date", render: (_, row) => <div>{row.businessDate}<span className="mt-1 block text-xs text-text-tertiary">{formatDateTime(row.effectiveAt)}</span></div> },
    { title: "变动（元）", key: "amount", align: "right", render: (_, row) => { const value = cents(row.amount); return <span className="font-medium tabular-nums">{value === null ? "—" : `${value > 0n ? "+" : ""}${money(value)}`}</span>; } },
    { title: "本笔生效后", key: "after", render: (_, row) => balanceDescription(row.direction, row.balanceAfter) },
  ];
  return <section id="entry-ledger" aria-label="当前单位往来明细" className="mt-3 rounded-control border border-border bg-surface">
    <p className="border-b border-border px-3 py-2 text-xs text-text-secondary">全部业务 · 正变动增加该方向余额，负变动减少余额；此处余额为每笔生效时的记录。</p>
    <div className="max-h-72 overflow-auto"><DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} minWidth={540}
      error={error ? <div role="alert" className="space-y-2"><p className="text-error">{error}</p><Button size="sm" onClick={() => setRevision(v => v + 1)}>重试明细</Button></div> : undefined}
      empty={<p className="text-text-secondary">该单位在此方向尚无往来记录。</p>} /></div>
    {!error && <div className="border-t border-border px-3 py-2"><Pagination page={page} pageSize={10} total={total} disabled={loading} onPageChange={setPage} /></div>}
  </section>;
}
