import { useLayoutEffect, useRef, useState } from "react";

// Start with the established row limit, then measure the actual table rows.
// Reserve the last-page totals/signatures on every sheet so moving a long row
// to the next page never displaces the final confirmation block or footer.
export function usePrintPages<T>(items: T[], maxRows: number, measurementKey?: unknown) {
  const ref = useRef<HTMLDivElement>(null);
  const [layout, setLayout] = useState<{ items: T[]; counts: number[] } | null>(null);
  const counts = layout?.items === items ? layout.counts : [];
  const pages: T[][] = [];
  let start = 0;
  while (start < items.length) {
    const count = counts[pages.length] ?? maxRows;
    pages.push(items.slice(start, start + count));
    start += count;
  }
  if (!pages.length) pages.push([]);

  useLayoutEffect(() => {
    const root = ref.current;
    if (!root) return;
    function measure() {
      if (!root || !items.length) return;
      const sheets = Array.from(root.querySelectorAll<HTMLElement>(".delivery-page"));
      const first = sheets[0];
      const rows = Array.from(root.querySelectorAll<HTMLElement>("tbody tr"));
      if (!first || rows.length !== items.length) return;
      const style = getComputedStyle(first);
      const tail = root.querySelector<HTMLElement>(".delivery-tail");
      const tailSpace = tail ? tail.getBoundingClientRect().height + parseFloat(getComputedStyle(tail).marginTop) : 0;
      const foot = first.querySelector<HTMLElement>(".delivery-foot")!;
      const tableTop = Math.max(...sheets.map(sheet =>
        sheet.querySelector("tbody")!.getBoundingClientRect().top - sheet.getBoundingClientRect().top));
      const available = parseFloat(style.minHeight) - parseFloat(style.paddingBottom)
        - tableTop - foot.getBoundingClientRect().height - tailSpace - 12;
      const next: number[] = [];
      let height = 0, count = 0;
      for (const row of rows) {
        const rowHeight = row.getBoundingClientRect().height;
        if (count && (count === maxRows || height + rowHeight > available)) {
          next.push(count); count = 0; height = 0;
        }
        count++; height += rowHeight;
      }
      if (count) next.push(count);
      setLayout(previous => previous?.items === items && previous.counts.join() === next.join()
        ? previous : { items, counts: next });
    }
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(root);
    let active = true;
    void document.fonts.ready.then(() => { if (active) measure(); });
    return () => { active = false; observer.disconnect(); };
  }, [items, maxRows, layout, measurementKey]);

  return { ref, pages };
}
