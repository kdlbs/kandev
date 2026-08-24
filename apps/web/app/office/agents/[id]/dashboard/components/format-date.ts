import { formatDate } from "@/lib/i18n/formats";

/**
 * Formatting helpers shared by the dashboard chart cards. Kept as a
 * standalone module so the unit tests can pin the output shape
 * without re-rendering an entire card.
 */

// Weekday and month names come from `Intl` through the shared `formatDate`
// helper rather than a hardcoded English array. Under `en` the output is
// unchanged ("Mon", "Jan"); every other locale now gets its own names instead of
// English ones. `timeZone: "UTC"` preserves the UTC bucketing below.

/**
 * Compact two-line label used under each chart bar. The first line
 * is the day-of-month; the second line is the abbreviated weekday.
 * Returns the raw input string when it doesn't parse as YYYY-MM-DD.
 */
export function formatBarLabel(iso: string): string {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(iso)) return iso;
  // Append T00:00:00Z so the date is interpreted in UTC, matching the
  // SQL bucket the backend produced. A naive `new Date("YYYY-MM-DD")`
  // is already UTC midnight in modern JS, but pinning explicitly keeps
  // server vs. client renders identical.
  const d = new Date(`${iso}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return iso;
  const day = d.getUTCDate();
  const dow = formatDate(d, { weekday: "short", timeZone: "UTC" });
  return `${day} ${dow}`;
}

/**
 * Compact "MMM D" date format used by the cost table and recent-run
 * pills. Returns the raw input when unparseable.
 */
export function formatShortDate(iso: string): string {
  if (!iso) return "";
  const dateOnly = iso.length >= 10 ? iso.slice(0, 10) : iso;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(dateOnly)) return iso;
  const d = new Date(`${dateOnly}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return iso;
  const month = formatDate(d, { month: "short", timeZone: "UTC" });
  return `${month} ${d.getUTCDate()}`;
}

/**
 * Renders an int64 subcents (hundredths of a cent) value as USD. The
 * backend ships every cost figure as a subcents integer so we don't
 * carry float drift; the formatter just divides by 10000 at the edge.
 * See docs/specs/office/requirements/costs.md for the unit contract.
 */
export function formatSubcents(subcents: number): string {
  if (!Number.isFinite(subcents)) return "$0.00";
  return `$${(subcents / 10000).toFixed(2)}`;
}
