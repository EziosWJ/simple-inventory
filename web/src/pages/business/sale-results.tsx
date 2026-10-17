import type { ReactNode } from "react";
import type { DataTableColumn } from "@/types";
import type { SaleDraft, SaleItemRecord, SaleItemPage } from "@/api/business";
import { DataTableCard } from "@/components/common/data-table-card";
import { DataTable } from "@/components/common/data-table";
import { Pagination } from "@/components/common/pagination";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { ItemSummaryPanel, ViewSwitch } from "./sale-query-views";
type Controls = {
    exportAction: ReactNode;
    pageSize: number;
    switchView: (view: "document" | "item") => void;
    onRefresh: () => void;
    changePageSize: (size: number) => void;
};
const pageSizes = [10, 20, 50];
export function SaleDocumentView({ page, pageSize, total, records, loading, error, columns, expanded, renderExpandedRow, setPage, switchView, onRefresh, changePageSize, exportAction }: Controls & {
    page: number;
    total: number;
    records: SaleDraft[];
    loading: boolean;
    error: string;
    columns: DataTableColumn<SaleDraft>[];
    expanded: string[];
    renderExpandedRow: (row: SaleDraft) => ReactNode;
    setPage: (page: number) => void;
}) {
    return (<DataTableCard toolbar={<TableToolbar title="销售出库单" description="按单号、客户、商品/服务、状态和业务日期组合查询；商品筛选时整单展示并突出匹配商品。" actions={<div className="flex flex-wrap items-center gap-space-2">{exportAction}<ViewSwitch view="document" onChange={switchView}/><Button size="sm" variant="secondary" onClick={onRefresh}>刷新</Button></div>}/>} pagination={<Pagination page={page} pageSize={pageSize} pageSizeOptions={pageSizes} total={total} disabled={loading} onPageChange={setPage} onPageSizeChange={changePageSize}/>}>
        <DataTable<SaleDraft> columns={columns} dataSource={records} loading={loading} error={error} rowKey="id" minWidth={1040} empty="暂无销售单" expandedRowKeys={expanded} renderExpandedRow={renderExpandedRow}/>
      </DataTableCard>);
}
export function SaleItemView({ itemPage, pageSize, itemResult, itemLoading, itemError, columns, setItemPage, switchView, onRefresh, changePageSize, exportAction }: Controls & {
    itemPage: number;
    itemResult: SaleItemPage | null;
    itemLoading: boolean;
    itemError: string;
    columns: DataTableColumn<SaleItemRecord>[];
    setItemPage: (page: number) => void;
}) {
    return (<DataTableCard toolbar={<TableToolbar title="销售明细" description="跨单查看销售对象、数量与成交价；合计覆盖全部筛选结果而不是当前页。" actions={<div className="flex flex-wrap items-center gap-space-2">{exportAction}<ViewSwitch view="item" onChange={switchView}/><Button size="sm" variant="secondary" onClick={onRefresh}>刷新</Button></div>}/>} pagination={<Pagination page={itemPage} pageSize={pageSize} pageSizeOptions={pageSizes} total={itemResult?.total ?? 0} disabled={itemLoading} onPageChange={setItemPage} onPageSizeChange={changePageSize}/>}>
        <ItemSummaryPanel summary={itemResult?.summary} loading={itemLoading} error={itemError}/>
        <DataTable<SaleItemRecord> columns={columns} dataSource={itemResult?.records ?? []} loading={itemLoading} error={itemError} rowKey="id" minWidth={1240} empty="没有匹配的销售明细"/>
      </DataTableCard>);
}
