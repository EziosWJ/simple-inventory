// Use integer arithmetic with the same per-line rounding as the API.
export function draftLineAmount(quantity: string, price: string): bigint | null {
  if (!/^\d+(\.\d{1,3})?$/.test(quantity) || !/^\d+(\.\d{1,2})?$/.test(price)) return null;
  const fixed = (s: string, scale: number) => { const [whole, fraction = ""] = s.split("."); return BigInt(whole) * 10n ** BigInt(scale) + BigInt(fraction.padEnd(scale, "0")); };
  const q = fixed(quantity, 3), p = fixed(price, 2);
  if (q <= 0n || q > 9223372036854775807n || p > 9223372036854775807n) return null;
  const cents = (q * p + 500n) / 1000n;
  return cents <= 9223372036854775807n ? cents : null;
}
export const draftMoney = (cents: bigint) => `${cents / 100n}.${String(cents % 100n).padStart(2, "0")}`;
