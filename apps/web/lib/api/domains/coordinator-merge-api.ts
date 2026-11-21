import { ApiError, fetchJson, type ApiRequestOptions } from "@/lib/api/client";
import {
  coordinatorPath,
  mutate,
  type ControlSetting,
  type MergeProposal,
  type WireProposal,
} from "@/lib/api/domains/coordinator-api";

// Mirrors internal/coordinator/mergestate/protection.go's Enforcement.
export type Enforcement = "enforced" | "not_enforced" | "unknown";

// Mirrors internal/coordinator/store_merge_policy.go's ProtectionObservation.
export type ProtectionObservation = {
  repository_id: string;
  branch: string;
  pipeline: Enforcement;
  approvals: Enforcement;
  threads: Enforcement;
  observed_at: string;
};

// Mirrors internal/coordinator/settings_merge.go's MergeRepoDTO.
export type MergeRepo = {
  repository_id: string;
  name: string;
  policy: ControlSetting;
  automatic_since_revision?: number;
  automatic_active: boolean;
};

// Mirrors internal/coordinator/settings_merge.go's MergeSettingsDTO.
export type MergeSettings = {
  repositories: MergeRepo[];
  protection: ProtectionObservation[];
};

// One entry of the settings PUT's merge member.
export type MergeRepoChange = {
  repository_id: string;
  policy: ControlSetting;
  replace_principal?: boolean;
};

// Mirrors internal/coordinator/merge_dependencies.go's DepRef: exactly one member is set.
export type DepRef = { task: string } | { change: string };

export type DepSet = { refs: DepRef[]; revision: number };

// Mirrors internal/coordinator/merge_detail.go.
export type MergeTri = "met" | "unmet" | "unavailable";
export type MergePipeline = "green" | "red" | "pending" | "none" | "unavailable";

export type MergeLive = {
  state: string;
  head_sha: string;
  draft: MergeTri;
  pipeline: MergePipeline;
  mergeable: MergeTri;
  conflict: "present" | "clear" | "unknown";
  unresolved_threads: number;
  approvals: MergeTri;
};

export type MergeDispatchView = {
  outcome: string;
  queued: boolean;
  dispatched_at: string;
  can_check: boolean;
  can_release: boolean;
};

export type MergeDetail = {
  change_key: string;
  repository_id: string;
  head_sha: string;
  method: string;
  automatic: boolean;
  manager_only: boolean;
  note?: string;
  last_reason?: string;
  reason?: string;
  live?: MergeLive;
  dispatch?: MergeDispatchView;
};

// Mirrors internal/coordinator/change_view.go's ChangeView. A missing count is null.
export type ChangeView = {
  key: string;
  provider: string;
  host: string;
  repository_id: string;
  number: number;
  target_branch: string;
  head_sha: string;
  state: string;
  pipeline: MergePipeline;
  mergeable: MergeTri;
  conflict: MergeTri;
  approvals: MergeTri;
  draft: MergeTri;
  unresolved_threads: number | null;
  behind: number | null;
  unavailable: boolean;
  observed_at: string;
};

function taskPath(workspaceId: string, coordinatorId: string, taskId: string, suffix: string) {
  const segment = ["tasks", encodeURIComponent(taskId)].join("/");
  return coordinatorPath(workspaceId, coordinatorId, `/${segment}${suffix}`);
}

function proposalMergePath(
  workspaceId: string,
  coordinatorId: string,
  pid: string,
  action: string,
) {
  return coordinatorPath(
    workspaceId,
    coordinatorId,
    `/proposals/${encodeURIComponent(pid)}/merge/${action}`,
  );
}

export function refreshMergeProtection(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<{ protection: ProtectionObservation[] }> {
  return mutate(
    coordinatorPath(workspaceId, coordinatorId, "/merge/protection/refresh"),
    "POST",
    {},
    options,
  );
}

// The set is read with `revision`; the save names the same number `dependency_revision`.
export async function getDependencies(
  workspaceId: string,
  coordinatorId: string,
  taskId: string,
  options?: ApiRequestOptions,
): Promise<DepSet> {
  const set = await fetchJson<Partial<DepSet> & { dependency_revision?: number }>(
    taskPath(workspaceId, coordinatorId, taskId, "/dependencies"),
    options,
  );
  return { refs: set.refs ?? [], revision: set.revision ?? set.dependency_revision ?? 0 };
}

export async function putDependencies(
  workspaceId: string,
  coordinatorId: string,
  taskId: string,
  revision: number,
  refs: DepRef[],
): Promise<DepSet> {
  const set = await mutate<Partial<DepSet> & { dependency_revision?: number }>(
    taskPath(workspaceId, coordinatorId, taskId, "/dependencies"),
    "PUT",
    { dependency_revision: revision, refs },
  );
  return { refs: set.refs ?? [], revision: set.revision ?? set.dependency_revision ?? revision };
}

// The detail read of a merge proposal runs one uncached provider read.
export function getMergeProposal(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  options?: ApiRequestOptions,
): Promise<MergeProposal> {
  return fetchJson<MergeProposal>(
    coordinatorPath(workspaceId, coordinatorId, `/proposals/${encodeURIComponent(proposalId)}`),
    options,
  );
}

export function checkMergeNow(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  options?: ApiRequestOptions,
): Promise<WireProposal> {
  return mutate<WireProposal>(
    proposalMergePath(workspaceId, coordinatorId, proposalId, "check"),
    "POST",
    {},
    options,
  );
}

export function releaseMerge(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  options?: ApiRequestOptions,
): Promise<WireProposal> {
  return mutate<WireProposal>(
    proposalMergePath(workspaceId, coordinatorId, proposalId, "release"),
    "POST",
    {},
    options,
  );
}

// The linked changes of a card. `retry` bypasses the 60-second reuse. A 404 means
// the backend serves no change state, which callers treat as "none".
export async function getTaskChanges(
  workspaceId: string,
  coordinatorId: string,
  taskId: string,
  retry: boolean,
  options?: ApiRequestOptions,
): Promise<ChangeView[] | null> {
  try {
    const res = await fetchJson<{ changes: ChangeView[] | null }>(
      taskPath(workspaceId, coordinatorId, taskId, `/changes${retry ? "?retry=1" : ""}`),
      options,
    );
    return res.changes ?? [];
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null;
    throw error;
  }
}

/** The closed `code` of a 400 or 409 body, or null for any other error. */
export function mergeErrorCode(error: unknown): string | null {
  if (!(error instanceof ApiError) || (error.status !== 400 && error.status !== 409)) return null;
  const body = error.body as { code?: unknown; error?: unknown } | null;
  if (typeof body?.code === "string") return body.code;
  return typeof body?.error === "string" ? body.error : null;
}
