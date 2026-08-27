import type { StateCreator } from "zustand";
import type { WorkspaceSlice, WorkspaceSliceState } from "./types";
import type { RepositoryBranchPolicy, RepositorySet } from "@/lib/types/http";

export const defaultWorkspaceState: WorkspaceSliceState = {
  workspaces: { items: [], activeId: null },
  repositories: { itemsByWorkspaceId: {}, loadingByWorkspaceId: {}, loadedByWorkspaceId: {} },
  repositorySets: {
    itemsByWorkspaceId: {},
    loadingByWorkspaceId: {},
    loadedByWorkspaceId: {},
    revisionByWorkspaceId: {},
  },
  repositoryBranchPolicies: {
    itemsByRepositoryId: {},
    loadingByRepositoryId: {},
    loadedByRepositoryId: {},
    revisionByRepositoryId: {},
  },
  repositoryBranches: {
    itemsByRepositoryId: {},
    loadingByRepositoryId: {},
    loadedByRepositoryId: {},
    fetchedAtByRepositoryId: {},
    fetchErrorByRepositoryId: {},
  },
  repositoryScripts: {
    itemsByRepositoryId: {},
    loadingByRepositoryId: {},
    loadedByRepositoryId: {},
  },
};

export const createWorkspaceSlice: StateCreator<
  WorkspaceSlice,
  [["zustand/immer", never]],
  [],
  WorkspaceSlice
> = (set, get) => ({
  ...defaultWorkspaceState,
  setActiveWorkspace: (workspaceId) => {
    if (get().workspaces.activeId === workspaceId) {
      return;
    }
    set((draft) => {
      draft.workspaces.activeId = workspaceId;
    });
  },
  setWorkspaces: (workspaces) =>
    set((draft) => {
      draft.workspaces.items = workspaces;
      if (!draft.workspaces.activeId && workspaces.length) {
        draft.workspaces.activeId = workspaces[0].id;
      }
    }),
  setRepositories: (workspaceId, repositories) =>
    set((draft) => {
      draft.repositories.itemsByWorkspaceId[workspaceId] = repositories;
      draft.repositories.loadingByWorkspaceId[workspaceId] = false;
      draft.repositories.loadedByWorkspaceId[workspaceId] = true;
    }),
  upsertRepository: (workspaceId, repository) =>
    set((draft) => {
      const repositories = draft.repositories.itemsByWorkspaceId[workspaceId] ?? [];
      const loaded = draft.repositories.loadedByWorkspaceId[workspaceId];
      const existingIndex = repositories.findIndex((candidate) => candidate.id === repository.id);
      if (existingIndex === -1) {
        repositories.push(repository);
      } else {
        repositories[existingIndex] = repository;
      }
      draft.repositories.itemsByWorkspaceId[workspaceId] = repositories;
      if (loaded !== undefined) {
        draft.repositories.loadedByWorkspaceId[workspaceId] = loaded;
      }
    }),
  setRepositoriesLoading: (workspaceId, loading) =>
    set((draft) => {
      draft.repositories.loadingByWorkspaceId[workspaceId] = loading;
    }),
  setRepositoryBranches: (repositoryId, branches, meta) =>
    set((draft) => {
      draft.repositoryBranches.itemsByRepositoryId[repositoryId] = branches;
      draft.repositoryBranches.loadingByRepositoryId[repositoryId] = false;
      draft.repositoryBranches.loadedByRepositoryId[repositoryId] = true;
      if (meta?.fetchedAt !== undefined) {
        draft.repositoryBranches.fetchedAtByRepositoryId[repositoryId] = meta.fetchedAt;
      }
      // fetchError is replaced on every successful response (empty string clears it).
      draft.repositoryBranches.fetchErrorByRepositoryId[repositoryId] =
        meta?.fetchError ?? undefined;
    }),
  setRepositoryBranchesLoading: (repositoryId, loading) =>
    set((draft) => {
      draft.repositoryBranches.loadingByRepositoryId[repositoryId] = loading;
    }),
  setRepositoryBranchesFetchError: (repositoryId, error) =>
    set((draft) => {
      draft.repositoryBranches.fetchErrorByRepositoryId[repositoryId] = error;
    }),
  setRepositoryScripts: (repositoryId, scripts) =>
    set((draft) => {
      draft.repositoryScripts.itemsByRepositoryId[repositoryId] = scripts;
      draft.repositoryScripts.loadingByRepositoryId[repositoryId] = false;
      draft.repositoryScripts.loadedByRepositoryId[repositoryId] = true;
    }),
  setRepositoryScriptsLoading: (repositoryId, loading) =>
    set((draft) => {
      draft.repositoryScripts.loadingByRepositoryId[repositoryId] = loading;
    }),
  clearRepositoryScripts: (repositoryId) =>
    set((draft) => {
      delete draft.repositoryScripts.itemsByRepositoryId[repositoryId];
      delete draft.repositoryScripts.loadingByRepositoryId[repositoryId];
      delete draft.repositoryScripts.loadedByRepositoryId[repositoryId];
    }),
  invalidateRepositories: (workspaceId) =>
    set((draft) => {
      draft.repositories.loadedByWorkspaceId[workspaceId] = false;
    }),
  ...createRepositorySetActions(set),
  ...createRepositoryBranchPolicyActions(set),
});

/**
 * The repository-set actions, split out so the slice factory stays under the
 * function-length cap.
 */
function createRepositorySetActions(
  set: (recipe: (draft: WorkspaceSlice) => void) => void,
): Pick<
  WorkspaceSlice,
  | "setRepositorySets"
  | "setRepositorySetsLoading"
  | "upsertRepositorySet"
  | "removeRepositorySet"
  | "invalidateRepositorySets"
