import {
  listWorkspacesAction,
  listWorkflowsAction,
  listTasksByWorkspaceAction,
  listRepositoriesAction,
  listWorkspaceWorkflowStepsAction,
} from "@/app/actions/workspaces";
import { fetchUserSettings } from "@/lib/api";
import { StateHydrator } from "@/components/state-hydrator";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { resolveDesiredWorkflowId } from "@/lib/kanban/resolve-workflow";
import { TasksPageClient } from "./tasks-page-client";
import type {
  Workflow,
  Task,
  WorkflowStep,
  Repository,
  Workspace,
  UserSettingsResponse,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

type WorkspaceData = {
  workflows: Workflow[];
  repositories: Repository[];
  tasks: Task[];
  total: number;
  steps: WorkflowStep[];
  activeWorkflowId: string | null;
};

async function fetchWorkspaceData(
  workspaceId: string,
  settingsResponse: UserSettingsResponse | null,
): Promise<WorkspaceData> {
  const savedWorkflowId = settingsResponse?.settings?.workflow_filter_id ?? null;
  const savedRepositoryId = settingsResponse?.settings?.repository_ids?.[0] ?? null;

  const [workflowsResponse, repositoriesResponse, tasksResponse, stepsResponse] = await Promise.all(
    [
      listWorkflowsAction(workspaceId),
      listRepositoriesAction(workspaceId),
      listTasksByWorkspaceAction(workspaceId, {
        page: 1,
        pageSize: 25,
        workflowId: savedWorkflowId,
        repositoryId: savedRepositoryId,
      }),
      listWorkspaceWorkflowStepsAction(workspaceId),
    ],
  );

  const workflows = workflowsResponse.workflows;
  // Preserve "All Workflows" (null/empty saved filter) instead of falling back
  // to the first workflow — resolveDesiredWorkflowId handles this consistently
  // with the kanban page and the root tasks page.
  const activeWorkflowId = resolveDesiredWorkflowId({
    settingsWorkflowId: savedWorkflowId,
    workspaceWorkflows: workflows,
  });

  return {
    workflows,
    repositories: repositoriesResponse.repositories,
    tasks: tasksResponse.tasks,
    total: tasksResponse.total,
    steps: stepsResponse.steps,
    activeWorkflowId,
  };
}

export default async function TasksPage({
  searchParams,
}: {
  searchParams: Promise<{ workspace?: string }>;
}) {
  const { workspace: workspaceParam } = await searchParams;

  let workspaces: Workspace[] = [];
  let workflows: Workflow[] = [];
  let steps: WorkflowStep[] = [];
  let repositories: Repository[] = [];
  let tasks: Task[] = [];
  let total = 0;
  let workspaceId = workspaceParam;
  let userSettingsResponse: UserSettingsResponse | null = null;
  let activeWorkflowId: string | null = null;

  try {
    const [workspacesResponse, settingsResponse] = await Promise.all([
      listWorkspacesAction(),
      fetchUserSettings({ cache: "no-store" }).catch(() => null),
    ]);
    workspaces = workspacesResponse.workspaces;
    userSettingsResponse = settingsResponse;

    // Use workspace from user settings or URL param or first workspace
    if (!workspaceId) {
      workspaceId = settingsResponse?.settings?.workspace_id || workspaces[0]?.id;
    }

    if (workspaceId) {
      // Fetch all data in parallel; resolve active filters so the server applies them
      const data = await fetchWorkspaceData(workspaceId, settingsResponse);
      workflows = data.workflows;
      repositories = data.repositories;
      tasks = data.tasks;
      total = data.total;
      steps = data.steps;
      activeWorkflowId = data.activeWorkflowId;
    }
  } catch (error) {
    console.error("Failed to load tasks page data:", error);
  }

  const mappedUserSettings = mapUserSettingsResponse(userSettingsResponse);

  const initialState: Partial<AppState> = {
    workspaces: {
      items: workspaces,
      activeId: workspaceId ?? null,
    },
    workflows: {
      items: workflows.map((w) => ({
        id: w.id,
        workspaceId: w.workspace_id,
        name: w.name,
        description: w.description ?? null,
        sortOrder: w.sort_order ?? 0,
        ...(w.agent_profile_id ? { agent_profile_id: w.agent_profile_id } : {}),
      })),
      activeId: activeWorkflowId,
    },
    userSettings: { ...mappedUserSettings, workspaceId: workspaceId ?? null },
    ...(workspaceId
      ? {
          repositories: {
            itemsByWorkspaceId: { [workspaceId]: repositories },
            loadingByWorkspaceId: { [workspaceId]: false },
            loadedByWorkspaceId: { [workspaceId]: true },
          },
        }
      : {}),
  };

  return (
    <>
      <StateHydrator initialState={initialState} />
      <TasksPageClient
        workspaces={workspaces}
        initialWorkspaceId={workspaceId}
        initialWorkflows={workflows}
        initialSteps={steps}
        initialRepositories={repositories}
        initialTasks={tasks}
        initialTotal={total}
      />
    </>
  );
}
