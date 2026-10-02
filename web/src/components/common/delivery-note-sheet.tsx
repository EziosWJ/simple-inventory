import type { DeliveryNote } from "@/api/business";
import { useEffect } from "react";
import { DELIVERY_NOTE_ROWS_PER_PAGE } from "@/lib/delivery-note";
import { usePrintPages } from "@/lib/use-print-pages";
import { PrintHeading } from "@/components/common/print-heading";

// DeliveryNoteSheet renders the printable part of a delivery note. It is split
// out so the browser acceptance test can render a saved document without the
// authenticated data path, and so the page itself stays a thin loader.
export function DeliveryNoteSheet({ note, showAmount, onPageCountChange }: { note: DeliveryNote; showAmount: boolean; onPageCountChange?: (count: number) => void }) {
  const { ref, pages } = usePrintPages(note.items, DELIVERY_NOTE_ROWS_PER_PAGE, showAmount);
  useEffect(() => { onPageCountChange?.(pages.length); }, [onPageCountChange, pages.length]);

  return (
    <div className="delivery-note-print" ref={ref}>
      {pages.map((rows, index) => (
        <section className="delivery-page" key={index}>
          {!note.posted && (
            <div className="delivery-watermark" aria-hidden="true">
              {note.status === "CANCELLED" ? "已取消" : "未过账"}
            </div>
          )}
          <PrintHeading title="送货单" name={note.ownerName} phone={note.ownerPhone} address={note.ownerAddress} />
          <div className="delivery-meta">
            <div className="delivery-meta-column">
              <span className="delivery-document-no">单号：{note.documentNo}</span>
              <span>业务日期：{note.businessDate.slice(0, 10)}</span>
            </div>
            <div className="delivery-meta-column">
              <span>客户名称：{note.partnerName}</span>
              <span>收货人：{note.deliveryContact ?? ""}</span>
              <span>收货人电话：{note.deliveryPhone ?? ""}</span>
              <span>送货地址：{note.deliveryAddress ?? ""}</span>
            </div>
          </div>
          <table className="delivery-table">
            <thead>
              <tr>
                <th className="col-index">序号</th>
                <th>商品名称</th>
                <th>型号</th>
                <th>规格</th>
                <th className="col-unit">单位</th>
                <th className="col-qty">数量</th>
                {showAmount && <><th className="col-money">单价</th><th className="col-money">金额</th></>}
                <th>备注</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((line, i) => (
                <tr key={`${index}-${i}`}>
                  <td className="col-index">{pages.slice(0, index).reduce((sum, page) => sum + page.length, 0) + i + 1}</td>
                  <td>{line.productName}</td>
                  <td>{line.productModel ?? ""}</td>
                  <td>{line.productSpecification ?? ""}</td>
                  <td className="col-unit">{line.unit}</td>
                  <td className="col-qty">{line.quantity}</td>
                  {showAmount && <><td className="col-money">¥{line.unitPrice}</td><td className="col-money">¥{line.amount}</td></>}
                  <td>{line.remark ?? ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {index === pages.length - 1 && (
            <div className="delivery-tail">
              <p className="delivery-total">
                合计数量：{note.totalQuantity}
                {showAmount && <> · 合计金额：¥{note.totalAmount}</>}
              </p>
              {note.remark && <p className="delivery-remark">备注：{note.remark}</p>}
              <p className="delivery-receipt-notice">收货人签字即代表货物数量无误且外观完好</p>
              <div className="delivery-sign">
                <span>送货人签字：____________</span>
                <span>收货人签字：____________</span>
                <span>收货日期：____年__月__日</span>
              </div>
            </div>
          )}
          <footer className="delivery-foot">
            单号 {note.documentNo} · 第 {index + 1} / {pages.length} 页
          </footer>
        </section>
      ))}
    </div>
  );
}
