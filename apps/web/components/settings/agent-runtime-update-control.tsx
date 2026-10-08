"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconAlertTriangle, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DialogFooter } from "@kandev/ui/dialog";
import { DrawerFooter } from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  canApproveAgentRuntimeUpdate,
  resolveRuntimeOperation,
  runtimeOperationLabelKey,
} from "@/lib/agent-runtime-update";
import type {
  AgentUpdateJob,
  AgentUpdateMode,
  AgentUpdatePreview,
  AgentUpdateStatus,
  InstallJob,
} from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";
import { AgentRuntimeUpdateSurface } from "./agent-runtime-update-surface";
import { RuntimeVersionPicker } from "./runtime-version-picker";
import { RuntimeVersionSummary } from "./runtime-version-summary";
import { useAgentUpdateDialogState } from "./use-agent-update-dialog-state";

const UPDATE_AGENT_KEY = "agents:updateAgent";

const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "probing",
  "saving",
  "refreshing",
]);

// The job `status` values are the wire enum; only the phase labels are copy, so
// they travel as catalog keys and resolve at render.
const UPDATE_PHASE_KEYS: Partial<Record<AgentUpdateJob["status"], string>> = {
  queued: "agents:updatePhaseQueued",
  resolving: "agents:updatePhaseResolving",
  updating: "agents:updatePhaseUpdating",
  probing: "agents:updatePhaseProbing",
  saving: "agents:updatePhaseSaving",
  refreshing: "agents:updatePhaseRefreshing",
};

function updatePhase(t: TFunction, status: AgentUpdateJob["status"] | undefined): string | null {
  const key = status ? UPDATE_PHASE_KEYS[status] : undefined;
  return key ? t(key) : null;
}

function UpdateResult({ agentName, job }: { agentName: string; job?: AgentUpdateJob }) {
  const { t } = useTranslation();
  if (!job) return null;
  const isUpToDate = job.operation === "up_to_date";
  if (job.status === "succeeded" && job.refresh_error) {
    return (
      <p
        className="break-words text-amber-600 dark:text-amber-400"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        {t("agents:runtimeUpdatedRefreshFailed", { error: job.refresh_error })}
      </p>
    );
  }
  if (job.status === "succeeded") {
    let successMessage = "agents:runtimeUpdatedSuccess";
    if (isUpToDate) successMessage = "agents:runtimeAlreadyUpToDate";
    else if (job.operation === "migrate") successMessage = "agents:openCodeMigrationSuccess";
    return (
      <p
        className="break-words text-green-600 dark:text-green-400"
        role="status"
        data-testid={`agent-update-result-${agentName}`}
      >
        {t(successMessage)}
      </p>
    );
  }
  if (job.status === "failed") {
    return (
      <p
        className="break-words text-destructive"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        {job.error || t("agents:runtimeUpdateFailed")}
      </p>
    );
  }
  return null;
}

type UpdateBodyProps = {
  agentName: string;
  preview: AgentUpdatePreview | null;
  loading: boolean;
  previewError: string | null;
  approveError: string | null;
  job?: AgentUpdateJob;
  onRetryPreview: () => void;
  selectedTarget: string;
  onSelectTarget: (targetVersion: string) => void;
  selectedUseDefault: boolean;
  selectedFamily?: "v2";
  onSelectDefault: () => void;
  onSelectMigration: () => void;
  onSelectCurrentRuntime: () => void;
  starting: boolean;
  isMobile: boolean;
};

