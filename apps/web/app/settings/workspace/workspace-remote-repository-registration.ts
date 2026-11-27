import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";

/** Request body for `POST /api/v1/workspaces/:id/repositories/remote`. */
export type RemoteRepositoryRegistrationPayload = {
  remote_url: string;
  default_branch?: string;
  provider?: string;
  provider_host?: string;
  provider_scope?: string;
  provider_repo_id?: string;
  provider_owner?: string;
  provider_name?: string;
};

/**
 * Mirrors the task-create locator: GitHub and pasted URLs travel as the bare
 * locator, while other providers keep the descriptor hints the picker already
 * has so the backend can verify them against the provider.
 */
export function remoteRepositoryRegistrationPayload(
  row: TaskRemoteRepoRow,
): RemoteRepositoryRegistrationPayload {
  const payload: RemoteRepositoryRegistrationPayload = {
    remote_url: row.remoteUrl?.trim() || row.url.trim(),
  };
  const branch = row.branch.trim();
  if (branch) payload.default_branch = branch;
  if (!row.provider || row.provider === "github") return payload;
  return {
    ...payload,
    provider: row.provider,
    provider_host: row.providerHost,
    provider_scope: row.providerScope,
    provider_repo_id: row.providerRepoId,
    provider_owner: row.providerOwner,
    provider_name: row.providerName,
  };
}
