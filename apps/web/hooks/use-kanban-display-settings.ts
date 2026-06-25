"use client";

import { useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useUserDisplaySettings } from "@/hooks/use-user-display-settings";
import { useWorkflows } from "@/hooks/use-workflows";
import { workspacesQueryOptions } from "@/lib/query/query-options";

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

function useWorkspaceSelectionData() {
  const storeWorkspaces = useAppStore((state) => state.workspaces.items);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const activeWorkflowId = useAppStore((state) => state.workflows.activeId);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);
  const workspacesQuery = useQuery(workspacesQueryOptions());
  const workspaces = workspacesQuery.data ?? storeWorkspaces;

  useEffect(() => {
    if (!workspacesQuery.data) return;
    setWorkspaces(workspacesQuery.data);
  }, [setWorkspaces, workspacesQuery.data]);

  return { workspaces, activeWorkspaceId, activeWorkflowId };
}

/**
 * Custom hook that consolidates all kanban display settings and eliminates prop drilling.
 * This hook provides access to workspaces, workflows, repositories, and preview settings,
 * along with handlers for changing these settings.
 */
export function useKanbanDisplaySettings() {
  const { workspaces, activeWorkspaceId, activeWorkflowId } = useWorkspaceSelectionData();
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  const setActiveWorkflow = useAppStore((state) => state.setActiveWorkflow);
  const { workflows } = useWorkflows(activeWorkspaceId, Boolean(activeWorkspaceId));

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

  const enablePreviewOnClick = useAppStore((state) => state.userSettings.enablePreviewOnClick);

  const handleWorkspaceChange = useCallback(
    (nextWorkspaceId: string | null) => {
      setActiveWorkspace(nextWorkspaceId);
      const url = nextWorkspaceId ? `/?workspaceId=${nextWorkspaceId}` : "/";
      window.history.pushState({}, "", url);
      commitSettings({
        workspaceId: nextWorkspaceId,
        workflowId: null,
        repositoryIds: [],
      });
    },
    [setActiveWorkspace, commitSettings],
  );

  const handleWorkflowChange = useCallback(
    (nextWorkflowId: string | null) => {
      setActiveWorkflow(nextWorkflowId);
      if (nextWorkflowId) {
        const workspaceId = workflows.find(
          (workflow) => workflow.id === nextWorkflowId,
        )?.workspaceId;
        const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : "";
        window.history.pushState({}, "", `/?workflowId=${nextWorkflowId}${workspaceParam}`);
      } else if (activeWorkspaceId) {
        window.history.pushState({}, "", `/?workspaceId=${activeWorkspaceId}`);
      }
      commitSettings({
        workspaceId: userSettings.workspaceId,
        workflowId: nextWorkflowId,
        repositoryIds: userSettings.repositoryIds,
      });
    },
    [
      setActiveWorkflow,
      workflows,
      commitSettings,
      userSettings.workspaceId,
      userSettings.repositoryIds,
      activeWorkspaceId,
    ],
  );

  const handleRepositoryChange = useCallback(
    (value: string | "all") => {
      const base = baseSettingsPayload(userSettings);
      commitSettings({ ...base, repositoryIds: value === "all" ? [] : [value] });
    },
    [commitSettings, userSettings],
  );

  const handleTogglePreviewOnClick = useCallback(
    (enabled: boolean) => {
      commitSettings({ ...baseSettingsPayload(userSettings), enablePreviewOnClick: enabled });
    },
    [commitSettings, userSettings],
  );

  const handleViewModeChange = useCallback(
    (mode: string) => {
      commitSettings({ ...baseSettingsPayload(userSettings), kanbanViewMode: mode || null });
    },
    [commitSettings, userSettings],
  );

  return {
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

    onWorkspaceChange: handleWorkspaceChange,
    onWorkflowChange: handleWorkflowChange,
    onRepositoryChange: handleRepositoryChange,
    onTogglePreviewOnClick: handleTogglePreviewOnClick,
    onViewModeChange: handleViewModeChange,
  };
}
