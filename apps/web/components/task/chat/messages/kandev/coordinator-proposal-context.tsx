"use client";

import { createContext, useContext, type ReactNode } from "react";

export type CoordinatorProposalContextValue = {
  workspaceId: string;
  coordinatorId: string;
  /** Closes the copilot popover, for the chat card's Edit/Reject navigation (proposal-cards.md#cards "Forms and navigation"). */
  closePopover: () => void;
};

const CoordinatorProposalContext = createContext<CoordinatorProposalContextValue | undefined>(
  undefined,
);

export function CoordinatorProposalProvider({
  value,
  children,
}: {
  value: CoordinatorProposalContextValue;
  children: ReactNode;
}) {
  return (
    <CoordinatorProposalContext.Provider value={value}>
      {children}
    </CoordinatorProposalContext.Provider>
  );
}

/**
 * Undefined outside the coordinator copilot transcript. Only a coordinator
 * agent can call `propose_task_kandev`, and the popover the chat card
 * renders inside is itself gated on `workspace.manage`, so the renderer
 * never has to handle a real reader viewing it.
 */
export function useCoordinatorProposalContext(): CoordinatorProposalContextValue | undefined {
  return useContext(CoordinatorProposalContext);
}