function RuntimeUpdatePreviewDetails({
  agentName,
  preview,
  selectedTarget,
  selectedUseDefault,
  loading,
  starting,
  job,
  onSelectTarget,
  onSelectDefault,
  selectedFamily,
  onSelectMigration,
  onSelectCurrentRuntime,
  isMobile,
}: {
  agentName: string;
  preview: AgentUpdatePreview;
  selectedTarget: string;
  selectedUseDefault: boolean;
  selectedFamily?: "v2";
  loading: boolean;
  starting: boolean;
  job?: AgentUpdateJob;
  onSelectTarget: (targetVersion: string) => void;
  onSelectDefault: () => void;
  onSelectMigration: () => void;
  onSelectCurrentRuntime: () => void;
  isMobile: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <RuntimeVersionSummary agentName={agentName} preview={preview} job={job} />
      {preview.migration_available && (
        <div
          className="flex flex-wrap gap-2"
          role="group"
          aria-label={t("agents:openCodeRuntimeChoice")}
        >
          <Button
            type="button"
            variant={selectedFamily === "v2" ? "outline" : "default"}
            className={isMobile ? "min-h-11" : undefined}
            aria-pressed={selectedFamily !== "v2"}
            disabled={loading || starting}
            onClick={onSelectCurrentRuntime}
            data-testid={`agent-update-current-family-${agentName}`}
          >
            {t("agents:updateOpenCodeV1")}
          </Button>
          <Button
            type="button"
            variant={selectedFamily === "v2" ? "default" : "outline"}
            className={isMobile ? "min-h-11" : undefined}
            aria-pressed={selectedFamily === "v2"}
            disabled={loading || starting}
            onClick={onSelectMigration}
            data-testid={`agent-update-migrate-family-${agentName}`}
          >
            {t("agents:upgradeOpenCodeV2")}
          </Button>
        </div>
      )}
      {selectedFamily === "v2" ? (
        <div
          className="space-y-1 text-xs text-muted-foreground"
          data-testid={`agent-update-migration-scope-${agentName}`}
        >
          <p>{t("agents:openCodeMigrationExternalProcesses")}</p>
        </div>
      ) : (
        preview.update_mode !== "self_update" && (
          <RuntimeVersionPicker
            agentName={agentName}
            preview={preview}
            selectedTarget={selectedTarget}
            selectedUseDefault={selectedUseDefault}
            loading={loading}
            starting={starting}
            job={job}
            onSelectTarget={onSelectTarget}
            onSelectDefault={onSelectDefault}
          />
        )
      )}
      <details className="group" data-testid={`agent-update-command-${agentName}`}>
        <summary className="cursor-pointer py-1 font-medium text-muted-foreground focus-visible:outline-ring max-md:min-h-11 max-md:py-3 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-3">
          {t("agents:commandThatWillRun")}
        </summary>
        <pre className="mt-1 whitespace-pre-wrap break-all rounded-md bg-muted p-2 font-mono text-xs text-muted-foreground">
          {preview.command_string}
        </pre>
      </details>
    </>
  );
}

function UpdateBody({
  agentName,
  preview,
  loading,
  previewError,
  approveError,
  job,
  onRetryPreview,
  selectedTarget,
  onSelectTarget,
  selectedUseDefault,
  selectedFamily,
  onSelectDefault,
  onSelectMigration,
  onSelectCurrentRuntime,
  starting,
  isMobile,
}: UpdateBodyProps) {
  const { t } = useTranslation();
  const phase = updatePhase(t, job?.status);
  return (
    <div
      className="max-h-[calc(92dvh-10rem)] min-h-0 space-y-2 overflow-y-auto overscroll-contain px-4 py-2 text-xs/normal sm:max-h-[calc(92dvh-8rem)]"
      data-testid={`agent-update-dialog-body-${agentName}`}
    >
      {loading && !preview && (
        <p className="flex items-center gap-2 text-muted-foreground" role="status">
          <IconLoader2 className="size-4 animate-spin" />
          {t("agents:checkingLatestRuntimeVersion")}
        </p>
      )}
      {previewError && (
        <div
          className="space-y-2 rounded-md border border-destructive/30 bg-destructive/5 p-2"
          role="alert"
        >
          <p className="flex items-start gap-2 text-destructive">
            <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
            <span>{previewError}</span>
          </p>
          <Button type="button" variant="outline" size="sm" onClick={onRetryPreview}>
            {t("agents:retryVersionCheck")}
          </Button>
        </div>
      )}
      {approveError && (
        <div
          className="rounded-md border border-destructive/30 bg-destructive/5 p-2"
          role="alert"
          data-testid={`agent-update-approve-error-${agentName}`}
        >
          <p className="flex items-start gap-2 text-destructive">
            <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
            <span>{t("agents:unableToStartUpdate", { error: approveError })}</span>
          </p>
        </div>
      )}
      {preview && (
        <RuntimeUpdatePreviewDetails
          agentName={agentName}
          preview={preview}
          selectedTarget={selectedTarget}
          selectedUseDefault={selectedUseDefault}
          selectedFamily={selectedFamily}
          loading={loading}
          starting={starting}
          job={job}
          onSelectTarget={onSelectTarget}
          onSelectDefault={onSelectDefault}
          onSelectMigration={onSelectMigration}
          onSelectCurrentRuntime={onSelectCurrentRuntime}
          isMobile={isMobile}
        />
      )}
      {phase && (
        <p
          className="flex items-center gap-1.5 text-muted-foreground"
          role="status"
          data-testid={`agent-update-phase-${agentName}`}
        >
          <IconLoader2 className="size-3.5 shrink-0 animate-spin" />
          {phase}
        </p>
      )}
      {job?.output && (
        <pre
          data-testid={`agent-update-log-${agentName}`}
          className="whitespace-pre-wrap break-words rounded-md bg-muted p-2 font-mono text-xs text-muted-foreground"
        >
          {job.output}
        </pre>
      )}
      <UpdateResult agentName={agentName} job={job} />
    </div>
  );
}

