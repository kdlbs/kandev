/**
 * Human-readable byte size formatter. Uses 1024-based units (B, KB, MB, GB, TB)
 * with one fractional digit for KB+ and integer bytes for the smallest unit.
 *
 * This is the shared helper for System pages (disk usage, database stats,
 * backups, logs). Other call sites have their own local helpers for
 * historical reasons; new code should import this one.
 */
export function formatBytes(bytes: number | null | undefined): string {
  if (bytes == null || !Number.isFinite(bytes)) return "-";
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  if (i === 0) return `${bytes} B`;
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(1)} ${units[i]}`;
}

/**
 * Formats two byte counts for side-by-side display (e.g. "X, over the Y
 * limit"), guaranteeing they never render as the same string when the
 * underlying values differ. `formatBytes`'s one-decimal rounding collapses
 * any two values within roughly the same 1/10-unit bucket to identical text
 * (e.g. 262,144 and 262,150 both read "256.0 KB"), which is exactly the
 * boundary a size-ceiling rejection needs to be legible at. Falls back to
 * exact byte counts for both numbers only when the rounded forms would
 * otherwise collide.
 */
export function formatDistinctByteSizes(a: number, b: number): [string, string] {
  const formattedA = formatBytes(a);
  const formattedB = formatBytes(b);
  if (a === b || formattedA !== formattedB) return [formattedA, formattedB];
  return [`${a} B`, `${b} B`];
}
