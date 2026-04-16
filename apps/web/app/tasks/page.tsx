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
import { TasksPageClient } from "./tasks-page-client";
import type { Workflow, Task, WorkflowStep, Repository, Workspace, UserSettingsResponse } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

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
  let activeRepositoryId: string | null = null;

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
      // Fetch all data in parallel (including steps via single batch endpoint)
      const [workflowsResponse, repositoriesResponse, tasksResponse, stepsResponse] =
        await Promise.all([
          listWorkflowsAction(workspaceId),
          listRepositoriesAction(workspaceId),
          listTasksByWorkspaceAction(workspaceId, 1, 25),
          listWorkspaceWorkflowStepsAction(workspaceId),
        ]);

      workflows = workflowsResponse.workflows;
      repositories = repositoriesResponse.repositories;

      // Resolve active workflow: user settings > first workflow
      const savedWorkflowId = settingsResponse?.settings?.workflow_filter_id || null;
      const preferred = workflows.find((w) => w.id === savedWorkflowId);
      activeWorkflowId = preferred?.id ?? workflows[0]?.id ?? null;

      // Resolve active repository filter from user settings
      activeRepositoryId = settingsResponse?.settings?.repository_ids?.[0] ?? null;

      total = tasksResponse.total;

      // Pre-filter by workflow + repository to match the active display filters,
      // so the initial render is already correct without waiting for store hydration.
      tasks = tasksResponse.tasks;
      if (activeWorkflowId) tasks = tasks.filter((t) => t.workflow_id === activeWorkflowId);
      if (activeRepositoryId) tasks = tasks.filter((t) => t.repositories?.some((r) => r.repository_id === activeRepositoryId));
      steps = stepsResponse.steps;
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
