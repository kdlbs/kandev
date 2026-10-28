import type { SliceCreator } from "@/lib/state/slice-creator";

import type { GitLabSlice, GitLabSliceState } from "./types";
import { normalizeGitLabOrigin } from "@/lib/gitlab-identity";
import type { TaskMR } from "@/lib/types/gitlab";

export const defaultGitLabState: GitLabSliceState = {
  taskMRs: { byWorkspaceId: {}, deletedAssociationRevisionsByWorkspaceId: {} },
  gitlabReviewWatches: { items: [], loaded: false, loading: false },
  gitlabIssueWatches: { items: [], loaded: false, loading: false },
  gitlabMRWatches: { items: [], loaded: false, loading: false },
  gitlabActionPresets: { byWorkspaceId: {}, loading: false },
  gitlabStatus: { byWorkspaceId: {} },
  taskMRAutomation: { byTaskId: {}, loading: {}, saving: {}, errors: {}, externalGeneration: {} },
};

type ImmerSet = Parameters<SliceCreator<GitLabSlice>>[0];

type GitLabSliceCreator = SliceCreator<GitLabSlice>;

export const createGitLabSlice: GitLabSliceCreator = (set: ImmerSet) => ({
  ...defaultGitLabState,
  ...taskMRActions(set),
  ...reviewWatchActions(set),
  ...issueWatchActions(set),
  ...mrWatchActions(set),
  ...presetActions(set),
  ...statusActions(set),
  ...taskMRAutomationActions(set),
});

function taskMRActions(set: ImmerSet) {
  return {
    setTaskMRs: (workspaceId: string, mrs: GitLabSliceState["taskMRs"]["byWorkspaceId"][string]) =>
      set((draft) => {
        const tombstones = taskMRDeletionMap(draft, workspaceId);
        const current = draft.taskMRs.byWorkspaceId[workspaceId] ?? {};
        const reconciled: Record<string, TaskMR[]> = {};
        for (const [taskId, incoming] of Object.entries(mrs)) {
          const existing = current[taskId] ?? [];
          const next: TaskMR[] = [];
          for (const mr of incoming) {
            const revision = taskMRAssociationRevision(mr);
            const deletedRevision = tombstones[mr.id];
            if (deletedRevision !== undefined && revision <= deletedRevision) continue;
            if (deletedRevision !== undefined) delete tombstones[mr.id];
            const currentMR = existing.find((item) => sameTaskMRIdentity(item, mr));
            next.push(
              currentMR && taskMRAssociationRevision(currentMR) > revision ? currentMR : mr,
            );
          }
          if (next.length > 0) reconciled[taskId] = next;
        }
        draft.taskMRs.byWorkspaceId[workspaceId] = reconciled;
      }),
    setTaskMR: (
      workspaceId: string,
      taskId: string,
      mr: GitLabSliceState["taskMRs"]["byWorkspaceId"][string][string][number],
    ) =>
      set((draft) => {
        const tombstones = taskMRDeletionMap(draft, workspaceId);
        const revision = taskMRAssociationRevision(mr);
        const deletedRevision = tombstones[mr.id];
        if (deletedRevision !== undefined && revision <= deletedRevision) return;
        const workspaceMRs = draft.taskMRs.byWorkspaceId[workspaceId] ?? {};
        const existing = workspaceMRs[taskId] ?? [];
        const idx = existing.findIndex((item) => sameTaskMRIdentity(item, mr));
        if (idx >= 0 && taskMRAssociationRevision(existing[idx]!) > revision) return;
        if (deletedRevision !== undefined) delete tombstones[mr.id];
        if (idx >= 0) existing[idx] = mr;
        else existing.push(mr);
        workspaceMRs[taskId] = existing;
        draft.taskMRs.byWorkspaceId[workspaceId] = workspaceMRs;
      }),
    removeTaskMR: (workspaceId: string, associationId: string, associationRevision = 0) =>
      set((draft) => {
        const workspaceMRs = draft.taskMRs.byWorkspaceId[workspaceId];
        const tombstones = taskMRDeletionMap(draft, workspaceId);
        const revision = Number.isFinite(associationRevision)
          ? Math.max(0, associationRevision)
          : 0;
        const current = workspaceMRs
          ? Object.values(workspaceMRs)
              .flat()
              .find((mr) => mr.id === associationId)
          : undefined;
        if (current && taskMRAssociationRevision(current) > revision) return;
        if ((tombstones[associationId] ?? -1) > revision) return;
        tombstones[associationId] = Math.max(tombstones[associationId] ?? 0, revision);
        if (!workspaceMRs) return;
        for (const taskId of Object.keys(workspaceMRs)) {
          workspaceMRs[taskId] = (workspaceMRs[taskId] ?? []).filter(
            (mr) => mr.id !== associationId || taskMRAssociationRevision(mr) > revision,
          );
        }
      }),
    resetTaskMRs: (workspaceId?: string) =>
      set((draft) => {
        if (workspaceId) {
          delete draft.taskMRs.byWorkspaceId[workspaceId];
          if (draft.taskMRs.deletedAssociationRevisionsByWorkspaceId) {
            delete draft.taskMRs.deletedAssociationRevisionsByWorkspaceId[workspaceId];
          }
          return;
        }
        draft.taskMRs.byWorkspaceId = {};
        draft.taskMRs.deletedAssociationRevisionsByWorkspaceId = {};
      }),
  };
}

