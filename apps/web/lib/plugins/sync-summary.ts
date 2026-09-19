/**
 * Formats a POST /api/plugins/sync SyncResult into the one-line toast
 * summary the Plugins settings page shows after a sync
 * (docs/specs/plugins/requirements/plugins.md "Filesystem sideloading & sync"). Errors are
 * surfaced separately (an inline `plugins-sync-errors` region) — they do
 * not affect this summary line.
 */
import { t } from "@/lib/i18n";
import type { SyncResult } from "@/lib/types/plugins";

export function summarizeSyncResult(result: SyncResult): string {
  const parts: string[] = [];
  if (result.added.length > 0)
    parts.push(t("plugins:syncSideloaded", { count: result.added.length }));
  if (result.installed.length > 0)
    parts.push(t("plugins:syncInstalled", { count: result.installed.length }));
  if (result.missing.length > 0)
    parts.push(t("plugins:syncMissing", { count: result.missing.length }));

  if (parts.length === 0) return t("plugins:syncUpToDate");
  return t("plugins:syncSummary", { parts: parts.join(", ") });
}
