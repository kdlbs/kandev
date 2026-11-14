"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconAlertTriangle, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DialogFooter } from "@kandev/ui/dialog";
import { DrawerFooter } from "@kandev/ui/drawer";
import {
  canApproveAgentRuntimeUpdate,
  resolveRuntimeOperation,
  runtimeOperationLabelKey,
} from "@/lib/agent-runtime-update";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import { CopilotRuntimeCompatibilityNotice } from "./copilot-runtime-compatibility";
import { RuntimeVersionPicker } from "./runtime-version-picker";
import { RuntimeVersionSummary } from "./runtime-version-summary";

export const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
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

export type AgentRuntimeUpdateBodyProps = {
  before?: ReactNode;
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
      <CopilotRuntimeCompatibilityNotice
        agentName={agentName}
        compatibility={preview.copilot_plugin_compatibility}
      />
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

export function AgentRuntimeUpdateBody({
  before,
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
}: AgentRuntimeUpdateBodyProps) {
  const { t } = useTranslation();
  const phase = updatePhase(t, job?.status);
  return (
    <div
      className="min-h-0 flex-1 space-y-2 overflow-y-auto overscroll-contain px-4 py-2 text-xs/normal"
      data-testid={`agent-update-dialog-body-${agentName}`}
    >
      {before}
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

export type AgentRuntimeUpdateFooterProps = {
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
  /** False for read-only operators: inspection stays available, mutation does not. */
  canManage?: boolean;
  /** Visible reason that keeps the primary action unavailable. */
  blockedReason?: string;
  /** Extra controls rendered above the action row inside the fixed footer. */
  beforeActions?: ReactNode;
};

export function AgentRuntimeUpdateFooter({
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
  canManage = true,
  blockedReason,
  beforeActions,
}: AgentRuntimeUpdateFooterProps) {
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
          disabled={!canApprove || !canManage || Boolean(blockedReason)}
          title={blockedReason}
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
      <DrawerFooter className="shrink-0 border-t px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
        {beforeActions}
        {content}
      </DrawerFooter>
    );
  }
  return (
    <DialogFooter className="shrink-0 border-t px-4 py-2">
      {beforeActions}
      {content}
    </DialogFooter>
  );
}
