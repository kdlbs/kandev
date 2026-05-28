import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import type { Workspace, ListWorkspacesResponse } from "@/lib/types/http";
import type { AgentProfileId } from "@/lib/types/ids";
import { workspaceId as toWorkspaceId, agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import { qk } from "@/lib/query/keys";

/** Resolves the default_config_agent_profile_id field from the updated event payload. */
function resolveConfigProfileId(
  payload: { default_config_agent_profile_id?: string | null },
  fallback: AgentProfileId | null | undefined,
  payloadHasKey: boolean,
): AgentProfileId | null {
  if (!payloadHasKey) return fallback ?? null;
  return payload.default_config_agent_profile_id
    ? toAgentProfileId(payload.default_config_agent_profile_id)
    : null;
}

/**
 * Registers WS handlers for workspace domain events into the TanStack Query cache.
 *
 * Mirrors lib/ws/handlers/workspaces.ts 1:1, replacing store.setState with
 * queryClient.setQueryData using immutable functional updaters.
 *
 * Events handled:
 *   workspace.created — upsert workspace into the all() list
 *   workspace.updated — patch existing workspace in the all() list
 *   workspace.deleted — remove workspace from the all() list
 *
 * Returns a cleanup function that removes all registered handlers.
 */
export function registerWorkspaceBridge(ws: WebSocketClient, queryClient: QueryClient): () => void {
  const unsubCreated = ws.on("workspace.created", (message) => {
    const payload = message.payload;
    const newWorkspace: Workspace = {
      id: toWorkspaceId(payload.id),
      name: payload.name,
      description: payload.description ?? null,
      owner_id: payload.owner_id ?? "",
      default_executor_id: payload.default_executor_id ?? null,
      default_environment_id: payload.default_environment_id ?? null,
      default_agent_profile_id: payload.default_agent_profile_id
        ? toAgentProfileId(payload.default_agent_profile_id)
        : null,
      default_config_agent_profile_id: payload.default_config_agent_profile_id
        ? toAgentProfileId(payload.default_config_agent_profile_id)
        : null,
      created_at: payload.created_at ?? new Date().toISOString(),
      updated_at: payload.updated_at ?? new Date().toISOString(),
    };

    queryClient.setQueryData<ListWorkspacesResponse>(
      qk.workspaces.all(),
      (prev): ListWorkspacesResponse | undefined => {
        if (!prev) return { workspaces: [newWorkspace], total: 1 };
        const exists = prev.workspaces.some((w) => w.id === payload.id);
        const workspaces = exists
          ? prev.workspaces.map((w) => (w.id === payload.id ? { ...w, ...newWorkspace } : w))
          : [newWorkspace, ...prev.workspaces];
        return { ...prev, workspaces, total: workspaces.length };
      },
    );
  });

  const unsubUpdated = ws.on("workspace.updated", (message) => {
    const payload = message.payload;

    queryClient.setQueryData<ListWorkspacesResponse>(
      qk.workspaces.all(),
      (prev): ListWorkspacesResponse | undefined => {
        if (!prev) return prev;
        return {
          ...prev,
          workspaces: prev.workspaces.map((w): Workspace => {
            if (w.id !== payload.id) return w;
            return {
              ...w,
              name: payload.name,
              description: payload.description ?? w.description,
              default_executor_id: payload.default_executor_id ?? null,
              default_environment_id: payload.default_environment_id ?? null,
              default_agent_profile_id: payload.default_agent_profile_id
                ? toAgentProfileId(payload.default_agent_profile_id)
                : null,
              default_config_agent_profile_id: resolveConfigProfileId(
                payload,
                w.default_config_agent_profile_id,
                "default_config_agent_profile_id" in payload,
              ),
              updated_at: payload.updated_at ?? w.updated_at,
            };
          }),
        };
      },
    );
  });

  const unsubDeleted = ws.on("workspace.deleted", (message) => {
    const id = message.payload.id;

    queryClient.setQueryData<ListWorkspacesResponse>(
      qk.workspaces.all(),
      (prev): ListWorkspacesResponse | undefined => {
        if (!prev) return prev;
        const workspaces = prev.workspaces.filter((w) => w.id !== id);
        return { ...prev, workspaces, total: workspaces.length };
      },
    );
  });

  // Workspace repositories: invalidate the per-workspace repos cache when a
  // repo is created / updated / deleted so dialogs that re-open within the
  // 30s staleTime still see fresh data (e.g. URL-added repos appearing in the
  // workspace dropdown on subsequent task-create dialog opens).
  const invalidateRepos = (workspaceId: string | undefined) => {
    if (!workspaceId) return;
    void queryClient.invalidateQueries({ queryKey: qk.workspaces.repos(workspaceId) });
  };
  const unsubRepoCreated = ws.on("repository.created", (message) => {
    invalidateRepos(message.payload.workspace_id);
  });
  const unsubRepoUpdated = ws.on("repository.updated", (message) => {
    invalidateRepos(message.payload.workspace_id);
  });
  const unsubRepoDeleted = ws.on("repository.deleted", (message) => {
    invalidateRepos(message.payload.workspace_id);
  });

  return () => {
    unsubCreated();
    unsubUpdated();
    unsubDeleted();
    unsubRepoCreated();
    unsubRepoUpdated();
    unsubRepoDeleted();
  };
}
