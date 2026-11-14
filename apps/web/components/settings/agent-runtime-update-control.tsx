"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  AgentUpdateJob,
  AgentUpdateMode,
  AgentUpdatePreview,
  AgentUpdateStatus,
  InstallJob,
} from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";
import { AgentRuntimeUpdateBody, AgentRuntimeUpdateFooter } from "./agent-runtime-update-content";
import { AgentRuntimeUpdateSurface } from "./agent-runtime-update-surface";
import { useAgentUpdateDialogState } from "./use-agent-update-dialog-state";

const UPDATE_AGENT_KEY = "agents:updateAgent";

export type UpdateTriggerProps = {
  agentName: string;
  displayName: string;
  runtimeUpdateStatus?: AgentUpdateStatus;
  installInFlight: boolean;
  onOpen: (opener: HTMLElement) => void;
};

export function UpdateTrigger({
  agentName,
  displayName,
  runtimeUpdateStatus,
  installInFlight,
  onOpen,
}: UpdateTriggerProps) {
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
            onClick={(event) => onOpen(event.currentTarget)}
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

export type AgentRuntimeUpdateControlProps = {
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
  /**
   * When supplied, the page owns the update surface and the control renders
   * only its trigger. This keeps installed cards from mounting a second
   * overlay next to the shared runtime manager.
   */
  onOpen?: (opener: HTMLElement) => void;
  /** Read-only operators keep inspection but cannot admit an update. */
  canManage?: boolean;
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
    <AgentRuntimeUpdateBody
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
    <AgentRuntimeUpdateFooter
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
      canManage={control.canManage}
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
  if (!props.runtimeUpdate.supported) return null;
  if (props.onOpen) {
    return (
      <UpdateTrigger
        agentName={props.agentName}
        displayName={props.displayName}
        runtimeUpdateStatus={props.runtimeUpdateStatus}
        installInFlight={
          props.installJob?.status === "queued" || props.installJob?.status === "running"
        }
        onOpen={(opener) => props.onOpen?.(opener)}
      />
    );
  }
  return <StandaloneAgentRuntimeUpdateControl {...props} isMobile={isMobile} />;
}

function StandaloneAgentRuntimeUpdateControl(
  props: AgentRuntimeUpdateControlProps & { isMobile: boolean },
) {
  const state = useAgentUpdateDialogState({
    agentName: props.agentName,
    job: props.job,
    onPreview: props.onPreview,
    onUpdate: props.onUpdate,
  });
  return <AgentRuntimeUpdateDialogView control={props} state={state} isMobile={props.isMobile} />;
}
