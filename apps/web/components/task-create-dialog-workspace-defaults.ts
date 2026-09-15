import type { TaskCreateLastUsedSourceApi } from "@/lib/types/http-user-settings";
import type { TaskRepositorySelection } from "@/components/task-create-dialog-types";

/**
 * Rehydrates the backend-owned source snapshot into the dialog's ordered
 * selection union. Empty arrays are intentional and therefore return an empty
 * list instead of the legacy placeholder row.
 */
export function selectionsFromLastUsedSources(
  sources: TaskCreateLastUsedSourceApi[] | undefined,
): TaskRepositorySelection[] {
  return (sources ?? []).flatMap((source, index) => selectionFromLastUsedSource(source, index));
}

function selectionFromLastUsedSource(
  source: TaskCreateLastUsedSourceApi,
  index: number,
): TaskRepositorySelection[] {
  const key = `last-used-${index}`;
  if (source.kind === "folder") return folderSelection(source, key);
  if (source.kind !== "repository") return [];
  if (isRemoteSource(source)) return [remoteSelection(source, key)];
  return localSelection(source, key);
}

function folderSelection(
  source: TaskCreateLastUsedSourceApi,
  key: string,
): TaskRepositorySelection[] {
  if (!source.local_path) return [];
  return [
    {
      kind: "folder",
      key,
      localPath: source.local_path,
      ...(source.display_name ? { displayName: source.display_name } : {}),
    },
  ];
}

function isRemoteSource(source: TaskCreateLastUsedSourceApi): boolean {
  return Boolean(
    source.provider || source.provider_repo_id || source.github_url || source.remote_url,
  );
}

function remoteSelection(
  source: TaskCreateLastUsedSourceApi,
  key: string,
): Extract<TaskRepositorySelection, { kind: "remote" }> {
  return {
    kind: "remote",
    key,
    url: source.github_url ?? source.remote_url ?? "",
    branch: source.checkout_branch ?? source.base_branch ?? "",
    source: source.provider || source.provider_repo_id ? "picker" : "paste",
    remoteUrl: source.remote_url,
    provider: source.provider,
    providerHost: source.provider_host,
    providerScope: source.provider_scope,
    providerRepoId: source.provider_repo_id,
    providerOwner: source.provider_owner,
    providerName: source.provider_name,
    prNumber: source.pr_number,
    prBaseBranch: source.base_branch,
    prHeadBranch: source.checkout_branch,
  };
}

function localSelection(
  source: TaskCreateLastUsedSourceApi,
  key: string,
): TaskRepositorySelection[] {
  if (!source.repository_id && !source.local_path) return [];
  return [
    {
      kind: "local",
      key,
      ...(source.repository_id ? { repositoryId: source.repository_id } : {}),
      ...(source.local_path ? { localPath: source.local_path } : {}),
      branch: source.checkout_branch ?? source.base_branch ?? "",
      ...(source.base_branch ? { baseBranch: source.base_branch } : {}),
      ...(source.branch_policy_id ? { branchPolicyId: source.branch_policy_id } : {}),
      ...(source.checkout_source ? { checkoutSource: source.checkout_source } : {}),
      ...(source.expected_origin ? { expectedOrigin: source.expected_origin } : {}),
    },
  ];
}

export function hasLastUsedWorkspaceSnapshot(
  snapshots: Record<string, TaskCreateLastUsedSourceApi[]>,
  workspaceId: string | null,
): boolean {
  return Boolean(workspaceId && Object.prototype.hasOwnProperty.call(snapshots, workspaceId));
}