type UpdateFooterProps = {
  agentName: string;
  preview: AgentUpdatePreview | null;
  previewError: string | null;
  job?: AgentUpdateJob;
  loading: boolean;
  starting: boolean;
  installInFlight: boolean;
  onApprove: () => void;
  onClose: () => void;
  mobile?: boolean;
};

function UpdateFooter({
  agentName,
  preview,
  previewError,
  job,
  loading,
  starting,
  installInFlight,
  onApprove,
  onClose,
  mobile,
}: UpdateFooterProps) {
  const { t } = useTranslation();
  const updateInFlight = Boolean(job && ACTIVE_UPDATE_STATUSES.has(job.status));
  const canRetry = job?.status === "failed";
  const canApprove = canApproveAgentRuntimeUpdate({
    preview,
    job,
    previewError,
    loading,
    updateInFlight,
    starting,
    installInFlight,
  });
  const showApprove = !job || canRetry;
  const content = (
    <>
      <Button type="button" variant="outline" onClick={onClose}>
        {job?.status === "succeeded" ? t("agents:done") : t("common:cancel")}
      </Button>
      {showApprove && (
        <Button
          type="button"
          disabled={!canApprove}
          onClick={onApprove}
          data-testid={`agent-update-confirm-${agentName}`}
        >
          {starting && <IconLoader2 className="mr-2 size-4 animate-spin" />}
          {canRetry
            ? t("agents:retryUpdate")
            : t(runtimeOperationLabelKey(resolveRuntimeOperation(preview, job)))}
        </Button>
      )}
    </>
  );
  if (mobile) {
    return (
      <DrawerFooter className="border-t px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
        {content}
      </DrawerFooter>
    );
  }
  return <DialogFooter className="border-t px-4 py-2">{content}</DialogFooter>;
}

function UpdateTrigger({
  agentName,
  displayName,
  runtimeUpdateStatus,
  installInFlight,
  onOpen,
}: {
  agentName: string;
  displayName: string;
  runtimeUpdateStatus?: AgentUpdateStatus;
  installInFlight: boolean;
  onOpen: () => void;
}) {
  const { t } = useTranslation();
  const statusLabel = runtimeUpdateStatusLabel(t, displayName, runtimeUpdateStatus);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={installInFlight ? 0 : -1} className="relative inline-flex">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="cursor-pointer active:scale-95"
            aria-label={statusLabel}
            disabled={installInFlight}
            onClick={onOpen}
            data-testid={`agent-update-trigger-${agentName}`}
          >
            <IconRefresh className="size-4" />
          </Button>
          {runtimeUpdateStatus?.check_state === "update_available" && (
            <span
              aria-hidden="true"
              className="pointer-events-none absolute right-0.5 top-0.5 size-2 rounded-full bg-sky-500 ring-2 ring-background"
              data-testid={`agent-update-available-dot-${agentName}`}
            />
          )}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        {installInFlight ? t("agents:agentInstallationInProgress") : statusLabel}
      </TooltipContent>
    </Tooltip>
  );
}

