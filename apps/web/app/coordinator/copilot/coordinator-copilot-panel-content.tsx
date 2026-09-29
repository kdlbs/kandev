"use client";

import type { ReactNode } from "react";
import type { CopilotChip } from "@/hooks/domains/coordinator/copilot-store";
import type { UseCoordinatorCopilotResult } from "./use-coordinator-copilot";
import { CoordinatorCopilotBody } from "./coordinator-copilot-body";
import { CoordinatorCopilotHeader } from "./coordinator-copilot-header";

export type CoordinatorCopilotPanelContentProps = {
  copilot: UseCoordinatorCopilotResult;
  workspaceId: string;
  coordinatorId: string;
  coordinatorName: string;
  onClose: () => void;
  /** Closes without returning focus to the launcher; a chat card's Edit and Reject use it. */
  onClosePopover: () => void;
  /** Header controls before Close. */
  headerActions?: ReactNode;
  /** Overrides the chip the controller holds (the workspace host derives its own). */
  chip?: CopilotChip | null;
  chipRow?: ReactNode;
};

/** The panel's header and conversation body, shared by the Coordinator screens'
 *  copilot and the workspace host; each wraps it in its own launcher and panel. */
export function CoordinatorCopilotPanelContent({
  copilot,
  workspaceId,
  coordinatorId,
  coordinatorName,
  onClose,
  onClosePopover,
  headerActions,
  chip,
  chipRow,
}: CoordinatorCopilotPanelContentProps) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <CoordinatorCopilotHeader
        coordinatorName={coordinatorName}
        busy={copilot.launcher.busy}
        onClose={onClose}
        actions={headerActions}
      />
      <CoordinatorCopilotBody
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        state={copilot.openSequence.state}
        routeSession={copilot.routeSession}
        chip={chip === undefined ? copilot.chip : chip}
        chipRow={chipRow}
        pendingDraft={copilot.pendingDraft}
        askKey={copilot.askKey}
        onRetry={copilot.openSequence.retry}
        onRemoveChip={copilot.removeChip}
        onSuggest={copilot.suggest}
        onClosePopover={onClosePopover}
      />
    </div>
  );
}
