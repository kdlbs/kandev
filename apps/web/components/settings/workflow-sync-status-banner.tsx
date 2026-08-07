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
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";

type SyncState = "waiting" | "ok" | "failed";

function syncState(config: WorkflowSyncConfig): SyncState {
  if (!config.last_synced_at) return "waiting";
  return config.last_ok ? "ok" : "failed";
}

function StateIcon({ state }: { state: SyncState }) {
  if (state === "ok") return <IconCheck className="h-4 w-4 text-green-600 dark:text-green-400" />;
  if (state === "failed") return <IconAlertTriangle className="h-4 w-4 text-destructive" />;
  return <IconClock className="h-4 w-4 text-muted-foreground" />;
}

function lastSyncedLabel(t: TFunction, config: WorkflowSyncConfig): string {
  if (config.last_synced_at) {
    // `formatRelative` routes its buckets through i18next; date-fns'
    // `formatDistanceToNow` would render English inside a translated sentence.
    const when = formatRelative(config.last_synced_at);
    return config.last_ok
      ? t("workflows:lastSynced", { when })
      : t("workflows:lastAttempt", { when });
  }
  return config.poll_enabled ? t("workflows:waitingForFirstSync") : t("workflows:notSyncedYet");
}

function MetadataLine({ config }: { config: WorkflowSyncConfig }) {
  const { t } = useTranslation();
  useTick(30_000);
  const parts = [
    t("workflows:directoryLine", { path: config.path || t("workflows:repositoryRoot") }),
    config.poll_enabled
      ? t("workflows:everySeconds", { count: config.interval_seconds })
      : t("workflows:autoSyncOff"),
    lastSyncedLabel(t, config),
  ];
  return <p className="text-xs text-muted-foreground">{parts.join(" · ")}</p>;
}

function WarningsAlert({ warnings }: { warnings: string[] }) {
  if (warnings.length === 0) return null;
  return (
    <Alert
      data-testid="workflow-sync-warnings"
      className="border-amber-500/40 bg-amber-500/10 dark:border-amber-400/30 dark:bg-amber-400/10"
    >
      <IconAlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400" />
      <AlertDescription className="text-sm">
        <ul className="list-disc pl-4 space-y-0.5">
          {warnings.map((warning, index) => (
            // Warnings are free-form backend sentences with no stable id;
            // include the index so repeated sentences keep unique keys.
            <li key={`${index}-${warning}`}>{warning}</li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}

type WorkflowSyncStatusCardProps = {
  config: WorkflowSyncConfig;
  syncing: boolean;
  onSyncNow: () => void;
};

// WorkflowSyncStatusCard is the compact always-visible summary of an active
// GitHub sync (mirrors the GitHub integration's connection-status card):
// headline with state icon, repo and branch, a muted metadata line, the last
// error when failing, and any warnings from the most recent attempt —
// warnings can be present even when last_ok is true (e.g. one file failed to
// parse but the rest synced).
export function WorkflowSyncStatusCard({
  config,
  syncing,
  onSyncNow,
}: WorkflowSyncStatusCardProps) {
  const { t } = useTranslation();
  const state = syncState(config);
  return (
    <div
      className="rounded-lg border bg-card p-4 space-y-2"
      data-testid="workflow-sync-status"
      data-state={state}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm">
          <StateIcon state={state} />
          <span>
            <Trans
              i18nKey="workflows:syncingFromRepository"
              values={{ repository: `${config.repo_owner}/${config.repo_name}` }}
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
          data-testid="workflow-sync-now"
        >
          {syncing ? (
            <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
          ) : (
            <IconRefresh className="h-4 w-4 mr-2" />
          )}
          {t("workflows:syncNow")}
        </Button>
      </div>
      <MetadataLine config={config} />
      {state === "failed" && (
        <p className="text-xs text-destructive">{config.last_error || t("workflows:syncFailed")}</p>
      )}
      <WarningsAlert warnings={config.last_warnings ?? []} />
    </div>
  );
}
