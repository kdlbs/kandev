"use client";

import { useCallback, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useUserDisplaySettings } from "@/hooks/use-user-display-settings";
import { useTaskListingView } from "@/hooks/use-task-listing-view";
import type { TaskListingView } from "@/lib/task-listing/view-preference";
import { listingHistoryHref } from "@/lib/task-listing/view-navigation";
import type { WorkflowsState } from "@/lib/state/slices";
import { selectWorkflowSwimlanes } from "@/lib/kanban/workflow-swimlanes";

type UserSettingsFields = {
  workspaceId: string | null;
  workflowId: string | null;
  repositoryIds: string[];
  hiddenWorkflowStepIds?: Record<string, string[]>;
  workflowIdsWithAutoHideEmptySteps?: string[];
};

type CommitSettingsFn = (
  payload: UserSettingsFields & {
    enablePreviewOnClick?: boolean;
    tasksListShowDetails?: boolean;
  },
) => void;

/** Build the base settings payload from current user settings. */
function baseSettingsPayload(settings: UserSettingsFields): UserSettingsFields {
  return {
    workspaceId: settings.workspaceId,
    workflowId: settings.workflowId,
    repositoryIds: settings.repositoryIds,
    hiddenWorkflowStepIds: settings.hiddenWorkflowStepIds,
    workflowIdsWithAutoHideEmptySteps: settings.workflowIdsWithAutoHideEmptySteps,
  };
}

function taskListingViewFor(mode: string): TaskListingView {
  if (mode === "graph2" || mode === "pipeline") return "pipeline";
  if (mode === "list") return "list";
  if (mode === "threads") return "threads";
  return "kanban";
}

function useViewModeChange() {
  const { effectiveView, setView } = useTaskListingView();
  const onViewModeChange = useCallback(
    (mode: string) => setView(taskListingViewFor(mode)),
    [setView],
  );
  return { effectiveView, onViewModeChange };
}

/**
 * Reflects a scope change in the URL without routing. Route-aware because the
 * board, the Tasks list and the Threads deck all share these handlers: pushing
 * a task-overview URL from a routed view leaves that view rendered under Home.
 */
function replaceTaskOverviewHistory(workspaceId?: string, workflowId?: string) {
  const href = listingHistoryHref(window.location.pathname, { workspaceId, workflowId });
  window.history.pushState({}, "", href);
}

type WorkspaceWorkflowHandlersInput = {
  activeWorkspaceId: string | null;
  workflows: WorkflowsState["items"];
  userSettings: UserSettingsFields;
  commitSettings: CommitSettingsFn;
  setActiveWorkspace: (id: string | null) => void;
  setActiveWorkflow: (id: string | null) => void;
};

