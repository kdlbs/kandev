import type { StateCreator } from 'zustand';
import type { WorkspaceSlice, WorkspaceSliceState } from './types';

export const defaultWorkspaceState: WorkspaceSliceState = {
  workspaces: { items: [], activeId: null },
  repositories: { itemsByWorkspaceId: {}, loadingByWorkspaceId: {}, loadedByWorkspaceId: {} },
  repositoryBranches: { itemsByRepositoryId: {}, loadingByRepositoryId: {}, loadedByRepositoryId: {} },
  repositoryScripts: { itemsByRepositoryId: {}, loadingByRepositoryId: {}, loadedByRepositoryId: {} },
};

export const createWorkspaceSlice: StateCreator<
  WorkspaceSlice,
  [['zustand/immer', never]],
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
  setRepositoriesLoading: (workspaceId, loading) =>
    set((draft) => {
      draft.repositories.loadingByWorkspaceId[workspaceId] = loading;
    }),
  setRepositoryBranches: (repositoryId, branches) =>
    set((draft) => {
      draft.repositoryBranches.itemsByRepositoryId[repositoryId] = branches;
      draft.repositoryBranches.loadingByRepositoryId[repositoryId] = false;
      draft.repositoryBranches.loadedByRepositoryId[repositoryId] = true;
    }),
  setRepositoryBranchesLoading: (repositoryId, loading) =>
    set((draft) => {
      draft.repositoryBranches.loadingByRepositoryId[repositoryId] = loading;
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
});
