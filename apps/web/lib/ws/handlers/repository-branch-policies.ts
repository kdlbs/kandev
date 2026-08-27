import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { RepositoryBranchPolicyPayload } from "@/lib/types/backend";
import type { RepositoryBranchPolicy } from "@/lib/types/http";
import { repositoryId } from "@/lib/types/ids";

export function registerRepositoryBranchPoliciesHandlers(store: StoreApi<AppState>): WsHandlers {
  const upsert = (message: { payload: RepositoryBranchPolicyPayload }) => {
    const policy = toRepositoryBranchPolicy(message.payload);
    if (policy) store.getState().upsertRepositoryBranchPolicy(policy);
  };

  return {
    "repository_branch_policy.created": upsert,
    "repository_branch_policy.updated": upsert,
    "repository_branch_policy.deleted": (message) => {
      const { id, repository_id: repositoryId } = message.payload;
      if (id && repositoryId) store.getState().removeRepositoryBranchPolicy(repositoryId, id);
    },
  };
}

function toRepositoryBranchPolicy(
  payload: RepositoryBranchPolicyPayload,
): RepositoryBranchPolicy | undefined {
  if (!payload.id || !payload.repository_id) return undefined;
  return {
    id: payload.id,
    repository_id: repositoryId(payload.repository_id),
    name: payload.name ?? "",
    description: payload.description ?? "",
    base_branch: payload.base_branch ?? "",
    branch_template: payload.branch_template ?? "",
    pull_request_target: payload.pull_request_target ?? "",
    created_at: payload.created_at ?? "",
    updated_at: payload.updated_at ?? "",
  };
}
