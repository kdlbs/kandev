import type { ControlSetting } from "@/lib/api/domains/coordinator-api";
import type {
  Enforcement,
  MergeRepo,
  MergeRepoChange,
  ProtectionObservation,
} from "@/lib/api/domains/coordinator-merge-api";

/** A manager's unsaved edit of one repository. */
export type MergeEdit = { policy: ControlSetting; replacePrincipal: boolean };

/** Unsaved edits by repository ID; a repository without an entry is unchanged. */
export type MergeDraft = Record<string, MergeEdit>;

export const MERGE_POLICIES: readonly ControlSetting[] = [
  "denied",
  "requires_approval",
  "automatic",
];

export function effectivePolicy(repo: MergeRepo, draft: MergeDraft): ControlSetting {
  return draft[repo.repository_id]?.policy ?? repo.policy;
}

/** True when the repository's automatic principal is no longer authorized. */
export function isPrincipalInactive(repo: MergeRepo): boolean {
  return repo.policy === "automatic" && !repo.automatic_active;
}

function isEffectiveEdit(repo: MergeRepo, edit: MergeEdit): boolean {
  return edit.policy !== repo.policy || (edit.replacePrincipal && isPrincipalInactive(repo));
}

/** Sets a repository's policy; choosing the stored policy again drops the edit. */
export function setPolicy(draft: MergeDraft, repo: MergeRepo, policy: ControlSetting): MergeDraft {
  const next = { ...draft };
  const replacePrincipal = draft[repo.repository_id]?.replacePrincipal ?? false;
  const edit: MergeEdit = { policy, replacePrincipal };
  if (isEffectiveEdit(repo, edit)) next[repo.repository_id] = edit;
  else delete next[repo.repository_id];
  return next;
}

/** Marks the repository's inactive automatic principal for replacement on save. */
export function markReplacePrincipal(draft: MergeDraft, repo: MergeRepo): MergeDraft {
  return { ...draft, [repo.repository_id]: { policy: repo.policy, replacePrincipal: true } };
}

/** Drops every edit the stored state already satisfies, for a re-read. */
export function pruneDraft(draft: MergeDraft, repos: MergeRepo[]): MergeDraft {
  const byId = new Map(repos.map((r) => [r.repository_id, r]));
  const next: MergeDraft = {};
  for (const [id, edit] of Object.entries(draft)) {
    const repo = byId.get(id);
    if (repo && isEffectiveEdit(repo, edit)) next[id] = edit;
  }
  return next;
}

export function isMergeDirty(draft: MergeDraft, repos: MergeRepo[]): boolean {
  return repos.some((r) => {
    const edit = draft[r.repository_id];
    return edit !== undefined && isEffectiveEdit(r, edit);
  });
}

/** The repositories the save lists: only those that differ, in repository order. */
export function buildMergeChanges(draft: MergeDraft, repos: MergeRepo[]): MergeRepoChange[] {
  const changes: MergeRepoChange[] = [];
  for (const repo of repos) {
    const edit = draft[repo.repository_id];
    if (!edit || !isEffectiveEdit(repo, edit)) continue;
    const change: MergeRepoChange = { repository_id: repo.repository_id, policy: edit.policy };
    if (edit.replacePrincipal && isPrincipalInactive(repo)) change.replace_principal = true;
    changes.push(change);
  }
  return changes;
}

/** Observations of one repository, ascending by branch name. */
export function protectionOf(
  protection: ProtectionObservation[],
  repositoryId: string,
): ProtectionObservation[] {
  return protection
    .filter((o) => o.repository_id === repositoryId)
    .sort((a, b) => Number(a.branch > b.branch) - Number(a.branch < b.branch));
}

export function hasUnknownProtection(observations: ProtectionObservation[]): boolean {
  const unknown: Enforcement = "unknown";
  return observations.some(
    (o) => o.pipeline === unknown || o.approvals === unknown || o.threads === unknown,
  );
}

/** True when the repos a change touches were stored identically in two reads. */
export function sameStoredPolicies(
  ids: string[],
  before: MergeRepo[],
  after: MergeRepo[],
): boolean {
  const pick = (repos: MergeRepo[], id: string) => repos.find((r) => r.repository_id === id);
  return ids.every((id) => {
    const a = pick(before, id);
    const b = pick(after, id);
    return !!a && !!b && a.policy === b.policy && a.automatic_active === b.automatic_active;
  });
}
