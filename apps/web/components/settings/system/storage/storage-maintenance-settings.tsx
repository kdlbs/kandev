"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Card, CardContent } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconCheck, IconPlayerPlay, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  useStorageMaintenance,
  type StorageBusyState,
} from "@/hooks/domains/system/use-storage-maintenance";
import type { StorageMaintenanceSettings as Settings, SystemJob } from "@/lib/types/system";
import { useSettingsSaveContributor } from "../../settings-save-provider";
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

function StorageActionFeedback({
  controller,
}: {
  controller: ReturnType<typeof useStorageMaintenance>;
}) {
  if (controller.busy) {
    return (
      <StorageBusyFeedback busy={controller.busy} onRunAnyway={() => void controller.runAnyway()} />
    );
  }
  if (!controller.error) return null;
  return (
    <Alert variant="destructive" data-testid="storage-error">
      <IconAlertTriangle className="size-4" />
      <AlertTitle>Storage action failed</AlertTitle>
      <AlertDescription className="break-words">{controller.error}</AlertDescription>
    </Alert>
  );
}

function StorageBusyFeedback({
  busy,
  onRunAnyway,
}: {
  busy: StorageBusyState;
  onRunAnyway: () => void;
}) {
  return (
    <Alert variant="destructive" data-testid="storage-busy">
      <IconAlertTriangle className="size-4" />
      <AlertTitle>Storage cleanup found active Kandev work</AlertTitle>
      <AlertDescription className="break-words">
        <p>Cleanup may disrupt the following work:</p>
        <ul className="mt-2 list-disc space-y-1 pl-5">
          {busy.resources.map((resource) => (
            <li key={resource.kind}>{resource.label}</li>
          ))}
        </ul>
        {busy.forceAvailable && (
          <>
            <p className="mt-3">Running cleanup anyway may disrupt this active work.</p>
            <StorageActionButton
              variant="outline"
              className="mt-3 w-full sm:w-auto"
              onClick={onRunAnyway}
              data-testid="storage-run-anyway"
            >
              <IconPlayerPlay className="size-4" /> Run anyway
            </StorageActionButton>
          </>
        )}
      </AlertDescription>
    </Alert>
  );
}

function serializeSettings(settings: Settings | null): string {
  return settings ? JSON.stringify(settings) : "loading";
}

function policyPendingAction(action: ReturnType<typeof useStorageMaintenance>["pendingAction"]) {
  return action === "save" || action === "adopt";
}

function policyBlockedReason(
  t: (key: string) => string,
  action: ReturnType<typeof useStorageMaintenance>["pendingAction"],
  loading: boolean,
) {
  if (action === "adopt") return t("settings:storageAdoptionPending");
  if (loading) return t("settings:storagePolicyLoadingBlock");
  return undefined;
}

function storageActionDisabledReason(
  t: (key: string) => string,
  action: ReturnType<typeof useStorageMaintenance>["pendingAction"],
) {
  if (action) return t("settings:storageActionPending");
  return undefined;
}

function useStoragePolicyDraft(controller: ReturnType<typeof useStorageMaintenance>) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Settings | null>(null);
  const previousServerSettings = useRef<Settings | null>(null);
  const savedSettings = controller.policy?.settings ?? controller.overview?.settings ?? null;
  const policyLoading = controller.loading?.policy ?? !savedSettings;

  useEffect(() => {
    if (!savedSettings) return;
    setDraft((current) => {
      const previous = previousServerSettings.current;
      if (!current || !previous || serializeSettings(current) === serializeSettings(previous)) {
        return savedSettings;
      }
      return {
        ...current,
        go_cache: { ...current.go_cache, adopted_path: savedSettings.go_cache.adopted_path },
      };
    });
    previousServerSettings.current = savedSettings;
  }, [savedSettings]);

  const invalidReason = policyBlockedReason(t, controller.pendingAction, policyLoading);
  useSettingsSaveContributor({
    id: "system:storage-policy",
    revision: serializeSettings(draft),
    isDirty: Boolean(
      draft && savedSettings && serializeSettings(draft) !== serializeSettings(savedSettings),
    ),
    canSave: !invalidReason,
    invalidReason,
    save: async () => {
      if (!draft || !savedSettings) return;
      const confirmation =
        draft.docker.dedicated_daemon_acknowledged &&
        !savedSettings.docker.dedicated_daemon_acknowledged
          ? "DEDICATED"
          : undefined;
      await controller.save(draft, confirmation);
    },
    discard: () => {
      if (savedSettings) setDraft(savedSettings);
    },
  });

  return { draft, setDraft, savedSettings };
}

