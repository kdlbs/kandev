'use client';

import { useCallback } from 'react';
import { useAppStore } from '@/components/state-provider';
import { useUserDisplaySettings } from '@/hooks/use-user-display-settings';
import type { WorkflowsState } from '@/lib/state/slices';

type UserSettingsFields = {
  workspaceId: string | null;
  workflowId: string | null;
  repositoryIds: string[];
};

/** Build the base settings payload from current user settings. */
function baseSettingsPayload(settings: UserSettingsFields): UserSettingsFields {
  return {
    workspaceId: settings.workspaceId,
    workflowId: settings.workflowId,
    repositoryIds: settings.repositoryIds,
  };
}

/**
 * Custom hook that consolidates all kanban display settings and eliminates prop drilling.
 * This hook provides access to workspaces, workflows, repositories, and preview settings,
 * along with handlers for changing these settings.
 */
export function useKanbanDisplaySettings() {
  // Access store directly
  const workspaces = useAppStore((state) => state.workspaces.items);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workflows = useAppStore((state) => state.workflows.items);
  const activeWorkflowId = useAppStore((state) => state.workflows.activeId);
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  const setActiveWorkflow = useAppStore((state) => state.setActiveWorkflow);

  // Use existing compound hook for user settings
  const {
    settings: userSettings,
    commitSettings,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
  } = useUserDisplaySettings({
    workspaceId: activeWorkspaceId,
    workflowId: activeWorkflowId,
  });

  // Get preview setting from store
  const enablePreviewOnClick = useAppStore((state) => state.userSettings.enablePreviewOnClick);

  // Use pushState instead of router.push to avoid triggering SSR re-fetches.
  // Filter changes only update client state; all data is already available.
  const handleWorkspaceChange = useCallback(
    (nextWorkspaceId: string | null) => {
      setActiveWorkspace(nextWorkspaceId);
      const url = nextWorkspaceId ? `/?workspaceId=${nextWorkspaceId}` : '/';
      window.history.pushState({}, '', url);
      commitSettings({
        workspaceId: nextWorkspaceId,
        workflowId: null,
        repositoryIds: [],
      });
    },
    [setActiveWorkspace, commitSettings]
  );

  const handleWorkflowChange = useCallback(
    (nextWorkflowId: string | null) => {
      setActiveWorkflow(nextWorkflowId);
      if (nextWorkflowId) {
        const workspaceId = workflows.find((workflow: WorkflowsState['items'][number]) => workflow.id === nextWorkflowId)?.workspaceId;
        const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : '';
        window.history.pushState({}, '', `/?workflowId=${nextWorkflowId}${workspaceParam}`);
      } else if (activeWorkspaceId) {
        window.history.pushState({}, '', `/?workspaceId=${activeWorkspaceId}`);
      }
      commitSettings({
        workspaceId: userSettings.workspaceId,
        workflowId: nextWorkflowId,
        repositoryIds: userSettings.repositoryIds,
      });
    },
    [setActiveWorkflow, workflows, commitSettings, userSettings.workspaceId, userSettings.repositoryIds, activeWorkspaceId]
  );

  const handleRepositoryChange = useCallback(
    (value: string | 'all') => {
      const base = baseSettingsPayload(userSettings);
      commitSettings({ ...base, repositoryIds: value === 'all' ? [] : [value] });
    },
    [commitSettings, userSettings]
  );

  const handleTogglePreviewOnClick = useCallback(
    (enabled: boolean) => {
      commitSettings({ ...baseSettingsPayload(userSettings), enablePreviewOnClick: enabled });
    },
    [commitSettings, userSettings]
  );

  const handleViewModeChange = useCallback(
    (mode: string) => {
      commitSettings({ ...baseSettingsPayload(userSettings), kanbanViewMode: mode || null });
    },
    [commitSettings, userSettings]
  );

  return {
    // Data
    workspaces,
    workflows,
    activeWorkspaceId,
    activeWorkflowId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId: userSettings.repositoryIds[0] ?? null,
    enablePreviewOnClick,
    kanbanViewMode: userSettings.kanbanViewMode,

    // Handlers
    onWorkspaceChange: handleWorkspaceChange,
    onWorkflowChange: handleWorkflowChange,
    onRepositoryChange: handleRepositoryChange,
    onTogglePreviewOnClick: handleTogglePreviewOnClick,
    onViewModeChange: handleViewModeChange,
  };
}