> {
  return {
    setRepositorySets: (workspaceId, sets, expectedRevision) =>
      set((draft) => {
        const current = draft.repositorySets.revisionByWorkspaceId[workspaceId] ?? 0;
        // A response captured before a WebSocket event must not be applied after
        // it: doing so removes a newly created set, resurrects a deleted one, or
        // restores stale membership.
        if (expectedRevision !== undefined && expectedRevision !== current) return;
        draft.repositorySets.itemsByWorkspaceId[workspaceId] = sortRepositorySets(sets);
        draft.repositorySets.loadingByWorkspaceId[workspaceId] = false;
        draft.repositorySets.loadedByWorkspaceId[workspaceId] = true;
      }),
    setRepositorySetsLoading: (workspaceId, loading) =>
      set((draft) => {
        draft.repositorySets.loadingByWorkspaceId[workspaceId] = loading;
      }),
    upsertRepositorySet: (workspaceId, repositorySet) =>
      set((draft) => {
        const sets = draft.repositorySets.itemsByWorkspaceId[workspaceId] ?? [];
        const existingIndex = sets.findIndex((candidate) => candidate.id === repositorySet.id);
        if (existingIndex === -1) {
          sets.push(repositorySet);
        } else {
          sets[existingIndex] = repositorySet;
        }
        // Re-sort rather than append: the list is rendered in this order, and a
        // set arriving by WebSocket event would otherwise sit at the bottom while
        // the same list refetched shows it in place.
        draft.repositorySets.itemsByWorkspaceId[workspaceId] = sortRepositorySets(sets);
        bumpRepositorySetRevision(draft, workspaceId);
        // Deliberately does not touch loadedByWorkspaceId: an event for a
        // workspace never listed must not suppress its initial fetch.
      }),
    removeRepositorySet: (workspaceId, setId) =>
      set((draft) => {
        const sets = draft.repositorySets.itemsByWorkspaceId[workspaceId];
        if (!sets) return;
        draft.repositorySets.itemsByWorkspaceId[workspaceId] = sets.filter(
          (candidate) => candidate.id !== setId,
        );
        bumpRepositorySetRevision(draft, workspaceId);
      }),
    invalidateRepositorySets: (workspaceId) =>
      set((draft) => {
        draft.repositorySets.loadedByWorkspaceId[workspaceId] = false;
      }),
  };
}

/**
 * Marks a workspace's repository sets as changed by an event. A list response
 * captured before this bump is dropped rather than applied over it.
 */
function bumpRepositorySetRevision(draft: WorkspaceSlice, workspaceId: string): void {
  draft.repositorySets.revisionByWorkspaceId[workspaceId] =
    (draft.repositorySets.revisionByWorkspaceId[workspaceId] ?? 0) + 1;
}

/** Sets are listed by name, case-insensitively, matching the backend list order. */
function sortRepositorySets(sets: RepositorySet[]): RepositorySet[] {
  return [...sets].sort((left, right) =>
    left.name.localeCompare(right.name, undefined, { sensitivity: "base" }),
  );
}

function createRepositoryBranchPolicyActions(
  set: (recipe: (draft: WorkspaceSlice) => void) => void,
): Pick<
  WorkspaceSlice,
  | "setRepositoryBranchPolicies"
  | "setRepositoryBranchPoliciesLoading"
  | "upsertRepositoryBranchPolicy"
  | "removeRepositoryBranchPolicy"
> {
  return {
    setRepositoryBranchPolicies: (repositoryId, policies, expectedRevision) =>
      set((draft) => {
        const current = draft.repositoryBranchPolicies.revisionByRepositoryId[repositoryId] ?? 0;
        if (expectedRevision !== undefined && expectedRevision !== current) return;
        draft.repositoryBranchPolicies.itemsByRepositoryId[repositoryId] =
          sortBranchPolicies(policies);
        draft.repositoryBranchPolicies.loadingByRepositoryId[repositoryId] = false;
        draft.repositoryBranchPolicies.loadedByRepositoryId[repositoryId] = true;
      }),
    setRepositoryBranchPoliciesLoading: (repositoryId, loading) =>
      set((draft) => {
        draft.repositoryBranchPolicies.loadingByRepositoryId[repositoryId] = loading;
      }),
    upsertRepositoryBranchPolicy: (policy) =>
      set((draft) => {
        const policies =
          draft.repositoryBranchPolicies.itemsByRepositoryId[policy.repository_id] ?? [];
        const index = policies.findIndex((candidate) => candidate.id === policy.id);
        if (index === -1) policies.push(policy);
        else policies[index] = policy;
        draft.repositoryBranchPolicies.itemsByRepositoryId[policy.repository_id] =
          sortBranchPolicies(policies);
        draft.repositoryBranchPolicies.revisionByRepositoryId[policy.repository_id] =
          (draft.repositoryBranchPolicies.revisionByRepositoryId[policy.repository_id] ?? 0) + 1;
      }),
    removeRepositoryBranchPolicy: (repositoryId, policyId) =>
      set((draft) => {
        const policies = draft.repositoryBranchPolicies.itemsByRepositoryId[repositoryId];
        if (policies) {
          draft.repositoryBranchPolicies.itemsByRepositoryId[repositoryId] = policies.filter(
            (policy) => policy.id !== policyId,
          );
        }
        draft.repositoryBranchPolicies.revisionByRepositoryId[repositoryId] =
          (draft.repositoryBranchPolicies.revisionByRepositoryId[repositoryId] ?? 0) + 1;
      }),
  };
}

function sortBranchPolicies(policies: RepositoryBranchPolicy[]): RepositoryBranchPolicy[] {
  return [...policies].sort((left, right) => {
    const byName = left.name.localeCompare(right.name, undefined, { sensitivity: "base" });
    return byName || left.id.localeCompare(right.id);
  });
}
