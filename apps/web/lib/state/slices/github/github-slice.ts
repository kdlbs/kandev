import type { StateCreator } from "zustand";
import type { GitHubSlice, GitHubSliceState } from "./types";

export const defaultGitHubState: GitHubSliceState = {
  githubStatus: { status: null, loaded: false, loading: false },
  taskPRs: { byTaskId: {} },
  taskIssues: { workspaceId: null, byTaskId: {} },
  pendingPrUrlByTaskId: { byTaskId: {} },
  prWatches: { items: [], loaded: false, loading: false },
  reviewWatches: { items: [], loaded: false, loading: false },
  issueWatches: { items: [], loaded: false, loading: false },
  actionPresets: { byWorkspaceId: {}, loading: {} },
  prFeedbackCache: { byKey: {} },
  taskCIAutomation: { byTaskId: {}, loading: {}, saving: {}, errors: {} },
};

const PR_FEEDBACK_CACHE_LIMIT = 20;

type ImmerSet = Parameters<
  StateCreator<GitHubSlice, [["zustand/immer", never]], [], GitHubSlice>
>[0];

function createGitHubStatusActions(
  set: ImmerSet,
): Pick<GitHubSlice, "setGitHubStatus" | "setGitHubStatusLoading"> {
  return {
    setGitHubStatus: (status) =>
      set((draft) => {
        draft.githubStatus.status = status;
        draft.githubStatus.loaded = true;
      }),
    setGitHubStatusLoading: (loading) =>
      set((draft) => {
        draft.githubStatus.loading = loading;
      }),
  };
}

function clearPendingPrUrlForRepo(draft: GitHubSlice, taskId: string, repoKey: string) {
  const pending = draft.pendingPrUrlByTaskId.byTaskId[taskId];
  if (!pending) return;
  delete pending[repoKey];
  if (Object.keys(pending).length === 0) {
    delete draft.pendingPrUrlByTaskId.byTaskId[taskId];
  }
}

/** Clear client-only pending URLs for the repo that just synced (not sibling repos). */
function clearPendingForTaskPR(
  draft: GitHubSlice,
  taskId: string,
  pr: { repository_id?: string; pr_url?: string },
) {
  clearPendingPrUrlForRepo(draft, taskId, pr.repository_id ?? "");
  clearPendingPrUrlForRepo(draft, taskId, "");
  const pending = draft.pendingPrUrlByTaskId.byTaskId[taskId];
  if (!pending || !pr.pr_url) return;
  for (const key of Object.keys(pending)) {
    if (pending[key] === pr.pr_url) clearPendingPrUrlForRepo(draft, taskId, key);
  }
}

function createTaskPRActions(
  set: ImmerSet,
): Pick<
  GitHubSlice,
  "setTaskPRs" | "setTaskPR" | "setPendingPrUrlForTask" | "setTaskIssues" | "upsertTaskIssue"