function runtimeUpdateStatusLabel(
  t: TFunction,
  displayName: string,
  status?: AgentUpdateStatus,
): string {
  if (status?.update_mode === "self_update" && status.check_state === "update_available") {
    return t(UPDATE_AGENT_KEY, { name: displayName });
  }
  if (status?.check_state === "update_available") {
    return t("agents:updateAvailableWithVersions", {
      name: displayName,
      current: status.effective_version,
      latest: status.latest_version,
    });
  }
  if (status?.check_state === "unknown") {
    return t("agents:runtimeUpdateStatusUnknown", { name: displayName });
  }
  return t(UPDATE_AGENT_KEY, { name: displayName });
}

type AgentRuntimeUpdateControlProps = {
  agentName: string;
  displayName: string;
  runtimeUpdate: RuntimeUpdate;
  runtimeUpdateStatus?: AgentUpdateStatus;
  job?: AgentUpdateJob;
  installJob?: InstallJob;
  onPreview: (
    agentName: string,
    targetVersion?: string,
    useDefault?: boolean,
    targetFamily?: "v2",
  ) => Promise<AgentUpdatePreview>;
  onUpdate: (
    agentName: string,
    targetVersion: string,
    useDefault?: boolean,
    targetFamily?: "v2" | AgentUpdateMode,
    expectedRuntimeRevision?: number,
  ) => Promise<AgentUpdateJob>;
};

type AgentRuntimeUpdateDialogViewProps = {
  control: AgentRuntimeUpdateControlProps;
  state: ReturnType<typeof useAgentUpdateDialogState>;
  isMobile: boolean;
};

function AgentRuntimeUpdateDialogView({
  control,
  state,
  isMobile,
}: AgentRuntimeUpdateDialogViewProps) {
  const { agentName, displayName, runtimeUpdateStatus, installJob } = control;
  const {
    activeJob,
    approve,
    approveError,
    handleOpenChange,
    loading,
    loadPreview,
    open,
    preview,
    previewError,
    selectTarget,
    selectDefault,
    selectMigration,
    selectCurrentRuntime,
    selectedTarget,
    selectedUseDefault,
    selectedFamily,
    starting,
  } = state;
  const installInFlight = installJob?.status === "queued" || installJob?.status === "running";

  const body = (
    <UpdateBody
      agentName={agentName}
      preview={preview}
      loading={loading}
      previewError={previewError}
      approveError={approveError}
      job={activeJob}
      onRetryPreview={() =>
        void loadPreview(
          selectedUseDefault ? undefined : selectedTarget,
          selectedUseDefault,
          selectedFamily,
        )
      }
      selectedTarget={selectedTarget}
      onSelectTarget={selectTarget}
      selectedUseDefault={selectedUseDefault}
      selectedFamily={selectedFamily}
      onSelectDefault={selectDefault}
      onSelectMigration={selectMigration}
      onSelectCurrentRuntime={selectCurrentRuntime}
      starting={starting}
      isMobile={isMobile}
    />
  );

  const footer = (mobile = false) => (
    <UpdateFooter
      agentName={agentName}
      preview={preview}
      previewError={previewError}
      job={activeJob}
      loading={loading}
      starting={starting}
      installInFlight={installInFlight}
      onApprove={() => void approve()}
      onClose={() => handleOpenChange(false)}
      mobile={mobile}
    />
  );

  return (
    <>
      <UpdateTrigger
        agentName={agentName}
        displayName={displayName}
        runtimeUpdateStatus={runtimeUpdateStatus}
        installInFlight={installInFlight}
        onOpen={() => handleOpenChange(true)}
      />
      <AgentRuntimeUpdateSurface
        managedFallback={control.runtimeUpdate.managed_fallback}
        agentName={agentName}
        displayName={displayName}
        isMobile={isMobile}
        open={open}
        onOpenChange={handleOpenChange}
        body={body}
        footer={footer}
      />
    </>
  );
}

export function AgentRuntimeUpdateControl(props: AgentRuntimeUpdateControlProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const state = useAgentUpdateDialogState({
    agentName: props.agentName,
    job: props.job,
    onPreview: props.onPreview,
    onUpdate: props.onUpdate,
  });
  if (!props.runtimeUpdate.supported) return null;
  return <AgentRuntimeUpdateDialogView control={props} state={state} isMobile={isMobile} />;
}