function taskMRAssociationRevision(mr: TaskMR): number {
  return Number.isFinite(mr.association_revision) ? Math.max(0, mr.association_revision ?? 0) : 0;
}

function sameTaskMRIdentity(left: TaskMR, right: TaskMR): boolean {
  return (
    (left.repository_id ?? "") === (right.repository_id ?? "") &&
    normalizeGitLabOrigin(left.host) === normalizeGitLabOrigin(right.host) &&
    left.project_path === right.project_path &&
    left.mr_iid === right.mr_iid
  );
}

function taskMRDeletionMap(state: GitLabSlice, workspaceId: string): Record<string, number> {
  state.taskMRs.deletedAssociationRevisionsByWorkspaceId ??= {};
  return (state.taskMRs.deletedAssociationRevisionsByWorkspaceId[workspaceId] ??= {});
}

function reviewWatchActions(set: ImmerSet) {
  return {
    setGitLabReviewWatches: (watches: GitLabSliceState["gitlabReviewWatches"]["items"]) =>
      set((draft) => {
        draft.gitlabReviewWatches.items = watches;
        draft.gitlabReviewWatches.loaded = true;
      }),
    setGitLabReviewWatchesLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabReviewWatches.loading = loading;
      }),
    addGitLabReviewWatch: (watch: GitLabSliceState["gitlabReviewWatches"]["items"][number]) =>
      set((draft) => {
        draft.gitlabReviewWatches.items = [...draft.gitlabReviewWatches.items, watch];
      }),
    updateGitLabReviewWatchInStore: (
      watch: GitLabSliceState["gitlabReviewWatches"]["items"][number],
    ) =>
      set((draft) => {
        draft.gitlabReviewWatches.items = draft.gitlabReviewWatches.items.map((w) =>
          w.id === watch.id ? watch : w,
        );
      }),
    removeGitLabReviewWatch: (id: string) =>
      set((draft) => {
        draft.gitlabReviewWatches.items = draft.gitlabReviewWatches.items.filter(
          (w) => w.id !== id,
        );
      }),
  };
}

function issueWatchActions(set: ImmerSet) {
  return {
    setGitLabIssueWatches: (watches: GitLabSliceState["gitlabIssueWatches"]["items"]) =>
      set((draft) => {
        draft.gitlabIssueWatches.items = watches;
        draft.gitlabIssueWatches.loaded = true;
      }),
    setGitLabIssueWatchesLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabIssueWatches.loading = loading;
      }),
    addGitLabIssueWatch: (watch: GitLabSliceState["gitlabIssueWatches"]["items"][number]) =>
      set((draft) => {
        draft.gitlabIssueWatches.items = [...draft.gitlabIssueWatches.items, watch];
      }),
    updateGitLabIssueWatchInStore: (
      watch: GitLabSliceState["gitlabIssueWatches"]["items"][number],
    ) =>
      set((draft) => {
        draft.gitlabIssueWatches.items = draft.gitlabIssueWatches.items.map((w) =>
          w.id === watch.id ? watch : w,
        );
      }),
    removeGitLabIssueWatch: (id: string) =>
      set((draft) => {
        draft.gitlabIssueWatches.items = draft.gitlabIssueWatches.items.filter((w) => w.id !== id);
      }),
  };
}

