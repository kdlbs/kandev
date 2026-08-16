"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { rememberWorkspaceSelection } from "@/components/app-sidebar/app-sidebar-workspace-navigation";
import type { ModeWorkspace } from "@/lib/state/slices/workspace/selectors";

/**
 * Makes a workspace the active one.
 *
 * The single way to change workspace: it updates the store and persists the
 * choice together, so the two can never disagree — a surface doing only one
 * half leaves the store on one workspace and the cookies on another.
 *
 * Chrome follows the active workspace, so this is also what switches Office and
 * kanban modes; there is no separate mode to set.
 */
export function useSelectWorkspace(): (workspace: ModeWorkspace) => void {
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);

  return useCallback(
    (workspace: ModeWorkspace) => {
      rememberWorkspaceSelection(workspace);
      setActiveWorkspace(workspace.id);
    },
    [setActiveWorkspace],
  );
}
