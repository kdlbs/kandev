// Sentinels for the optional repository binding in the watcher dialogs (Linear,
// Jira, Sentry). The form stores "" to mean "no repository" (repo-less task,
// the historical behaviour) and "" for the base branch to mean "the
// repository's default branch". Radix disallows <SelectItem value="">, so the
// dropdown options carry these sentinel values and map back to "" on change.
//
// Note: there is no "step default" for a repository — a workflow step has no
// repository association — so the empty state means literally "no repository".
// The sentinel values contain ":" which is illegal in Git ref names and never
// appears in a repository UUID, so they cannot collide with a real branch name
// or repository id (which would otherwise let resolve*() silently rewrite a
// user's selection).
export const NO_REPOSITORY = "kandev:no-repository";
export const NO_REPOSITORY_LABEL = "(no repository)";
export const DEFAULT_BRANCH = "kandev:default-branch";
export const DEFAULT_BRANCH_LABEL = "(repository default branch)";

// Map a repository select value back to the stored id, collapsing the sentinel
// to "" so the payload keeps signalling "no repository".
export function resolveRepositoryId(value: string): string {
  return value === NO_REPOSITORY ? "" : value;
}

// Map a branch select value back to the stored branch, collapsing the sentinel
// to "" so the backend fills the repository's default branch at save time.
export function resolveBaseBranch(value: string): string {
  return value === DEFAULT_BRANCH ? "" : value;
}

// clearWorkspaceScopedForm switches a watcher form to a new workspace and clears
// every field scoped to the previous one (workflow, step, and the repository
// binding) so a stale cross-workspace reference can't be saved. Shared across
// the Linear/Jira/Sentry dialogs to keep this data-loss guard in one place.
export function clearWorkspaceScopedForm<
  T extends {
    workspaceId: string;
    workflowId: string;
    workflowStepId: string;
    repositoryId: string;
    baseBranch: string;
  },
>(prev: T, workspaceId: string): T {
  // No-op when the workspace didn't actually change, so re-selecting the current
  // workspace doesn't wipe the user's workflow/step/repository choices.
  if (prev.workspaceId === workspaceId) return prev;
  return {
    ...prev,
    workspaceId,
    workflowId: "",
    workflowStepId: "",
    repositoryId: "",
    baseBranch: "",
  };
}

// branchPlaceholder mirrors the workflow-step placeholder pattern: it nudges the
// user to pick a repository first, then shows a loading hint while branches
// stream in.
export function branchPlaceholder(repositoryId: string, loading: boolean): string {
  if (!repositoryId) return "Pick a repository first";
  if (loading) return "Loading…";
  return DEFAULT_BRANCH_LABEL;
}
