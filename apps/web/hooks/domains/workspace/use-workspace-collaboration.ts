import { useFeature } from "@/hooks/domains/features/use-feature";
import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import {
  COLLABORATION_ACTIONS,
  readWorkspaceCollaboration,
  putWorkspaceCollaboration,
  type CollaborationAction,
  type CollaborationFlags,
} from "@/lib/api/domains/workspace-collaboration-api";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { hasScope, SCOPE } from "@/lib/types/team-access";

export type CollaborationRow = {
  id: string;
  name: string;
  readable: boolean;
  saved: CollaborationFlags;
  draft: CollaborationFlags;
  saving: boolean;
  error: "denied" | "failed" | null;
};
type EditorState = {
  key: string;
  status: "loading" | "ready" | "denied" | "failed";
  rows: CollaborationRow[];
};
function updateRow(state: EditorState, id: string, patch: Partial<CollaborationRow>): EditorState {
  return { ...state, rows: state.rows.map((row) => (row.id === id ? { ...row, ...patch } : row)) };
}
const emptyFlags = (): CollaborationFlags => ({
  read: false,
  report: false,
  prompt: false,
  control: false,
});
export const sameCollaborationFlags = (a: CollaborationFlags, b: CollaborationFlags) =>
  COLLABORATION_ACTIONS.every((action) => a[action] === b[action]);
const errorKind = (error: unknown) =>
  error instanceof ApiError && error.status === 403 ? "denied" : "failed";

function loadedRows(
  result: Awaited<ReturnType<typeof readWorkspaceCollaboration>>,
  sourceId: string,
  previous: CollaborationRow[],
): CollaborationRow[] {
  const ids = new Set([
    ...result.workspaces.map((w) => w.id),
    ...result.grants.map((g) => g.target_workspace_id),
    ...previous.map((r) => r.id),
  ]);
  ids.delete(sourceId);
  return [...ids].map((id) => {
    const workspace = result.workspaces.find((w) => w.id === id);
    const grant = result.grants.find((g) => g.target_workspace_id === id);
    const prior = previous.find((row) => row.id === id);
    const saved = grant
      ? (Object.fromEntries(
          COLLABORATION_ACTIONS.map((action) => [action, grant[action]]),
        ) as CollaborationFlags)
      : emptyFlags();
    return {
      id,
      name: workspace?.name ?? id,
      readable: hasScope(workspace?.scopes, SCOPE.workspaceRead),
      saved,
      draft: prior && !sameCollaborationFlags(prior.saved, prior.draft) ? prior.draft : saved,
      saving: false,
      error: null,
    };
  });
}

export function useWorkspaceCollaboration(taskId: string, workspaceId: string, enabled: boolean) {
  const store = useAppStoreApi();
  const featureEnabled = useFeature("workspaceCollaboration");
  const active = enabled && featureEnabled;
  const identity = useAppStore(stateReadScopeIdentity);
  const key = JSON.stringify([taskId, workspaceId, active, identity]);
  const latest = useRef(key);
  latest.current = key;
  const [state, setState] = useState<EditorState>({ key, status: "loading", rows: [] });
  const reads = useRef<AbortController | null>(null);
  const writes = useRef(new Map<string, AbortController>());
  const current = useCallback(
    (controller: AbortController) =>
      active &&
      !controller.signal.aborted &&
      latest.current === key &&
      stateReadScopeIdentity(store.getState()) === identity,
    [active, identity, key, store],
  );

  const reload = useCallback(async () => {
    if (!active) return;
    reads.current?.abort();
    const controller = new AbortController();
    if (!current(controller)) return;
    reads.current = controller;
    setState((s) => ({ key, status: "loading", rows: s.key === key ? s.rows : [] }));
    try {
      const result = await readWorkspaceCollaboration(taskId, workspaceId, controller.signal);
      if (current(controller))
        setState((s) => ({
          key,
          status: "ready",
          rows: loadedRows(result, workspaceId, s.key === key ? s.rows : []),
        }));
    } catch (error) {
      if (current(controller)) setState((s) => ({ ...s, key, status: errorKind(error) }));
    }
  }, [active, current, key, taskId, workspaceId]);
  useEffect(() => {
    void reload();
    const pending = writes.current;
    return () => {
      reads.current?.abort();
      for (const controller of pending.values()) controller.abort();
      pending.clear();
    };
  }, [reload]);

  const edit = (id: string, action: CollaborationAction, checked: boolean) => {
    setState((s) =>
      s.key !== key || s.status !== "ready"
        ? s
        : {
            ...s,
            rows: s.rows.map((row) =>
              row.id === id && row.readable
                ? { ...row, draft: { ...row.draft, [action]: checked }, error: null }
                : row,
            ),
          },
    );
  };
  const view = state.key === key ? state : { key, status: "loading" as const, rows: [] };
  const save = async (id: string) => {
    const row = view.rows.find((r) => r.id === id);
    if (
      !active ||
      view.status !== "ready" ||
      !row?.readable ||
      writes.current.has(id) ||
      sameCollaborationFlags(row.saved, row.draft)
    )
      return;
    const submitted = { ...row.draft };
    const controller = new AbortController();
    if (!current(controller)) return;
    writes.current.set(id, controller);
    setState((s) => updateRow(s, id, { saving: true, error: null }));
    try {
      await putWorkspaceCollaboration(taskId, workspaceId, id, submitted, {
        init: { signal: controller.signal },
      });
      if (current(controller))
        setState((s) => updateRow(s, id, { saved: submitted, saving: false, error: null }));
    } catch (error) {
      if (current(controller))
        setState((s) => updateRow(s, id, { saving: false, error: errorKind(error) }));
    } finally {
      if (writes.current.get(id) === controller) writes.current.delete(id);
    }
  };
  return { ...view, enabled: active, edit, save, reload };
}