function mrWatchActions(set: ImmerSet) {
  return {
    setGitLabMRWatches: (watches: GitLabSliceState["gitlabMRWatches"]["items"]) =>
      set((draft) => {
        draft.gitlabMRWatches.items = watches;
        draft.gitlabMRWatches.loaded = true;
      }),
    setGitLabMRWatchesLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabMRWatches.loading = loading;
      }),
    removeGitLabMRWatch: (id: string) =>
      set((draft) => {
        draft.gitlabMRWatches.items = draft.gitlabMRWatches.items.filter((w) => w.id !== id);
      }),
  };
}

function presetActions(set: ImmerSet) {
  return {
    setGitLabActionPresets: (
      workspaceId: string,
      presets: GitLabSliceState["gitlabActionPresets"]["byWorkspaceId"][string],
    ) =>
      set((draft) => {
        draft.gitlabActionPresets.byWorkspaceId[workspaceId] = presets;
      }),
    setGitLabActionPresetsLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabActionPresets.loading = loading;
      }),
  };
}

function statusActions(set: ImmerSet) {
  return {
    setGitLabStatus: (
      workspaceId: string,
      status: GitLabSliceState["gitlabStatus"]["byWorkspaceId"][string]["data"],
    ) =>
      set((draft) => {
        const entry =
          draft.gitlabStatus.byWorkspaceId[workspaceId] ??
          (draft.gitlabStatus.byWorkspaceId[workspaceId] = {
            data: null,
            loading: false,
            loadedAt: null,
          });
        entry.data = status;
        entry.loadedAt = Date.now();
      }),
    setGitLabStatusLoading: (workspaceId: string, loading: boolean) =>
      set((draft) => {
        const entry =
          draft.gitlabStatus.byWorkspaceId[workspaceId] ??
          (draft.gitlabStatus.byWorkspaceId[workspaceId] = {
            data: null,
            loading: false,
            loadedAt: null,
          });
        entry.loading = loading;
      }),
    resetGitLabStatus: (workspaceId: string) =>
      set((draft) => {
        draft.gitlabStatus.byWorkspaceId[workspaceId] = {
          data: null,
          loading: false,
          loadedAt: null,
        };
      }),
  };
}

function taskMRAutomationActions(set: ImmerSet) {
  return {
    setTaskMRAutomationOptions: (
      taskId: string,
      options: GitLabSliceState["taskMRAutomation"]["byTaskId"][string],
    ) =>
      set((draft) => {
        const existing = draft.taskMRAutomation.byTaskId[taskId];
        if (existing && (options.automation_revision ?? 0) < (existing.automation_revision ?? 0)) {
          return;
        }
        draft.taskMRAutomation.byTaskId[taskId] = options;
      }),
    setTaskMRAutomationLoading: (taskId: string, loading: boolean) =>
      set((draft) => {
        draft.taskMRAutomation.loading[taskId] = loading;
      }),
    setTaskMRAutomationSaving: (taskId: string, saving: boolean) =>
      set((draft) => {
        draft.taskMRAutomation.saving[taskId] = saving;
      }),
    setTaskMRAutomationError: (taskId: string, error: string | null) =>
      set((draft) => {
        draft.taskMRAutomation.errors[taskId] = error;
      }),
    markTaskMRAutomationExternalUpdate: (taskId: string) =>
      set((draft) => {
        draft.taskMRAutomation.externalGeneration[taskId] =
          (draft.taskMRAutomation.externalGeneration[taskId] ?? 0) + 1;
      }),
  };
}
