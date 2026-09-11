import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { RepositorySetPayload } from "@/lib/types/backend";
import type { RepositorySet, RepositorySetItem } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";

/**
 * Keeps the `repositorySets` slice current from the backend's
 * `repository_set.created|updated|deleted` events, so a set defined in one tab or
 * in workspace settings shows up in another tab's create dialog without a reload.
 *
 * Repositories themselves publish no WebSocket events, so there is no existing
 * repository-family handler to extend here.
 */
export function registerRepositorySetsHandlers(store: StoreApi<AppState>): WsHandlers {
  const upsert = (message: { payload: RepositorySetPayload }) => {
    const set = toRepositorySet(message.payload);
    // Without a workspace id there is no slice key to write; an `undefined` key
    // would produce a bucket no reader ever looks at.
    if (!set) return;
    store.getState().upsertRepositorySet(set.workspace_id, set);
  };

  return {
    "repository_set.created": upsert,
    "repository_set.updated": upsert,
    "repository_set.deleted": (message) => {
      const { id, workspace_id: owningWorkspaceId } = message.payload;
      if (!id || !owningWorkspaceId) return;
      store.getState().removeRepositorySet(owningWorkspaceId, id);
    },
  };
}

function toRepositorySet(payload: RepositorySetPayload): RepositorySet | undefined {
  if (!payload.id || !payload.workspace_id) return undefined;
  return {
    // Ids arrive as plain strings on the wire; the brand constructors mark which
    // kind of id each one is.
    id: payload.id,
    workspace_id: workspaceId(payload.workspace_id),
    name: payload.name ?? "",
    description: payload.description ?? "",
    repositories: toRepositorySetItems(payload.repositories),
    created_at: payload.created_at ?? "",
    updated_at: payload.updated_at ?? "",
  };
}

/**
 * Normalizes membership to an array in position order. A set whose repositories
 * were all deleted legitimately has none, and readers index the list without a
 * nil check.
 */
function toRepositorySetItems(items: RepositorySetPayload["repositories"]): RepositorySetItem[] {
  if (!Array.isArray(items)) return [];
  return items
    .filter((entry) => Boolean(entry?.repository_id))
    .map((entry, index) => ({
      repository_id: repositoryId(entry.repository_id),
      position: typeof entry.position === "number" ? entry.position : index,
      base_branch: entry.base_branch ?? "",
    }))
    .sort((left, right) => left.position - right.position);
}
