"use client";

import { Trans, useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconCheck,
  IconClock,
  IconLoader2,
  IconRefresh,
} from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useTick } from "@/components/integrations/auth-status-banner";
import type { TFunction } from "i18next";
import { formatRelative } from "@/lib/i18n/formats";
import type { OfficeConfigSyncConfig } from "@/lib/types/office-config-sync";

type SyncState = "waiting" | "ok" | "failed";

function syncState(config: OfficeConfigSyncConfig): SyncState {
  if (!config.last_synced_at) return "waiting";
  return config.last_ok ? "ok" : "failed";
}

// syncSourceLabel is the human-readable sync source: "owner/repo" for GitHub,
// the namespace path (already "group/project" shaped) for GitLab.
function syncSourceLabel(config: OfficeConfigSyncConfig): string {
  return config.provider === "gitlab"
    ? config.project_path
    : `${config.repo_owner}/${config.repo_name}`;
}

function StateIcon({ state }: { state: SyncState }) {
  if (state === "ok") return <IconCheck className="h-4 w-4 text-green-600 dark:text-green-400" />;
  if (state === "failed") return <IconAlertTriangle className="h-4 w-4 text-destructive" />;
  return <IconClock className="h-4 w-4 text-muted-foreground" />;
}

function lastSyncedLabel(t: TFunction, config: OfficeConfigSyncConfig): string {
  if (config.last_synced_at) {
    // `formatRelative` routes its buckets through i18next; date-fns'
    // `formatDistanceToNow` would render English inside a translated sentence.
    const when = formatRelative(config.last_synced_at);
    return config.last_ok
      ? t("office:configSyncLastSynced", { when })
      : t("office:configSyncLastAttempt", { when });
  }
  return config.poll_enabled
    ? t("office:configSyncWaitingForFirstSync")
    : t("office:configSyncNotSyncedYet");
}

function MetadataLine({ config }: { config: OfficeConfigSyncConfig }) {
  const { t } = useTranslation();
  useTick(30_000);
  const parts = [
    t("office:configSyncDirectoryLine", {
      path: config.path || t("office:configSyncRepositoryRoot"),
    }),
    config.poll_enabled
      ? t("office:configSyncEverySeconds", { count: config.interval_seconds })
      : t("office:configSyncAutoSyncOff"),
    lastSyncedLabel(t, config),
  ];
  return <p className="text-xs text-muted-foreground">{parts.join(" · ")}</p>;
}

// MAX_VISIBLE_WARNINGS caps the rendered warning list (AC-OFFICE-CONFIG-SYNC-006.3):
// at most the first 10, in the recorded order, with a count of any beyond
// that rather than an unbounded list.
const MAX_VISIBLE_WARNINGS = 10;

function WarningsAlert({ warnings }: { warnings: string[] }) {
  const { t } = useTranslation();
  if (warnings.length === 0) return null;
  const visible = warnings.slice(0, MAX_VISIBLE_WARNINGS);
  const remainder = warnings.length - visible.length;
  return (
    <Alert
      data-testid="office-config-sync-warnings"
      className="border-amber-500/40 bg-amber-500/10 dark:border-amber-400/30 dark:bg-amber-400/10"
    >
      <IconAlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400" />
      <AlertDescription className="text-sm">
        <ul className="list-disc pl-4 space-y-0.5">
          {visible.map((warning, index) => (
            // Warnings are free-form backend sentences with no stable id;
            // include the index so repeated sentences keep unique keys.
            <li key={`${index}-${warning}`}>{warning}</li>
          ))}
        </ul>
        {remainder > 0 && (
          <p className="mt-1 text-xs text-muted-foreground">
            {t("office:configSyncWarningsRemainder", { count: remainder })}
          </p>
        )}
      </AlertDescription>
    </Alert>
  );
}

type OfficeConfigSyncStatusCardProps = {
  config: OfficeConfigSyncConfig;
  syncing: boolean;
  onSyncNow: () => void;
};

// OfficeConfigSyncStatusCard is the compact always-visible summary of an
// active Office config sync source: headline with state icon, repo and
// branch, a muted metadata line, the last error when failing, and any
// warnings from the most recent attempt — warnings can be present even when
// last_ok is true (e.g. one file failed to parse but the rest synced).
export function OfficeConfigSyncStatusCard({
  config,
  syncing,
  onSyncNow,
}: OfficeConfigSyncStatusCardProps) {
  const { t } = useTranslation();
  const state = syncState(config);
  return (
    <div
      className="rounded-lg border bg-card p-4 space-y-2"
      data-testid="office-config-sync-status"
      data-state={state}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm">
          <StateIcon state={state} />
          <span>
            <Trans
              i18nKey="office:configSyncSyncingFromRepository"
              values={{ repository: syncSourceLabel(config) }}
            >
              Syncing from <span className="font-semibold" />
            </Trans>
          </span>
          <Badge variant="secondary">{config.branch}</Badge>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onSyncNow}
          disabled={syncing}
          className="cursor-pointer"
          data-testid="office-config-sync-now"
        >
          {syncing ? (
            <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
          ) : (
            <IconRefresh className="h-4 w-4 mr-2" />
          )}
          {t("office:configSyncSyncNow")}
        </Button>
      </div>
      <MetadataLine config={config} />
      {state === "failed" && (
        <p className="text-xs text-destructive">
          {config.last_error || t("office:configSyncSyncFailed")}
        </p>
      )}
      <WarningsAlert warnings={config.last_warnings ?? []} />
    </div>
  );
}
