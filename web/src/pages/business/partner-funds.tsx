import { PartnerEntryPage } from "./partner-entry-page";

export function PartnerFundsPage({ kind }: { kind: "SETTLEMENT" | "REFUND" }) {
  return <PartnerEntryPage key={kind} category={kind} />;
}
