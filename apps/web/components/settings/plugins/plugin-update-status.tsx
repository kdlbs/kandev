"use client";

import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";

type PluginUpdateStatusProps = {
  checking: boolean;
  lastCheckedAt: string | null;
  error: string | null;
};

/**
 * The header status strip beside the Sync/Install toolbar: explains that
 * Sync now also checks the marketplace (self-documenting settings — the
 * button alone doesn't say that), and reports the outcome of the last check.
 * A check failure never blocks the plugin list; it only shows here.
 */
export function PluginUpdateStatus({ checking, lastCheckedAt, error }: PluginUpdateStatusProps) {
  const { t } = useTranslation();

  return (
    <div className="space-y-1 text-xs text-muted-foreground">
      <p>{t("plugins:syncAndCheckDescription")}</p>
      {checking && <p data-testid="plugins-updates-checking">{t("plugins:checkingForUpdates")}</p>}
      {!checking && lastCheckedAt && (
        <p data-testid="plugins-updates-last-checked">
          {t("plugins:updatesLastChecked", { time: formatDateTime(lastCheckedAt) })}
        </p>
      )}
      {error && (
        <div
          role="alert"
          data-testid="plugins-update-check-error"
          className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-destructive [overflow-wrap:anywhere]"
        >
          {error}
        </div>
      )}
    </div>
  );
}
