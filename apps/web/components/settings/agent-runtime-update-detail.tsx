"use client";

import type { ReactNode } from "react";
import type { AgentRuntimeUpdateControlProps } from "./agent-runtime-update-control";
import { AgentRuntimeUpdateBody, AgentRuntimeUpdateFooter } from "./agent-runtime-update-content";
import { AgentRuntimeUpdateSurface } from "./agent-runtime-update-surface";
import { useAgentUpdateDialogState } from "./use-agent-update-dialog-state";

export type AgentRuntimeUpdateDetailProps = {
  control: AgentRuntimeUpdateControlProps;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isMobile: boolean;
  onBack?: () => void;
  backLabel?: string;
  /** Runtime policy or ownership content rendered above the update body. */
  beforeBody?: ReactNode;
  /** Fixed-footer content rendered above the update action row. */
  footerBefore?: ReactNode;
  approveDisabledReason?: string;
};

/**
 * The one controlled runtime detail surface. The page-level manager mounts it
 * inside its shared overlay; the standalone control keeps its own trigger.
 */
export function AgentRuntimeUpdateDetail({
  control,
  open,
  onOpenChange,
  isMobile,
  onBack,
  backLabel,
  beforeBody,
  footerBefore,
  approveDisabledReason,
}: AgentRuntimeUpdateDetailProps) {
  const { agentName, displayName, installJob } = control;
  const state = useAgentUpdateDialogState({
    agentName,
    job: control.job,
    open,
    onOpenChange,
    onPreview: control.onPreview,
    onUpdate: control.onUpdate,
  });
  const installInFlight = installJob?.status === "queued" || installJob?.status === "running";
  return (
    <AgentRuntimeUpdateSurface
      managedFallback={control.runtimeUpdate.managed_fallback}
      agentName={agentName}
      displayName={displayName}
      isMobile={isMobile}
      open={open}
      onOpenChange={state.handleOpenChange}
      onBack={onBack}
      backLabel={backLabel}
      body={
        <AgentRuntimeUpdateBody
          before={beforeBody}
          agentName={agentName}
          preview={state.preview}
          loading={state.loading}
          previewError={state.previewError}
          approveError={state.approveError}
          job={state.activeJob}
          onRetryPreview={() =>
            void state.loadPreview(
              state.selectedUseDefault ? undefined : state.selectedTarget,
              state.selectedUseDefault,
              state.selectedFamily,
            )
          }
          selectedTarget={state.selectedTarget}
          onSelectTarget={state.selectTarget}
          selectedUseDefault={state.selectedUseDefault}
          selectedFamily={state.selectedFamily}
          onSelectDefault={state.selectDefault}
          onSelectMigration={state.selectMigration}
          onSelectCurrentRuntime={state.selectCurrentRuntime}
          starting={state.starting}
          isMobile={isMobile}
        />
      }
      footer={(mobile) => (
        <AgentRuntimeUpdateFooter
          agentName={agentName}
          preview={state.preview}
          previewError={state.previewError}
          job={state.activeJob}
          loading={state.loading}
          starting={state.starting}
          installInFlight={installInFlight}
          beforeActions={footerBefore}
          onApprove={() => void state.approve()}
          onClose={() => state.handleOpenChange(false)}
          mobile={mobile}
          canManage={control.canManage}
          blockedReason={approveDisabledReason}
        />
      )}
    />
  );
}