function StoragePolicyState({ loading, error }: { loading: boolean; error?: string | null }) {
  const { t } = useTranslation();
  if (!loading && !error) return null;
  return (
    <Card data-testid="storage-policy-state">
      <CardContent className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
        {loading && <Spinner className="size-4" data-testid="storage-policy-spinner" />}
        <span>{loading ? t("settings:loading") : t("settings:storageSectionUnavailable")}</span>
        {error && <span className="break-words text-destructive">{error}</span>}
      </CardContent>
    </Card>
  );
}

function StoragePrimarySections({
  controller,
  disabledReason,
  draft,
  setDraft,
  savedSettings,
}: {
  controller: ReturnType<typeof useStorageMaintenance>;
  disabledReason?: string;
  draft: Settings | null;
  setDraft: (settings: Settings | null) => void;
  savedSettings: Settings | null;
}) {
  const controlsPending = policyPendingAction(controller.pendingAction);
  const policyLoading = controller.loading?.policy ?? !savedSettings;
  const capabilities = controller.policy?.capabilities ?? controller.overview?.capabilities;
  return (
    <div className="min-w-0 space-y-4" data-testid="storage-primary-sections">
      <StorageOverviewCard
        overview={controller.overview}
        settings={savedSettings ?? undefined}
        loading={controller.loading?.overview}
        error={controller.sectionErrors?.overview}
        disabledReason={disabledReason}
        onRunGoCache={() => void controller.runNow(["go_cache"])}
      />
      <StoragePolicyState loading={policyLoading} error={controller.sectionErrors?.policy} />
      {draft && savedSettings && capabilities && (
        <StoragePolicyCard
          settings={draft}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={controlsPending}
          onChange={setDraft}
          onAdopt={controller.adopt}
        />
      )}
    </div>
  );
}

function StorageQuarantineSection({
  controller,
  disabledReason,
  savedSettings,
}: {
  controller: ReturnType<typeof useStorageMaintenance>;
  disabledReason?: string;
  savedSettings: Settings | null;
}) {
  const deleteJobActive =
    controller.deleteJob?.state === "queued" || controller.deleteJob?.state === "running";
  return (
    <StorageQuarantineCard
      entries={controller.quarantine}
      loading={controller.loading?.quarantine}
      error={controller.sectionErrors?.quarantine}
      deleteJobId={controller.deleteJob?.id}
      deleteJobActive={deleteJobActive}
      disabledReason={disabledReason}
      schedulingEnabled={savedSettings?.enabled ?? false}
      checkIntervalHours={savedSettings?.check_interval_hours ?? 24}
      onRestore={controller.restore}
      onDelete={controller.permanentlyDelete}
      onClearEligible={controller.clearEligible}
      onForceClearAll={controller.forceClearAll}
    />
  );
}

function StoragePageSections({
  controller,
  disabledReason,
}: {
  controller: ReturnType<typeof useStorageMaintenance>;
  disabledReason?: string;
}) {
  const { draft, setDraft, savedSettings } = useStoragePolicyDraft(controller);
  return (
    <>
      <StoragePrimarySections
        controller={controller}
        disabledReason={disabledReason}
        draft={draft}
        setDraft={setDraft}
        savedSettings={savedSettings}
      />
      <StorageRunHistory
        runs={controller.runs}
        loading={controller.loading?.runs}
        error={controller.sectionErrors?.runs}
      />
      <StorageQuarantineSection
        controller={controller}
        disabledReason={disabledReason}
        savedSettings={savedSettings}
      />
    </>
  );
}

export function StorageMaintenanceSettings() {
  const { t } = useTranslation();
  const controller = useStorageMaintenance();
  const actionDisabledReason = storageActionDisabledReason(t, controller.pendingAction);

  return (
    <div className="min-w-0 space-y-6" data-testid="storage-settings-page">
      <StorageActions controller={controller} disabledReason={actionDisabledReason} />

      <StorageActionFeedback controller={controller} />

      <StoragePageSections controller={controller} disabledReason={actionDisabledReason} />
    </div>
  );
}
