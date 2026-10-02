import type { DeliveryNoteLine } from "@/api/business";

// Upper bound per printed A4 page. The print sheet also measures row heights
// and reduces the count when long content needs more room.
export const DELIVERY_NOTE_ROWS_PER_PAGE = 18;

// paginateLines splits delivery note lines into printable A4 pages. An empty
// document still yields one page so the header, signatures and totals print.
export function paginateLines(lines: DeliveryNoteLine[], perPage = DELIVERY_NOTE_ROWS_PER_PAGE): DeliveryNoteLine[][] {
  if (lines.length === 0) return [[]];
  const pages: DeliveryNoteLine[][] = [];
  for (let i = 0; i < lines.length; i += perPage) pages.push(lines.slice(i, i + perPage));
  return pages;
}
