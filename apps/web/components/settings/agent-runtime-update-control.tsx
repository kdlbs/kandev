"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconAlertTriangle, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  canApproveAgentRuntimeUpdate,
  resolveRuntimeVersionPair,
} from "@/lib/agent-runtime-update";
import type { AgentUpdateJob, AgentUpdatePreview, InstallJob } from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";
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
  const { versionsMatch } = resolveRuntimeVersionPair(null, job);
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
        {versionsMatch ? t("agents:runtimeAlreadyUpToDate") : t("agents:runtimeUpdatedSuccess")}
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
};

function RuntimeVersionSummary({
  preview,
  job,
}: {
  preview: AgentUpdatePreview;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  const { currentVersion, targetVersion, versionsMatch } = resolveRuntimeVersionPair(preview, job);

  return (
    <div className="space-y-1">
      <p className="font-medium">{t("agents:runtimeVersion")}</p>
      <p className="break-words font-mono text-sm">
        {versionsMatch ? currentVersion : `${currentVersion} → ${targetVersion}`}
      </p>
      {versionsMatch && (
        <p className="text-sm text-muted-foreground" role="status">
          {t("agents:upToDate")}
        </p>
      )}
    </div>
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
}: UpdateBodyProps) {
  const { t } = useTranslation();
  const phase = updatePhase(t, job?.status);
  return (
    <div
      className="max-h-[calc(80dvh-10rem)] min-h-0 space-y-4 overflow-y-auto overscroll-contain px-4 py-3 text-xs/relaxed"
      data-testid={`agent-update-dialog-body-${agentName}`}
    >
      {loading && (
        <p className="flex items-center gap-2 text-muted-foreground" role="status">
          <IconLoader2 className="size-4 animate-spin" />
          {t("agents:checkingLatestRuntimeVersion")}
        </p>
      )}
      {previewError && (
        <div
          className="space-y-2 rounded-md border border-destructive/30 bg-destructive/5 p-3"
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
          className="rounded-md border border-destructive/30 bg-destructive/5 p-3"
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
        <>
          <RuntimeVersionSummary preview={preview} job={job} />
          <div className="space-y-1 text-muted-foreground">
            <p>{t("agents:runtimeUpdateExplainer")}</p>
            <p>{t("agents:runtimeUpdateSessionsNote")}</p>
          </div>
          <div className="space-y-1">
            <p className="font-medium">{t("agents:commandThatWillRun")}</p>
            <pre className="whitespace-pre-wrap break-all rounded-md bg-muted p-3 font-mono text-xs text-muted-foreground">
              {preview.command_string}
            </pre>
          </div>
        </>
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
          className="whitespace-pre-wrap break-words rounded-md bg-muted p-3 font-mono text-xs text-muted-foreground"
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
          {canRetry ? t("agents:retryUpdate") : t("agents:approveUpdate")}
        </Button>
      )}
    </>
  );
  if (mobile) {
    return (
      <DrawerFooter className="border-t pb-[max(1rem,env(safe-area-inset-bottom))]">
        {content}
      </DrawerFooter>
    );
  }
  return <DialogFooter className="border-t px-4 py-3">{content}</DialogFooter>;
}

function UpdateTrigger({
  agentName,
  displayName,
  installInFlight,
  onOpen,
}: {
  agentName: string;
  displayName: string;
  installInFlight: boolean;
  onOpen: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={installInFlight ? 0 : -1} className="inline-flex">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-11 w-11 cursor-pointer active:scale-95 sm:h-7 sm:w-7"
            aria-label={t(UPDATE_AGENT_KEY, { name: displayName })}
            disabled={installInFlight}
            onClick={onOpen}
            data-testid={`agent-update-trigger-${agentName}`}
          >
            <IconRefresh className="size-4" />
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        {installInFlight
          ? t("agents:agentInstallationInProgress")
          : t(UPDATE_AGENT_KEY, { name: displayName })}
      </TooltipContent>
    </Tooltip>
  );
}

export function AgentRuntimeUpdateControl({
  agentName,
  displayName,
  runtimeUpdate,
  job,
  installJob,
  onPreview,
  onUpdate,
}: {
  agentName: string;
  displayName: string;
  runtimeUpdate: RuntimeUpdate;
  job?: AgentUpdateJob;
  installJob?: InstallJob;
  onPreview: (agentName: string) => Promise<AgentUpdatePreview>;
  onUpdate: (agentName: string) => Promise<AgentUpdateJob>;
}) {
  const { t } = useTranslation();
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
      onRetryPreview={() => void loadPreview()}
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
        installInFlight={installInFlight}
        onOpen={() => handleOpenChange(true)}
      />
      {isMobile ? (
        <Drawer open={open} onOpenChange={handleOpenChange}>
          <DrawerContent className="max-h-[80dvh]" data-testid={`agent-update-drawer-${agentName}`}>
            <DrawerHeader className="shrink-0 text-left">
              <DrawerTitle>{t(UPDATE_AGENT_KEY, { name: displayName })}</DrawerTitle>
              <DrawerDescription>{t("agents:reviewUpdateBeforeApplying")}</DrawerDescription>
            </DrawerHeader>
            {body}
            {footer(true)}
          </DrawerContent>
        </Drawer>
      ) : (
        <Dialog open={open} onOpenChange={handleOpenChange}>
          <DialogContent
            className="max-h-[80dvh] gap-0 p-0 sm:max-w-xl"
            data-testid={`agent-update-dialog-${agentName}`}
          >
            <DialogHeader className="px-4 pt-4">
              <DialogTitle>{t(UPDATE_AGENT_KEY, { name: displayName })}</DialogTitle>
              <DialogDescription>{t("agents:reviewUpdateBeforeApplying")}</DialogDescription>
            </DialogHeader>
            {body}
            {footer()}
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
