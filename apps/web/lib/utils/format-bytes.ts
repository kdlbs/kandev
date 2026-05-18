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
