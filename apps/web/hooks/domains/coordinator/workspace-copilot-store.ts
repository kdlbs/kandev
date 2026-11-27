import { create, type StoreApi, type UseBoundStore } from "zustand";
import {
  INITIAL_SLOT,
  createCopilotSlot,
  type CopilotStoreState,
} from "@/hooks/domains/coordinator/copilot-store";

export type RightPanelOwner = "preview" | "copilot" | null;

export type WorkspaceCopilotState = CopilotStoreState & {
  /** The workspace the slot was opened in; null before the first open. */
  workspaceId: string | null;
  /** Route key whose page chip the user removed; cleared when the key changes. */
  chipDismissedFor: string | null;
  /** Which panel holds the single right-hand slot. */
  rightPanel: RightPanelOwner;
  openFor: (workspaceId: string, coordinatorId: string) => void;
  /** Gives `coordinatorId` a fresh slot, open, keeping the workspace. */
  switchTo: (coordinatorId: string) => void;
  claimPreview: () => void;
  /** Nulls the claim only while the preview holds it. */
  releasePreview: () => void;
  dismissChip: (routeKey: string) => void;
  clearDismissalUnless: (routeKey: string) => void;
  /** Back to the initial value; a preview claim is not the copilot's to drop. */
  reset: () => void;
};

export type WorkspaceCopilotStore = UseBoundStore<StoreApi<WorkspaceCopilotState>>;

/** `open` and `rightPanel` change in one write: opening claims the panel, and
 *  closing releases it unless the preview took it over. */
function withPanelClaim(
  prev: WorkspaceCopilotState,
  patch: Partial<WorkspaceCopilotState>,
): Partial<WorkspaceCopilotState> {
  const open = patch.open ?? prev.open;
  if (open === prev.open) return patch;
  if (open) return { ...patch, rightPanel: "copilot" };
  return prev.rightPanel === "copilot" ? { ...patch, rightPanel: null } : patch;
}

export function createWorkspaceCopilotStore(): WorkspaceCopilotStore {
  return create<WorkspaceCopilotState>()((set, get) => {
    const slotSet = (fn: (state: CopilotStoreState) => Partial<CopilotStoreState>) =>
      set((state) => {
        const patch = fn(state);
        return patch === state ? state : withPanelClaim(state, patch);
      });
    return {
      ...createCopilotSlot(slotSet, get),
      workspaceId: null,
      chipDismissedFor: null,
      rightPanel: null,
      openFor: (workspaceId, coordinatorId) =>
        set((state) => {
          const slot = state.coordinatorId === coordinatorId ? state : INITIAL_SLOT;
          return withPanelClaim(state, { ...slot, coordinatorId, open: true, workspaceId });
        }),
      switchTo: (coordinatorId) =>
        set((state) => ({
          ...INITIAL_SLOT,
          coordinatorId,
          open: true,
          rightPanel: "copilot",
          workspaceId: state.workspaceId,
          chipDismissedFor: state.chipDismissedFor,
        })),
      claimPreview: () => set({ rightPanel: "preview", open: false }),
      releasePreview: () =>
        set((state) => (state.rightPanel === "preview" ? { rightPanel: null } : state)),
      dismissChip: (routeKey) => set({ chipDismissedFor: routeKey }),
      clearDismissalUnless: (routeKey) =>
        set((state) =>
          state.chipDismissedFor === null || state.chipDismissedFor === routeKey
            ? state
            : { chipDismissedFor: null },
        ),
      reset: () =>
        set((state) => ({
          ...INITIAL_SLOT,
          workspaceId: null,
          chipDismissedFor: null,
          rightPanel: state.rightPanel === "preview" ? "preview" : null,
        })),
    };
  });
}