> {
  return {
    setTaskPRs: (prs) =>
      set((draft) => {
        draft.taskPRs.byTaskId = prs;
      }),
    setTaskIssues: (workspaceId, issues) =>
      set((draft) => {
        draft.taskIssues.workspaceId = workspaceId;
        draft.taskIssues.byTaskId = issues;
      }),
    upsertTaskIssue: (workspaceId, issue) =>
      set((draft) => {
        if (draft.taskIssues.workspaceId !== workspaceId) return;
        draft.taskIssues.byTaskId[issue.task_id] = issue;
      }),
    setTaskPR: (taskId, pr) =>
      set((draft) => {
        // Upsert by (repository_id, pr_number) so multi-branch tasks can
        // hold N PRs on the same repo as siblings. Keying on
        // repository_id alone collapses every PR for that repo onto one
        // slot — the second WS event silently overwrites the first and
        // the UI shows only the most-recent PR. Legacy rows without a
        // repository_id match on the empty key + pr_number, preserving
        // prior single-PR semantics for single-repo tasks.
        const current = draft.taskPRs.byTaskId[taskId];
        const existing = Array.isArray(current) ? current : [];
        const repoKey = pr.repository_id ?? "";
        const idx = existing.findIndex(
          (p) => (p.repository_id ?? "") === repoKey && p.pr_number === pr.pr_number,
        );
        if (idx >= 0) existing[idx] = pr;
        else existing.push(pr);
        draft.taskPRs.byTaskId[taskId] = existing;
        clearPendingForTaskPR(draft, taskId, pr);
      }),
    setPendingPrUrlForTask: (taskId, repoKey, prUrl) =>
      set((draft) => {
        const trimmed = prUrl.trim();
        if (!trimmed) {
          clearPendingPrUrlForRepo(draft, taskId, repoKey);
          return;
        }
        if (!draft.pendingPrUrlByTaskId.byTaskId[taskId]) {
          draft.pendingPrUrlByTaskId.byTaskId[taskId] = {};
        }
        draft.pendingPrUrlByTaskId.byTaskId[taskId][repoKey] = trimmed;
      }),
  };
}

function createWatchActions(
  set: ImmerSet,
): Pick<
  GitHubSlice,
  | "setPRWatches"
  | "setPRWatchesLoading"
  | "removePRWatch"
  | "setReviewWatches"
  | "setReviewWatchesLoading"
  | "addReviewWatch"
  | "updateReviewWatch"
  | "removeReviewWatch"
  | "setIssueWatches"
  | "setIssueWatchesLoading"
  | "addIssueWatch"
  | "updateIssueWatch"
  | "removeIssueWatch"
> {
  return {
    setPRWatches: (watches) =>
      set((draft) => {
        draft.prWatches.items = watches;
        draft.prWatches.loaded = true;
      }),
    setPRWatchesLoading: (loading) =>
      set((draft) => {
        draft.prWatches.loading = loading;
      }),
    removePRWatch: (id) =>
      set((draft) => {
        draft.prWatches.items = draft.prWatches.items.filter((w) => w.id !== id);
      }),
    setReviewWatches: (watches) =>
      set((draft) => {
        draft.reviewWatches.items = watches;
        draft.reviewWatches.loaded = true;
      }),
    setReviewWatchesLoading: (loading) =>
      set((draft) => {
        draft.reviewWatches.loading = loading;
      }),
    addReviewWatch: (watch) =>
      set((draft) => {
        draft.reviewWatches.items = [
          ...draft.reviewWatches.items.filter((w) => w.id !== watch.id),
          watch,
        ];
        draft.reviewWatches.loaded = true;
      }),
    updateReviewWatch: (watch) =>
      set((draft) => {
        const idx = draft.reviewWatches.items.findIndex((w) => w.id === watch.id);
        if (idx >= 0) {
          draft.reviewWatches.items[idx] = watch;
        }
      }),
    removeReviewWatch: (id) =>
      set((draft) => {
        draft.reviewWatches.items = draft.reviewWatches.items.filter((w) => w.id !== id);
      }),
    setIssueWatches: (watches) =>
      set((draft) => {
        draft.issueWatches.items = watches;
        draft.issueWatches.loaded = true;
      }),
    setIssueWatchesLoading: (loading) =>
      set((draft) => {
        draft.issueWatches.loading = loading;
      }),
    addIssueWatch: (watch) =>
      set((draft) => {
        draft.issueWatches.items = [
          ...draft.issueWatches.items.filter((w) => w.id !== watch.id),
          watch,
        ];
        draft.issueWatches.loaded = true;
      }),
    updateIssueWatch: (watch) =>
      set((draft) => {
        const idx = draft.issueWatches.items.findIndex((w) => w.id === watch.id);
        if (idx >= 0) {
          draft.issueWatches.items[idx] = watch;
        }
      }),
    removeIssueWatch: (id) =>
      set((draft) => {
        draft.issueWatches.items = draft.issueWatches.items.filter((w) => w.id !== id);
      }),
  };
}

