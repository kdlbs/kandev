"use client";

import { createContext, useContext, type ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { useShallow } from "zustand/react/shallow";
import { isOfficeWorkspace, type WorkspaceItem } from "@/lib/state/slices/workspace/selectors";

/**
 * Which workspace a subtree is looking at, and therefore which mode's chrome it
 * gets.
 *
 * `"unknown"` is a real third state, not a placeholder: every boot passes
 * through a moment where the workspace list has not hydrated, and a consumer
 * that reads that as "not an Office workspace" renders kanban chrome and then
 * visibly swaps. Callers hold instead.
 */
export type WorkspaceMode = "office" | "kanban" | "unknown";

export type WorkspaceScope = {
  workspace: WorkspaceItem | undefined;
  workspaceId: string | null;
  mode: WorkspaceMode;
};

const UNRESOLVED: WorkspaceScope = { workspace: undefined, workspaceId: null, mode: "unknown" };

const WorkspaceScopeContext = createContext<WorkspaceScope | null>(null);

/**
 * Supplies the active workspace to the application subtree.
 *
 * The application has one provider because the store has one active workspace.
 * Keeping this boundary tied to the same atomic store snapshot prevents a
 * mode value from disagreeing with the workspace data that feeds it.
 */
export function WorkspaceScopeProvider({ children }: { children: ReactNode }) {
  const value = useAppStore(
    useShallow((state): WorkspaceScope => {
      const { items, activeId } = state.workspaces;
      const workspace = items.find((item) => item.id === activeId);

      if (!workspace) {
        // An empty workspace list is "not loaded yet". A populated list that
        // does not contain the active workspace is a resolved answer: there is
        // no workspace to render.
        return items.length > 0
          ? { workspace: undefined, workspaceId: null, mode: "kanban" }
          : UNRESOLVED;
      }
      return {
        workspace,
        workspaceId: workspace.id,
        mode: isOfficeWorkspace(workspace) ? "office" : "kanban",
      };
    }),
  );

  return <WorkspaceScopeContext.Provider value={value}>{children}</WorkspaceScopeContext.Provider>;
}

/**
 * The workspace scope for this subtree. Outside a provider it reports
 * `"unknown"` rather than throwing, so an isolated component render in a test
 * behaves like a pre-hydration frame instead of crashing.
 */
export function useWorkspaceScope(): WorkspaceScope {
  return useContext(WorkspaceScopeContext) ?? UNRESOLVED;
}
