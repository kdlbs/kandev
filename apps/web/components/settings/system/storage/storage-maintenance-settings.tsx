"use client";

import { useEffect, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { IconAlertTriangle, IconPlayerPlay, IconRefresh } from "@tabler/icons-react";
import { useStorageMaintenance } from "@/hooks/domains/system/use-storage-maintenance";
import type { StorageMaintenanceSettings as Settings } from "@/lib/types/system";
import { JobProgressIndicator } from "../job-progress-indicator";
import { StorageActionButton } from "./storage-action-button";
import { StorageOverviewCard } from "./storage-overview-card";
import { StoragePolicyCard } from "./storage-policy-card";
import { StorageQuarantineCard } from "./storage-quarantine-card";
import { StorageRunHistory } from "./storage-run-history";

export function StorageMaintenanceSettings() {
  const controller = useStorageMaintenance();
  const [draft, setDraft] = useState<Settings | null>(null);
  useEffect(() => {
    if (controller.overview) setDraft(controller.overview.settings);
  }, [controller.overview]);
  const pending = controller.pendingAction !== null;
  const disabledReason = pending ? "Wait for the current storage action to finish." : undefined;

  return (
    <div className="min-w-0 space-y-6" data-testid="storage-settings-page">
      <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 flex-col gap-2">
          <JobProgressIndicator
            kind="storage-analysis"
            jobId={controller.analysisJob?.id}
            testId="storage-analysis-job"
          />
          <JobProgressIndicator
            kind="storage-cleanup"
            jobId={controller.cleanupJob?.id}
            testId="storage-cleanup-job"
          />
          <JobProgressIndicator
            kind="storage-quarantine-delete"
            jobId={controller.deleteJob?.id}
            testId="storage-delete-job"
          />
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <StorageActionButton
            variant="outline"
            disabledReason={disabledReason}
            onClick={() => void controller.analyze()}
            data-testid="storage-analyze"
          >
            <IconRefresh className="size-4" /> Analyze
          </StorageActionButton>
          <StorageActionButton
            disabledReason={disabledReason}
            onClick={() => void controller.runNow()}
            data-testid="storage-run-now"
          >
            <IconPlayerPlay className="size-4" /> Run now
          </StorageActionButton>
        </div>
      </div>

      {controller.error && (
        <Alert variant="destructive" data-testid="storage-error">
          <IconAlertTriangle className="size-4" />
          <AlertTitle>Storage action failed</AlertTitle>
          <AlertDescription className="break-words">{controller.error}</AlertDescription>
        </Alert>
      )}

      <div className="grid min-w-0 grid-cols-1 gap-4 xl:grid-cols-2">
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
        disabledReason={disabledReason}
        onRestore={controller.restore}
        onDelete={controller.permanentlyDelete}
      />
    </div>
  );
}