function useWorkspaceWorkflowHandlers({
  activeWorkspaceId,
  workflows,
  userSettings,
  commitSettings,
  setActiveWorkspace,
  setActiveWorkflow,
}: WorkspaceWorkflowHandlersInput) {
  const onWorkspaceChange = useCallback(
    (nextWorkspaceId: string | null) => {
      setActiveWorkspace(nextWorkspaceId);
      replaceTaskOverviewHistory(nextWorkspaceId ?? undefined);
      commitSettings({ workspaceId: nextWorkspaceId, workflowId: null, repositoryIds: [] });
    },
    [setActiveWorkspace, commitSettings],
  );
  const onWorkflowChange = useCallback(
    (nextWorkflowId: string | null) => {
      setActiveWorkflow(nextWorkflowId);
      if (nextWorkflowId) {
        const wsId = workflows.find(
          (wf: WorkflowsState["items"][number]) => wf.id === nextWorkflowId,
        )?.workspaceId;
        replaceTaskOverviewHistory(wsId, nextWorkflowId);
      } else if (activeWorkspaceId) {
        replaceTaskOverviewHistory(activeWorkspaceId);
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
  return { onWorkspaceChange, onWorkflowChange };
}

function useStepVisibilityHandlers(
  workflows: WorkflowsState["items"],
  snapshots: Record<string, unknown>,
  userSettings: UserSettingsFields,
  commitSettings: CommitSettingsFn,
  activeWorkflowId: string | null,
) {
  const eligibleWorkflows = useMemo(
    () => selectWorkflowSwimlanes(activeWorkflowId, workflows, snapshots),
    [activeWorkflowId, workflows, snapshots],
  );
  const onToggleStepVisibility = useCallback(
    (workflowId: string, stepId: string) => {
      const current = userSettings.hiddenWorkflowStepIds ?? {};
      const currentHidden = current[workflowId] ?? [];
      const isHidden = currentHidden.includes(stepId);
      const nextHidden = isHidden
        ? currentHidden.filter((id) => id !== stepId)
        : [...currentHidden, stepId];
      commitSettings({
        ...baseSettingsPayload(userSettings),
        hiddenWorkflowStepIds: { ...current, [workflowId]: nextHidden },
      });
    },
    [commitSettings, userSettings],
  );
  const onToggleAutoHideEmpty = useCallback(
    (workflowId: string) => {
      const current = userSettings.workflowIdsWithAutoHideEmptySteps ?? [];
      const next = current.includes(workflowId)
        ? current.filter((id) => id !== workflowId)
        : [...current, workflowId];
      commitSettings({
        ...baseSettingsPayload(userSettings),
        workflowIdsWithAutoHideEmptySteps: next,
      });
    },
    [commitSettings, userSettings],
  );
  return { eligibleWorkflows, onToggleStepVisibility, onToggleAutoHideEmpty };
}

/**
 * Custom hook that consolidates all kanban display settings and eliminates prop drilling.
 * This hook provides access to workspaces, workflows, repositories, and preview settings,
 * along with handlers for changing these settings.
 */
export function useKanbanDisplaySettings() {
  const workspaces = useAppStore((state) => state.workspaces.items);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workflows = useAppStore((state) => state.workflows.items);
  const activeWorkflowId = useAppStore((state) => state.workflows.activeId);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  const setActiveWorkflow = useAppStore((state) => state.setActiveWorkflow);
  const enablePreviewOnClick = useAppStore((state) => state.userSettings.enablePreviewOnClick);

  const {
    settings: userSettings,
    commitSettings,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
  } = useUserDisplaySettings({ workspaceId: activeWorkspaceId, workflowId: activeWorkflowId });

  const { onWorkspaceChange, onWorkflowChange } = useWorkspaceWorkflowHandlers({
    activeWorkspaceId,
    workflows,
    userSettings,
    commitSettings,
    setActiveWorkspace,
    setActiveWorkflow,
  });

  const onRepositoryChange = useCallback(
    (value: string | "all") => {
      commitSettings({
        ...baseSettingsPayload(userSettings),
        repositoryIds: value === "all" ? [] : [value],
      });
    },
    [commitSettings, userSettings],
  );
  const onTogglePreviewOnClick = useCallback(
    (enabled: boolean) => {
      commitSettings({ ...baseSettingsPayload(userSettings), enablePreviewOnClick: enabled });
    },
    [commitSettings, userSettings],
  );
  const onToggleTasksListShowDetails = useCallback(
    (enabled: boolean) => {
      commitSettings({ ...baseSettingsPayload(userSettings), tasksListShowDetails: enabled });
    },
    [commitSettings, userSettings],
  );

  const { eligibleWorkflows, onToggleStepVisibility, onToggleAutoHideEmpty } =
    useStepVisibilityHandlers(workflows, snapshots, userSettings, commitSettings, activeWorkflowId);

  const { effectiveView, onViewModeChange } = useViewModeChange();

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
    tasksListShowDetails: userSettings.tasksListShowDetails ?? false,
    effectiveTaskListingView: effectiveView,
    eligibleWorkflows,
    snapshots,
    hiddenWorkflowStepIds: userSettings.hiddenWorkflowStepIds ?? {},
    workflowIdsWithAutoHideEmptySteps: userSettings.workflowIdsWithAutoHideEmptySteps ?? [],
    onWorkspaceChange,
    onWorkflowChange,
    onRepositoryChange,
    onTogglePreviewOnClick,
    onToggleTasksListShowDetails,
    onToggleStepVisibility,
    onToggleAutoHideEmpty,
    onViewModeChange,
  };
}
