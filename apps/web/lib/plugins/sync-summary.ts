/**
 * Formats a POST /api/plugins/sync SyncResult into the one-line toast
 * summary the Plugins settings page shows after a sync
 * (docs/specs/plugins/spec.md "Filesystem sideloading & sync"). Errors are
 * surfaced separately (an inline `plugins-sync-errors` region) — they do
 * not affect this summary line.
 */
import type { SyncResult } from "@/lib/types/plugins";

export function summarizeSyncResult(result: SyncResult): string {
  const parts: string[] = [];
  if (result.added.length > 0) parts.push(`${result.added.length} sideloaded`);
  if (result.installed.length > 0) parts.push(`${result.installed.length} installed`);
  if (result.missing.length > 0) parts.push(`${result.missing.length} missing`);

  if (parts.length === 0) return "Everything up to date";
  return `Sync: ${parts.join(", ")}`;
}
