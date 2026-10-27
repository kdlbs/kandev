import { useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import type { SidebarInventory } from "@/lib/state/slices/task-overview-coverage";
import { sidebarTaskSource } from "@/lib/sidebar/sidebar-task-source";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { resolveSidebarTaskView, type SidebarTaskQueryOverrides } from "./use-sidebar-page-context";

export function selectSidebarStoreTasks(
  state: SidebarInventory,
  workspaceId: string | null,
  view: SidebarView = DEFAULT_VIEW,
  queryOverrides?: SidebarTaskQueryOverrides,
) {
  const resultView = resolveSidebarTaskView(view, queryOverrides);
  return (
    sidebarTaskSource(state, workspaceId, resultView)?.map((task) => ({
      ...task,
      _workflowId: task.workflowId,
    })) ?? null
  );
}

export function useSidebarStoreTasks(
  workspaceId: string | null,
  queryOverrides?: SidebarTaskQueryOverrides,
) {
  const view = useEffectiveSidebarView(workspaceId);
  const inventory = useAppStore(
    useShallow((state) => ({
      workspaces: state.workspaces,
      workflows: state.workflows,
      kanbanMulti: state.kanbanMulti,
      workspaceContextRead: state.workspaceContextRead,
      workspaceContextGeneration: state.workspaceContextGeneration,
      auth: state.auth,
      repositories: state.repositories,
      userSettings: state.userSettings,
    })),
  );
  return useMemo(
    () => selectSidebarStoreTasks(inventory, workspaceId, view, queryOverrides),
    [inventory, workspaceId, view, queryOverrides],
  );
}
