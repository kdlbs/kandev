"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconCheck, IconPlayerPlay, IconRefresh } from "@tabler/icons-react";
import { useStorageMaintenance } from "@/hooks/domains/system/use-storage-maintenance";
import type { StorageMaintenanceSettings as Settings, SystemJob } from "@/lib/types/system";
import { StorageActionButton } from "./storage-action-button";
import { StorageOverviewCard } from "./storage-overview-card";
import { StoragePolicyCard } from "./storage-policy-card";
import { StorageQuarantineCard } from "./storage-quarantine-card";
import { StorageRunHistory } from "./storage-run-history";

function StorageJobButtonContent({
  job,
  idleLabel,
  activeLabel,
  successLabel,
  failedLabel,
  idleIcon,
}: {
  job?: SystemJob;
  idleLabel: string;
  activeLabel: string;
  successLabel: string;
  failedLabel: string;
  idleIcon: ReactNode;
}) {
  if (job?.state === "queued" || job?.state === "running") {
    return (
      <>
        <Spinner className="size-4" /> {activeLabel}
      </>
    );
  }
  if (job?.state === "succeeded") {
    return (
      <>
        <IconCheck className="size-4" /> {successLabel}
      </>
    );
  }
  if (job?.state === "failed") {
    return (
      <>
        <IconAlertTriangle className="size-4" /> {failedLabel}
      </>
    );
  }
  return (
    <>
      {idleIcon} {idleLabel}
    </>
  );
}

function StorageActions({
  controller,
  disabledReason,
}: {
  controller: ReturnType<typeof useStorageMaintenance>;
  disabledReason?: string;
}) {
  const analysisActive =
    controller.analysisJob?.state === "queued" || controller.analysisJob?.state === "running";
  const cleanupActive =
    controller.cleanupJob?.state === "queued" || controller.cleanupJob?.state === "running";
  return (
    <div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0 sm:max-w-xl">
        <p className="text-sm font-medium">Reclaim disk space safely</p>
        <p className="text-xs text-muted-foreground">
          Analyze for a read-only snapshot, or run the enabled cleanup rules when you want to
          recover space immediately.
        </p>
      </div>
      <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
        <div data-testid="storage-analyze-control">
          <StorageActionButton
            variant="outline"
            className="w-full sm:w-44"
            disabledReason={
              disabledReason ?? (analysisActive ? "Storage analysis is still running." : undefined)
            }
            onClick={() => void controller.analyze()}
            data-testid="storage-analyze"
            data-job-state={controller.analysisJob?.state}
          >
            <StorageJobButtonContent
              job={controller.analysisJob}
              idleLabel="Analyze"
              activeLabel="Analyzing..."
              successLabel="Analysis complete"
              failedLabel="Analysis failed"
              idleIcon={<IconRefresh className="size-4" />}
            />
          </StorageActionButton>
        </div>
        <div data-testid="storage-cleanup-control">
          <StorageActionButton
            className="w-full sm:w-44"
            disabledReason={
              disabledReason ?? (cleanupActive ? "Storage cleanup is still running." : undefined)
            }
            onClick={() => void controller.runNow()}
            data-testid="storage-run-now"
            data-job-state={controller.cleanupJob?.state}
          >
            <StorageJobButtonContent
              job={controller.cleanupJob}
              idleLabel="Run now"
              activeLabel="Cleaning..."
              successLabel="Cleanup complete"
              failedLabel="Cleanup failed"
              idleIcon={<IconPlayerPlay className="size-4" />}
            />
          </StorageActionButton>
        </div>
      </div>
    </div>
  );
}

export function StorageMaintenanceSettings() {
  const controller = useStorageMaintenance();
  const [draft, setDraft] = useState<Settings | null>(null);
  const previousServerSettings = useRef<Settings | null>(null);
  useEffect(() => {
    if (!controller.overview) return;
    const nextSettings = controller.overview.settings;
    setDraft((current) => {
      const previous = previousServerSettings.current;
      if (!current || !previous || JSON.stringify(current) === JSON.stringify(previous)) {
        return nextSettings;
      }
      return current;
    });
    previousServerSettings.current = nextSettings;
  }, [controller.overview]);
  const pending = controller.pendingAction !== null;
  const disabledReason = pending ? "Wait for the current storage action to finish." : undefined;

  return (
    <div className="min-w-0 space-y-6" data-testid="storage-settings-page">
      <StorageActions controller={controller} disabledReason={disabledReason} />

      {controller.error && (
        <Alert variant="destructive" data-testid="storage-error">
          <IconAlertTriangle className="size-4" />
          <AlertTitle>Storage action failed</AlertTitle>
          <AlertDescription className="break-words">{controller.error}</AlertDescription>
        </Alert>
      )}

      <div className="min-w-0 space-y-4" data-testid="storage-primary-sections">
        <StorageOverviewCard
          overview={controller.overview}
          disabledReason={disabledReason}
          onRunGoCache={() => void controller.runNow(["go_cache"])}
        />
        {draft && controller.overview && (
          <StoragePolicyCard
            settings={draft}
            capabilities={controller.overview.capabilities}
            pending={pending}
            onChange={setDraft}
            onSave={() => controller.save(draft)}
            onDedicatedConfirm={(next) => controller.save(next, "DEDICATED")}
            onAdopt={controller.adopt}
          />
        )}
      </div>
      <StorageRunHistory runs={controller.runs} />
      <StorageQuarantineCard
        entries={controller.quarantine}
        deleteJobId={controller.deleteJob?.id}
        disabledReason={disabledReason}
        onRestore={controller.restore}
        onDelete={controller.permanentlyDelete}
      />
    </div>
  );
}
