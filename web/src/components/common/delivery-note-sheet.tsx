import type { DeliveryNote } from "@/api/business";
import { DELIVERY_NOTE_ROWS_PER_PAGE, paginateLines } from "@/lib/delivery-note";

// DeliveryNoteSheet renders the printable part of a delivery note. It is split
// out so the browser acceptance test can render a saved document without the
// authenticated data path, and so the page itself stays a thin loader.
export function DeliveryNoteSheet({ note, showAmount }: { note: DeliveryNote; showAmount: boolean }) {
  const pages = paginateLines(note.items);

  return (
    <div className="delivery-note-print">
      {pages.map((rows, index) => (
        <section className="delivery-page" key={index}>
          {!note.posted && (
            <div className="delivery-watermark" aria-hidden="true">
              {note.status === "CANCELLED" ? "已取消" : "未过账"}
            </div>
          )}
          <header className="delivery-head">
            <div className="delivery-owner">
              <strong>{note.ownerName}</strong>
              <span>{note.ownerPhone}</span>
              <span>{note.ownerAddress}</span>
            </div>
            <h2 className="delivery-title">送&nbsp;货&nbsp;单</h2>
          </header>
          <div className="delivery-meta">
            <span>单号：{note.documentNo}</span>
            <span>业务日期：{note.businessDate.slice(0, 10)}</span>
            <span>客户：{note.partnerName}</span>
            <span>联系人：{note.deliveryContact ?? ""}</span>
            <span>电话：{note.deliveryPhone ?? ""}</span>
            <span>送货地址：{note.deliveryAddress ?? ""}</span>
          </div>
          <table className="delivery-table">
            <thead>
              <tr>
                <th className="col-index">序号</th>
                <th>商品名称</th>
                <th>型号</th>
                <th>规格</th>
                <th>单位</th>
                <th className="col-qty">数量</th>
                {showAmount && <><th className="col-money">单价</th><th className="col-money">金额</th></>}
                <th>备注</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((line, i) => (
                <tr key={`${index}-${i}`}>
                  <td className="col-index">{index * DELIVERY_NOTE_ROWS_PER_PAGE + i + 1}</td>
                  <td>{line.productName}</td>
                  <td>{line.productModel ?? ""}</td>
                  <td>{line.productSpecification ?? ""}</td>
                  <td>{line.unit}</td>
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
              <div className="delivery-sign">
                <span>送货人（签字）：______________</span>
                <span>收货人（签字）：______________</span>
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
