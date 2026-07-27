"use client";

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
import type { AgentUpdateJob, AgentUpdatePreview, InstallJob } from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";
import { useAgentUpdateDialogState } from "./use-agent-update-dialog-state";

const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "refreshing",
]);

function updatePhase(status: AgentUpdateJob["status"] | undefined): string | null {
  switch (status) {
    case "queued":
      return "Update queued…";
    case "resolving":
      return "Checking latest version…";
    case "updating":
      return "Updating runtime…";
    case "refreshing":
      return "Refreshing agent capabilities…";
    default:
      return null;
  }
}

function UpdateResult({ agentName, job }: { agentName: string; job?: AgentUpdateJob }) {
  if (!job) return null;
  if (job.status === "succeeded" && job.refresh_error) {
    return (
      <p
        className="break-words text-amber-600 dark:text-amber-400"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        Runtime updated, but capabilities could not be refreshed: {job.refresh_error}
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
        Runtime updated successfully.
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
        {job.error || "The runtime update failed."}
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

function UpdateBody({
  agentName,
  preview,
  loading,
  previewError,
  approveError,
  job,
  onRetryPreview,
}: UpdateBodyProps) {
  const phase = updatePhase(job?.status);
  return (
    <div
      className="max-h-[calc(80dvh-10rem)] min-h-0 space-y-4 overflow-y-auto overscroll-contain px-4 py-3 text-xs/relaxed"
      data-testid={`agent-update-dialog-body-${agentName}`}
    >
      {loading && (
        <p className="flex items-center gap-2 text-muted-foreground" role="status">
          <IconLoader2 className="size-4 animate-spin" />
          Checking the latest runtime version…
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
            Retry version check
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
            <span>Unable to start update: {approveError}</span>
          </p>
        </div>
      )}
      {preview && (
        <>
          <div className="space-y-1">
            <p className="font-medium">Runtime version</p>
            <p className="break-words font-mono text-sm">
              {(job?.current_version || preview.current_version || "Unknown") +
                " → " +
                (job?.target_version || preview.target_version)}
            </p>
          </div>
          <div className="space-y-1 text-muted-foreground">
            <p>
              This updates the managed runtime on this Kandev host, then refreshes its models and
              capabilities.
            </p>
            <p>
              Active sessions keep running; only later probes and launches use the refreshed
              runtime.
            </p>
          </div>
          <div className="space-y-1">
            <p className="font-medium">Command that will run</p>
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

function canApproveUpdate({
  preview,
  previewError,
  loading,
  updateInFlight,
  starting,
  installInFlight,
}: {
  preview: AgentUpdatePreview | null;
  previewError: string | null;
  loading: boolean;
  updateInFlight: boolean;
  starting: boolean;
  installInFlight: boolean;
}) {
  return (
    Boolean(preview?.current_version) &&
    !previewError &&
    !loading &&
    !updateInFlight &&
    !starting &&
    !installInFlight
  );
}

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
  const updateInFlight = Boolean(job && ACTIVE_UPDATE_STATUSES.has(job.status));
  const canRetry = job?.status === "failed";
  const canApprove = canApproveUpdate({
    preview,
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
        {job?.status === "succeeded" ? "Done" : "Cancel"}
      </Button>
      {showApprove && (
        <Button
          type="button"
          disabled={!canApprove}
          onClick={onApprove}
          data-testid={`agent-update-confirm-${agentName}`}
        >
          {starting && <IconLoader2 className="mr-2 size-4 animate-spin" />}
          {canRetry ? "Retry update" : "Approve update"}
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
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={installInFlight ? 0 : -1} className="inline-flex">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-11 w-11 cursor-pointer active:scale-95 sm:h-7 sm:w-7"
            aria-label={`Update ${displayName}`}
            disabled={installInFlight}
            onClick={onOpen}
            data-testid={`agent-update-trigger-${agentName}`}
          >
            <IconRefresh className="size-4" />
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        {installInFlight ? "Agent installation is in progress" : `Update ${displayName}`}
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
              <DrawerTitle>Update {displayName}</DrawerTitle>
              <DrawerDescription>
                Review the update before Kandev changes this host runtime.
              </DrawerDescription>
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
              <DialogTitle>Update {displayName}</DialogTitle>
              <DialogDescription>
                Review the update before Kandev changes this host runtime.
              </DialogDescription>
            </DialogHeader>
            {body}
            {footer()}
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
