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
import { StorageDiskCapacityCard } from "./storage-disk-capacity-card";
import { StorageOverviewCard } from "./storage-overview-card";
import { StoragePolicyCard } from "./storage-policy-card";
import { StorageQuarantineCard } from "./storage-quarantine-card";
import { StorageRunHistory } from "./storage-run-history";
import { SettingsTarget } from "../../settings-target";
import { SYSTEM_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/system";
import { TemporaryArtifactsDialog } from "./storage-confirmation-dialogs";

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
  const { t } = useTranslation();
  const analysisActive =
    controller.analysisJob?.state === "queued" || controller.analysisJob?.state === "running";
  const cleanupActive =
    controller.cleanupJob?.state === "queued" || controller.cleanupJob?.state === "running";
  return (
    <SettingsTarget
      targetId={SYSTEM_SETTINGS_TARGETS.storageActions}
      className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"
    >
      <div className="min-w-0 sm:max-w-xl">
        <p className="text-sm font-medium">{t("system:storageActionsTitle")}</p>
        <p className="text-xs text-muted-foreground">{t("system:storageActionsDescription")}</p>
      </div>
      <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
        <div data-testid="storage-analyze-control">
          <StorageActionButton
            variant="outline"
            className="w-full sm:w-44"
            disabledReason={
              disabledReason ?? (analysisActive ? t("system:storageAnalysisRunning") : undefined)
            }
            onClick={() => void controller.analyze()}
            data-testid="storage-analyze"
            data-job-state={controller.analysisJob?.state}
          >
            <StorageJobButtonContent
              job={controller.analysisJob}
              idleLabel={t("system:storageAnalyze")}
              activeLabel={t("system:storageAnalyzing")}
              successLabel={t("system:storageAnalysisComplete")}
              failedLabel={t("system:storageAnalysisFailed")}
              idleIcon={<IconRefresh className="size-4" />}
            />
          </StorageActionButton>
        </div>
        <div data-testid="storage-cleanup-control">
          <StorageActionButton
            className="w-full sm:w-44"
            disabledReason={
              disabledReason ?? (cleanupActive ? t("system:storageCleanupRunning") : undefined)
            }
            onClick={() => void controller.runNow()}
            data-testid="storage-run-now"
            data-job-state={controller.cleanupJob?.state}
          >
            <StorageJobButtonContent
              job={controller.cleanupJob}
              idleLabel={t("system:storageRunNow")}
              activeLabel={t("system:storageCleaning")}
              successLabel={t("system:storageCleanupComplete")}
              failedLabel={t("system:storageCleanupFailed")}
              idleIcon={<IconPlayerPlay className="size-4" />}
            />
          </StorageActionButton>
        </div>
      </div>
    </SettingsTarget>
  );
}

function StorageActionFeedback({
  controller,
}: {
  controller: ReturnType<typeof useStorageMaintenance>;
}) {
  const { t } = useTranslation();
  if (controller.busy) {
    return (
      <StorageBusyFeedback busy={controller.busy} onRunAnyway={() => void controller.runAnyway()} />
    );
  }
  if (!controller.error) return null;
  return (
    <Alert variant="destructive" data-testid="storage-error">
      <IconAlertTriangle className="size-4" />
      <AlertTitle>{t("system:storageActionFailed")}</AlertTitle>
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
  const { t } = useTranslation();
  return (
    <Alert variant="destructive" data-testid="storage-busy">
      <IconAlertTriangle className="size-4" />
      <AlertTitle>{t("system:storageBusyTitle")}</AlertTitle>
      <AlertDescription className="break-words">
        <p>{t("system:storageBusyIntro")}</p>
        <ul className="mt-2 list-disc space-y-1 pl-5">
          {/* Resource labels are rendered by the API, not the catalog. */}
          {busy.resources.map((resource) => (
            <li key={resource.kind}>{resource.label}</li>
          ))}
        </ul>
        {busy.forceAvailable && (
          <>
            <p className="mt-3">{t("system:storageBusyForceHint")}</p>
            <StorageActionButton
              variant="outline"
              className="mt-3 w-full sm:w-auto"
              onClick={onRunAnyway}
              data-testid="storage-run-anyway"
            >
              <IconPlayerPlay className="size-4" /> {t("system:storageRunAnyway")}
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
  if (action === "adopt") return t("system:storageAdoptionPending");
  if (loading) return t("system:storagePolicyLoadingBlock");
  return undefined;
}

function storageActionDisabledReason(
  t: (key: string) => string,
  action: ReturnType<typeof useStorageMaintenance>["pendingAction"],
) {
  if (action) return t("system:storageActionPending");
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
        <span>{loading ? t("settings:loading") : t("system:storageSectionUnavailable")}</span>
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
  onRunTemporaryArtifacts,
}: {
  controller: ReturnType<typeof useStorageMaintenance>;
  disabledReason?: string;
  draft: Settings | null;
  setDraft: (settings: Settings | null) => void;
  savedSettings: Settings | null;
  onRunTemporaryArtifacts: () => void;
}) {
  const controlsPending = policyPendingAction(controller.pendingAction);
  const policyLoading = controller.loading?.policy ?? !savedSettings;
  const capabilities = controller.policy?.capabilities ?? controller.overview?.capabilities;
  return (
    <div className="min-w-0 space-y-4" data-testid="storage-primary-sections">
      <StorageDiskCapacityCard
        disk={controller.disk}
        loading={controller.loading?.disk}
        error={controller.sectionErrors?.disk}
      />
      <StorageOverviewCard
        overview={controller.overview}
        settings={savedSettings ?? undefined}
        loading={controller.loading?.overview}
        error={controller.sectionErrors?.overview}
        disabledReason={disabledReason}
        onRunGoCache={() => void controller.runNow(["go_cache"])}
        onRunTemporaryArtifacts={onRunTemporaryArtifacts}
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
          onCleanDependencies={() => void controller.runNow(["workspace_dependencies"])}
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
  onRunTemporaryArtifacts,
}: {
  controller: ReturnType<typeof useStorageMaintenance>;
  disabledReason?: string;
  onRunTemporaryArtifacts: () => void;
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
        onRunTemporaryArtifacts={onRunTemporaryArtifacts}
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
  const [temporaryCleanupOpen, setTemporaryCleanupOpen] = useState(false);

  return (
    <div className="min-w-0 space-y-6" data-testid="storage-settings-page">
      <StorageActions controller={controller} disabledReason={actionDisabledReason} />

      <StorageActionFeedback controller={controller} />

      <StoragePageSections
        controller={controller}
        disabledReason={actionDisabledReason}
        onRunTemporaryArtifacts={() => setTemporaryCleanupOpen(true)}
      />
      <TemporaryArtifactsDialog
        open={temporaryCleanupOpen}
        onOpenChange={setTemporaryCleanupOpen}
        onConfirm={() => {
          setTemporaryCleanupOpen(false);
          void controller.runNow(["temporary_artifacts"]);
        }}
      />
    </div>
  );
}
