import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { getPartnerStatement, type PartnerStatement } from "@/api/business";
import { getPrintProfile, type PrintProfile } from "@/api/print-profile";
import { PageHeader } from "@/components/common/page-header";
import { PrintHeading } from "@/components/common/print-heading";
import { Button } from "@/components/ui/button";
import { entryTypeLabel } from "@/lib/partner-ledger";
import { usePrintPages } from "@/lib/use-print-pages";
import "./delivery-note.css";
import "./partner-statement.css";

const dateTime = (value: string) => new Date(value).toLocaleString("zh-CN", { hour12: false });

export function PartnerStatementSheet({ statement, profile, onPageCountChange }: {
  statement: PartnerStatement; profile: PrintProfile; onPageCountChange?: (count: number) => void;
}) {
  const { ref, pages } = usePrintPages(statement.records, 12, `${profile.name}:${profile.phone}`);
  useEffect(() => { onPageCountChange?.(pages.length); }, [onPageCountChange, pages.length]);
  const direction = statement.direction === "CUSTOMER" ? "客户（应收）" : "供应商（应付）";
  const identifier = `ST-${statement.partnerId}-${statement.direction === "CUSTOMER" ? "C" : "S"}`;
  return (
    <div className="delivery-note-print partner-statement-print" ref={ref}>
      {pages.map((records, index) => (
        <section className="delivery-page statement-page" key={index} aria-label={`往来对账单第${index + 1}页`}>
          <PrintHeading title="往来对账单" name={profile.name} phone={profile.phone} />
          <div className="delivery-meta statement-meta">
            <div className="delivery-meta-column">
              <span>对账标识：{identifier}</span>
              <span>对账方向：{direction}</span>
              <span>期间期初余额：{statement.openingAmount} 元</span>
            </div>
            <div className="delivery-meta-column">
              <strong>往来单位：{statement.partnerName}</strong>
              <span>期间起点：{dateTime(statement.from)}（含）</span>
              <span>期间终点：{dateTime(statement.to)}（不含）</span>
            </div>
          </div>
          <p className="statement-note">按实际生效时间对账；币种：人民币。正余额为欠款，负余额为待退款。己方抬头使用本次打印时的经营者资料。</p>
          <table className="delivery-table statement-table">
            <colgroup><col style={{ width: "6%" }} /><col style={{ width: "13%" }} /><col style={{ width: "17%" }} /><col style={{ width: "36%" }} /><col style={{ width: "14%" }} /><col style={{ width: "14%" }} /></colgroup>
            <thead><tr><th>序号</th><th>业务日期</th><th>实际生效</th><th>类型 / 来源单号</th><th>变动金额</th><th>变动后余额</th></tr></thead>
            <tbody>
              {records.map((record, i) => (
                <tr key={record.id}>
                  <td>{pages.slice(0, index).reduce((sum, page) => sum + page.length, 0) + i + 1}</td>
                  <td>{record.businessDate.slice(0, 10)}</td>
                  <td>{dateTime(record.effectiveAt)}</td>
                  <td>
                    <span>{entryTypeLabel[record.entryType] ?? record.entryType}</span>
                    <span className="statement-source-no">{record.documentNo}</span>
                    {record.reversedDocumentNo && <span className="statement-source-no">原：{record.reversedDocumentNo}</span>}
                  </td>
                  <td className="statement-money">{record.amount}</td>
                  <td className="statement-money">{record.balanceAfter}</td>
                </tr>
              ))}
              {!statement.records.length && <tr><td colSpan={6}>期间无金额变动</td></tr>}
            </tbody>
          </table>
          {index === pages.length - 1 && (
            <div className="delivery-tail statement-totals">
              <div className="statement-summary">
                <span>期间增加：{statement.increaseAmount} 元</span><span>期间减少：{statement.decreaseAmount} 元</span>
                <span>期间净变动：{statement.netChange} 元</span><strong>期末余额：{statement.closingAmount} 元</strong>
              </div>
              <div className="delivery-sign statement-check">
                <span>己方确认：____________</span><span>对方确认：____________</span><span>确认日期：____年__月__日</span>
              </div>
            </div>
          )}
          <footer className="delivery-foot">{identifier} · 第 {index + 1} / {pages.length} 页</footer>
        </section>
      ))}
    </div>
  );
}

export function PartnerStatementPage() {
  const [params] = useSearchParams();
  const partnerId = params.get("partnerId") ?? "", direction = params.get("direction") ?? "";
  const from = params.get("from") ?? "", to = params.get("to") ?? "";
  const [data, setData] = useState<{ statement: PartnerStatement; profile: PrintProfile } | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [pageCount, setPageCount] = useState(0);
  const load = useCallback(() => {
    let active = true;
    setLoading(true); setData(null); setError("");
    void Promise.all([getPartnerStatement({ partnerId, direction, from, to }), getPrintProfile()])
      .then(([statement, profile]) => { if (active) setData({ statement, profile }); })
      .catch((reason: unknown) => { if (active) setError(reason instanceof Error ? reason.message : "读取对账单或经营者资料失败"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [partnerId, direction, from, to]);
  useEffect(load, [load]);
  return (
    <div className="space-y-space-4">
      <div className="print-hide">
        <PageHeader title="往来对账单 / A4打印" description="完整期间数据来自服务器。生效时间含起点、不含终点；己方抬头使用本次打印时的经营者资料。" actions={<Button disabled={!data || loading} onClick={() => window.print()}>打印 A4</Button>} />
        {error && <p role="alert" className="text-error">{error}</p>}
        {loading && <p role="status">正在读取完整期间及经营者资料…</p>}
        {data && <p className="text-sm text-text-secondary">完整期间共 {data.statement.records.length} 笔 / {pageCount} 页。正余额：欠款；负余额：待退款。</p>}
      </div>
      {data && <PartnerStatementSheet {...data} onPageCountChange={setPageCount} />}
    </div>
  );
}