function createActionPresetActions(
  set: ImmerSet,
): Pick<GitHubSlice, "setActionPresets" | "setActionPresetsLoading"> {
  return {
    setActionPresets: (workspaceId, presets) =>
      set((draft) => {
        draft.actionPresets.byWorkspaceId[workspaceId] = presets;
      }),
    setActionPresetsLoading: (workspaceId, loading) =>
      set((draft) => {
        draft.actionPresets.loading[workspaceId] = loading;
      }),
  };
}

function createPRFeedbackCacheActions(
  set: ImmerSet,
): Pick<GitHubSlice, "setPRFeedbackCacheEntry" | "removePRFeedbackCacheEntry"> {
  return {
    setPRFeedbackCacheEntry: (key, feedback) =>
      set((draft) => {
        draft.prFeedbackCache.byKey[key] = { feedback, lastUpdatedAt: Date.now() };
        // Bound cache size: drop the oldest entries when over the limit so a
        // user opening many PRs doesn't grow the slice unboundedly.
        const entries = Object.entries(draft.prFeedbackCache.byKey);
        if (entries.length > PR_FEEDBACK_CACHE_LIMIT) {
          entries.sort((a, b) => a[1].lastUpdatedAt - b[1].lastUpdatedAt);
          const drop = entries.length - PR_FEEDBACK_CACHE_LIMIT;
          for (let i = 0; i < drop; i++) {
            delete draft.prFeedbackCache.byKey[entries[i][0]];
          }
        }
      }),
    removePRFeedbackCacheEntry: (key) =>
      set((draft) => {
        delete draft.prFeedbackCache.byKey[key];
      }),
  };
}

function createTaskCIAutomationActions(
  set: ImmerSet,
): Pick<
  GitHubSlice,
  | "setTaskCIAutomationOptions"
  | "setTaskCIAutomationLoading"
  | "setTaskCIAutomationSaving"
  | "setTaskCIAutomationError"
> {
  return {
    setTaskCIAutomationOptions: (taskId, options) =>
      set((draft) => {
        draft.taskCIAutomation.byTaskId[taskId] = options;
      }),
    setTaskCIAutomationLoading: (taskId, loading) =>
      set((draft) => {
        draft.taskCIAutomation.loading[taskId] = loading;
      }),
    setTaskCIAutomationSaving: (taskId, saving) =>
      set((draft) => {
        draft.taskCIAutomation.saving[taskId] = saving;
      }),
    setTaskCIAutomationError: (taskId, error) =>
      set((draft) => {
        draft.taskCIAutomation.errors[taskId] = error;
      }),
  };
}

function createRateLimitActions(set: ImmerSet): Pick<GitHubSlice, "applyGitHubRateLimitUpdate"> {
  return {
    applyGitHubRateLimitUpdate: (update) =>
      set((draft) => {
        const existing = draft.githubStatus.status;
        if (!existing) {
          // Status not yet hydrated; defer until the SSR/HTTP fetch lands.
          return;
        }
        const rateLimit = { ...(existing.rate_limit ?? {}) };
        for (const snap of update.snapshots) {
          rateLimit[snap.resource] = snap;
        }
        draft.githubStatus.status = { ...existing, rate_limit: rateLimit };
      }),
  };
}

export const createGitHubSlice: StateCreator<
  GitHubSlice,
  [["zustand/immer", never]],
  [],
  GitHubSlice
> = (set) => ({
  ...defaultGitHubState,
  ...createGitHubStatusActions(set),
  ...createTaskPRActions(set),
  ...createWatchActions(set),
  ...createActionPresetActions(set),
  ...createRateLimitActions(set),
  ...createPRFeedbackCacheActions(set),
  ...createTaskCIAutomationActions(set),
});
