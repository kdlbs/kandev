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
  resolveRuntimeActiveVersion,
  resolveRuntimeEffectiveVersion,
  resolveRuntimeOperation,
  resolveRuntimeVersionPair,
  runtimeOperationLabelKey,
} from "@/lib/agent-runtime-update";
import type { AgentUpdateJob, AgentUpdatePreview, AgentUpdateStatus, InstallJob } from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";
import { AgentRuntimeUpdateSurface } from "./agent-runtime-update-surface";
import { RuntimeVersionPicker } from "./runtime-version-picker";
import { useAgentUpdateDialogState } from "./use-agent-update-dialog-state";

const UPDATE_AGENT_KEY = "agents:updateAgent";

const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "refreshing",
]);

// The job `status` values are the wire enum; only the phase labels are copy, so
// they travel as catalog keys and resolve at render.
const UPDATE_PHASE_KEYS: Partial<Record<AgentUpdateJob["status"], string>> = {
  queued: "agents:updatePhaseQueued",
  resolving: "agents:updatePhaseResolving",
  updating: "agents:updatePhaseUpdating",
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
    return (
      <p
        className="break-words text-green-600 dark:text-green-400"
        role="status"
        data-testid={`agent-update-result-${agentName}`}
      >
        {isUpToDate ? t("agents:runtimeAlreadyUpToDate") : t("agents:runtimeUpdatedSuccess")}
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
  onSelectDefault: () => void;
  starting: boolean;
};

function RuntimeVersionSummary({
  agentName,
  preview,
  job,
}: {
  agentName: string;
  preview: AgentUpdatePreview;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  const { currentVersion, targetVersion } = resolveRuntimeVersionPair(preview, job);
  const activeVersion = resolveRuntimeActiveVersion(preview, job);
  const operation = resolveRuntimeOperation(preview, job);
  const isUpToDate = operation === "up_to_date";
  const effectiveVersion = resolveRuntimeEffectiveVersion(preview, job);

  return (
    <div className="space-y-0.5" data-testid={`agent-update-version-summary-${agentName}`}>
      <p className="font-medium" role={isUpToDate ? "status" : undefined}>
        {t(runtimeOperationLabelKey(operation))}
      </p>
      <p className="break-words font-mono text-sm">
        {isUpToDate ? currentVersion : `${currentVersion} → ${targetVersion}`}
      </p>
      <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs text-muted-foreground sm:grid-cols-3">
        {activeVersion && <p>{t("agents:activeRuntimeVersion", { version: activeVersion })}</p>}
        <p>{t("agents:effectiveRuntimeVersion", { version: effectiveVersion })}</p>
        {preview.default_version && (
          <p>{t("agents:kandevDefaultVersion", { version: preview.default_version })}</p>
        )}
      </div>
    </div>
  );
}

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
}: {
  agentName: string;
  preview: AgentUpdatePreview;
  selectedTarget: string;
  selectedUseDefault: boolean;
  loading: boolean;
  starting: boolean;
  job?: AgentUpdateJob;
  onSelectTarget: (targetVersion: string) => void;
  onSelectDefault: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <RuntimeVersionSummary agentName={agentName} preview={preview} job={job} />
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
      <div className="space-y-0.5 text-xs text-muted-foreground">
        <p>{t("agents:runtimeUpdateExplainer")}</p>
        <p>{t("agents:runtimeUpdateSessionsNote")}</p>
      </div>
      <div className="space-y-0.5">
        <p className="font-medium">{t("agents:commandThatWillRun")}</p>
        <pre className="whitespace-pre-wrap break-all rounded-md bg-muted p-2 font-mono text-xs text-muted-foreground">
          {preview.command_string}
        </pre>
      </div>
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
  onSelectDefault,
  starting,
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
          loading={loading}
          starting={starting}
          job={job}
          onSelectTarget={onSelectTarget}
          onSelectDefault={onSelectDefault}
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
            : t(runtimeOperationLabelKey(job?.operation ?? preview?.operation))}
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
            className="h-11 w-11 cursor-pointer active:scale-95 sm:h-7 sm:w-7"
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

export function AgentRuntimeUpdateControl({
  agentName,
  displayName,
  runtimeUpdate,
  runtimeUpdateStatus,
  job,
  installJob,
  onPreview,
  onUpdate,
}: {
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
  ) => Promise<AgentUpdatePreview>;
  onUpdate: (
    agentName: string,
    targetVersion: string,
    useDefault?: boolean,
  ) => Promise<AgentUpdateJob>;
}) {
  const { isMobile } = useResponsiveBreakpoint();
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
    selectedTarget,
    selectedUseDefault,
    starting,
  } = useAgentUpdateDialogState({ agentName, job, onPreview, onUpdate });
  const installInFlight = installJob?.status === "queued" || installJob?.status === "running";

  if (!runtimeUpdate.supported) return null;

  const body = (
    <UpdateBody
      agentName={agentName}
      preview={preview}
      loading={loading}
      previewError={previewError}
      approveError={approveError}
      job={activeJob}
      onRetryPreview={() =>
        void loadPreview(selectedUseDefault ? undefined : selectedTarget, selectedUseDefault)
      }
      selectedTarget={selectedTarget}
      onSelectTarget={selectTarget}
      selectedUseDefault={selectedUseDefault}
      onSelectDefault={selectDefault}
      starting={starting}
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
