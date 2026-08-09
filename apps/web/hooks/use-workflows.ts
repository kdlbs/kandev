import { useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listWorkflows } from "@/lib/api";
import type { WorkflowsState } from "@/lib/state/slices";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";

type StoreWorkflow = WorkflowsState["items"][number];
type SetWorkflows = (workflows: StoreWorkflow[]) => void;

/**
 * Fire-and-forget fetch effect. Kept internal so callers that only need to
 * populate `state.workflows.items` (e.g. `useEnsureWorkspaceWorkflows`) don't
 * also subscribe to the store slice they wrote to — that would re-render the
 * caller on every fetch and defeats the "top-level layout" placement.
 */
function useWorkflowsFetchEffect(
  workspaceId: string | null,
  enabled: boolean,
  requireActiveWorkspace: boolean,
  setWorkflows: SetWorkflows,
  store: StoreApi<AppState>,
) {
  useEffect(() => {
    if (!enabled || !workspaceId) return;
    let cancelled = false;
    const generation = store.getState().workspaceContextGeneration;
    listWorkflows(workspaceId, { cache: "no-store", includeHidden: true })
      .then((response) => {
        const state = store.getState();
        const staleWorkspaceContext = requireActiveWorkspace
          ? !isCurrentWorkspaceContext(state, workspaceId, generation)
          : state.workspaceContextGeneration !== generation;
        if (cancelled || staleWorkspaceContext) {
          return;
        }
        const mapped = response.workflows.map((workflow) => ({
          id: workflow.id,
          workspaceId: workflow.workspace_id,
          name: workflow.name,
          description: workflow.description,
          prompt: workflow.prompt,
          sortOrder: workflow.sort_order ?? 0,
          agent_profile_id: workflow.agent_profile_id,
          hidden: workflow.hidden,
          style: workflow.style,
        }));
        setWorkflows(mapped);
      })
      // Do not clear on error — the sidebar mounts on every route, and boot
      // hydrates workflows before the refresh fires. Blowing the slice away on
      // a network flake would leave the sidebar and board with no workflow IDs
      // until another success. The next successful fetch replaces the slice.
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [enabled, requireActiveWorkspace, setWorkflows, store, workspaceId]);
}

/**
 * Load workflows for the active workspace. Call from a component that stays
 * mounted independently of any collapsible section, so `state.workflows.items`
 * follows the active workspace even when the sidebar's Tasks section is
 * collapsed and its children (which consume workflows) are unmounted.
 */
export function useEnsureWorkspaceWorkflows() {
  const store = useAppStoreApi();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const setWorkflows = useAppStore((state) => state.setWorkflows);
  useWorkflowsFetchEffect(workspaceId, true, true, setWorkflows, store);
}

export function useWorkflows(workspaceId: string | null, enabled = true) {
  const store = useAppStoreApi();
  const workflows = useAppStore((state) => state.workflows.items);
  const setWorkflows = useAppStore((state) => state.setWorkflows);
  useWorkflowsFetchEffect(workspaceId, enabled, false, setWorkflows, store);
  return { workflows };
}
