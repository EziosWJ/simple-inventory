const beijingFormatter = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit",
  hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23",
});

function beijingParts(date: Date) {
  if (!Number.isFinite(date.getTime())) return null;
  return Object.fromEntries(beijingFormatter.formatToParts(date).map(part => [part.type, part.value]));
}

function parseTimestamp(value: string) {
  const normalized = value.trim().replace(" ", "T");
  // API timestamps are UTC; legacy timestamps without an offset also represent UTC.
  return new Date(/(?:Z|[+-]\d{2}:?\d{2})$/i.test(normalized) ? normalized : `${normalized}Z`);
}

export function formatDateOnly(value?: string | null) {
  if (!value?.trim()) return "-";
  // Business dates are calendar values, not instants.
  if (/^\d{4}-\d{2}-\d{2}$/.test(value.trim())) return value.trim();
  const parts = beijingParts(parseTimestamp(value));
  return parts ? `${parts.year}-${parts.month}-${parts.day}` : "-";
}

export function formatDateTime(value?: string | null) {
  if (!value?.trim()) return "-";
  const parts = beijingParts(parseTimestamp(value));
  return parts ? `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}` : "-";
}

export function businessToday(now = new Date()) {
  const parts = beijingParts(now);
  return parts ? `${parts.year}-${parts.month}-${parts.day}` : "";
}

// The "this month" shortcut follows the same Beijing business calendar as the
// rest of the app, so a document saved in the evening belongs to the month the
// operator is working in, including when the host clock is on another day.
export function businessMonthRange(now = new Date()) {
  const today = businessToday(now);
  if (!today) return { from: "", to: "" };
  const [year, month] = today.split("-");
  const lastDay = new Date(Date.UTC(Number(year), Number(month), 0)).getUTCDate();
  return { from: `${year}-${month}-01`, to: `${year}-${month}-${String(lastDay).padStart(2, "0")}` };
}

export function businessMidnightToUtc(date: string, nextDay = false) {
  const midnight = new Date(`${date}T00:00:00+08:00`);
  if (nextDay) midnight.setUTCDate(midnight.getUTCDate() + 1);
  return midnight.toISOString();
}
