import { useState } from "react";
import { createRoot } from "react-dom/client";
import { DeliveryNoteSheet } from "@/components/common/delivery-note-sheet";
import type { DeliveryNote } from "@/api/business";
import "@/pages/business/delivery-note.css";
import "@/styles/globals.css";

function fixedLines(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    productId: i + 1,
    productCode: `G${i + 1}`,
    productName: `商品${i + 1}`,
    productModel: i % 2 === 0 ? null : `M${i + 1}`,
    productSpecification: `S${i + 1}`,
    unit: "台",
    quantity: "2",
    unitPrice: "1.50",
    amount: "3.00",
    remark: null,
  }));
}

const saved: DeliveryNote = {
  documentNo: "SO20261001-ABC",
  status: "POSTED",
  posted: true,
  businessDate: "2026-10-01",
  partnerId: 1,
  partnerName: "送货客户",
  deliveryContact: "张三",
  deliveryPhone: "10086",
  deliveryAddress: "客户地址",
  ownerName: "个体经营者",
  ownerPhone: "13900000000",
  ownerAddress: "经营者地址",
  remark: null,
  items: fixedLines(22),
  totalQuantity: "44",
  totalAmount: "66.00",
};

const draftNote: DeliveryNote = { ...saved, documentNo: "SO20261001-DRAFT", status: "DRAFT", posted: false };

export function Fixture() {
  const [note, setNote] = useState<DeliveryNote>(saved);
  const [showAmount, setShowAmount] = useState(true);
  return (
    <div>
      <div data-testid="controls" style={{ marginBottom: 12 }}>
        <button data-testid="toggle-amount" onClick={() => setShowAmount((v) => !v)}>切换金额</button>
        <button data-testid="use-draft" onClick={() => setNote(draftNote)}>草稿</button>
        <button data-testid="use-saved" onClick={() => setNote(saved)}>正式</button>
      </div>
      <DeliveryNoteSheet note={note} showAmount={showAmount} />
    </div>
  );
}

createRoot(document.getElementById("root")!).render(<Fixture />);
