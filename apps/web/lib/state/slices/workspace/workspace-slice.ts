import type { StateCreator } from "zustand";
import type { WorkspaceSlice, WorkspaceSliceState } from "./types";

export const defaultWorkspaceState: WorkspaceSliceState = {
  workspaces: { activeId: null },
};

export const createWorkspaceSlice: StateCreator<
  WorkspaceSlice,
  [["zustand/immer", never]],
  [],
  WorkspaceSlice
> = (set, get) => ({
  ...defaultWorkspaceState,
  setActiveWorkspace: (workspaceId) => {
    if (get().workspaces.activeId === workspaceId) {
      return;
    }
    set((draft) => {
      draft.workspaces.activeId = workspaceId;
    });
  },
});
