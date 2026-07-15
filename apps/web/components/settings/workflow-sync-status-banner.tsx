"use client";

import { formatDistanceToNow } from "date-fns";
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

function lastSyncedLabel(config: WorkflowSyncConfig): string {
  if (config.last_synced_at) {
    return `last synced ${formatDistanceToNow(new Date(config.last_synced_at), { addSuffix: true })}`;
  }
  return config.poll_enabled ? "waiting for first sync…" : "not synced yet — use Sync now";
}

function MetadataLine({ config }: { config: WorkflowSyncConfig }) {
  useTick(30_000);
  const parts = [
    `Directory ${config.path || "(repository root)"}`,
    config.poll_enabled ? `every ${config.interval_seconds}s` : "auto-sync off",
    lastSyncedLabel(config),
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
          {warnings.map((warning) => (
            // Warnings are free-form backend sentences with no stable id;
            // the text itself is stable enough to key on for this list.
            <li key={warning}>{warning}</li>
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
            Syncing from{" "}
            <span className="font-semibold">
              {config.repo_owner}/{config.repo_name}
            </span>
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
          Sync now
        </Button>
      </div>
      <MetadataLine config={config} />
      {state === "failed" && (
        <p className="text-xs text-destructive">{config.last_error || "Sync failed"}</p>
      )}
      <WarningsAlert warnings={config.last_warnings ?? []} />
    </div>
  );
}
