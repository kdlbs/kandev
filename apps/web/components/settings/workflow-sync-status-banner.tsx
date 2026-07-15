"use client";

import { formatDistanceToNow } from "date-fns";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { useTick } from "@/components/integrations/auth-status-banner";
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";

function LastSyncedLabel({ syncedAt }: { syncedAt: Date }) {
  useTick(30_000);
  return (
    <span className="text-xs text-muted-foreground ml-2">
      · checked {formatDistanceToNow(syncedAt, { addSuffix: true })}
    </span>
  );
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

function SyncStatusAlert({ config }: { config: WorkflowSyncConfig }) {
  if (!config.last_synced_at) {
    return (
      <Alert data-testid="workflow-sync-status" data-state="waiting">
        <AlertDescription className="text-sm">Waiting for first sync…</AlertDescription>
      </Alert>
    );
  }
  if (config.last_ok) {
    return (
      <Alert
        data-testid="workflow-sync-status"
        data-state="ok"
        className="border-green-500/40 bg-green-500/10 dark:border-green-400/30 dark:bg-green-400/10"
      >
        <IconCheck className="h-4 w-4 text-green-600 dark:text-green-400" />
        <AlertDescription className="text-sm font-medium">
          Synced
          <LastSyncedLabel syncedAt={new Date(config.last_synced_at)} />
        </AlertDescription>
      </Alert>
    );
  }
  return (
    <Alert data-testid="workflow-sync-status" data-state="failed" variant="destructive">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertDescription className="text-sm">
        {config.last_error || "Sync failed"}
        <LastSyncedLabel syncedAt={new Date(config.last_synced_at)} />
      </AlertDescription>
    </Alert>
  );
}

// WorkflowSyncStatusBanner renders the last-sync outcome (never-synced / ok /
// failed) plus, independently, any non-empty warnings from the most recent
// attempt — warnings can be present even when last_ok is true (e.g. an
// individual file failed to parse but the rest synced).
export function WorkflowSyncStatusBanner({ config }: { config: WorkflowSyncConfig | null }) {
  if (!config) return null;
  return (
    <div className="space-y-2">
      <SyncStatusAlert config={config} />
      <WarningsAlert warnings={config.last_warnings ?? []} />
    </div>
  );
}
