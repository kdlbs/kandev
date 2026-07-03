import {
  listWorkspacesAction,
  listWorkflowsAction,
  listTasksByWorkspaceAction,
  listRepositoriesAction,
} from "@/app/actions/workspaces";
import { fetchUserSettings } from "@/lib/api";
import { StateHydrator } from "@/components/state-hydrator";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { resolveDesiredWorkflowId } from "@/lib/kanban/resolve-workflow";
import { TasksPageClient } from "./tasks-page-client";
import {
  parseTasksListGroup,
  parseTasksListSort,
  sortTasksForList,
  type TasksListGroup,
  type TasksListSort,
} from "@/lib/tasks/tasks-list-options";
import type { Workflow, Task, Repository, Workspace, UserSettingsResponse } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

type WorkspaceData = {
  workflows: Workflow[];
  repositories: Repository[];
  tasks: Task[];
  total: number;
  activeWorkflowId: string | null;
};

type TasksListPreferences = {
  sort: TasksListSort;
  group: TasksListGroup;
};

function resolveTasksListPreferences(
  sortParam: string | undefined,
  groupParam: string | undefined,
  settingsResponse: UserSettingsResponse | null,
): TasksListPreferences {
  return {
    sort: parseTasksListSort(sortParam ?? settingsResponse?.settings?.tasks_list_sort),
    group: parseTasksListGroup(groupParam ?? settingsResponse?.settings?.tasks_list_group),
  };
}

function resolveTasksPageWorkspaceId(
  workspaceParam: string | undefined,
  settingsResponse: UserSettingsResponse | null,
  workspaces: Workspace[],
): string | undefined {
  if (workspaceParam) return workspaceParam;
  return settingsResponse?.settings?.workspace_id || workspaces[0]?.id;
}

async function fetchWorkspaceData(
  workspaceId: string,
  settingsResponse: UserSettingsResponse | null,
  tasksListSort: TasksListSort,
): Promise<WorkspaceData> {
  const savedWorkflowId = settingsResponse?.settings?.workflow_filter_id ?? null;
  const savedRepositoryId = settingsResponse?.settings?.repository_ids?.[0] ?? null;

  const [workflowsResponse, repositoriesResponse, tasksResponse] = await Promise.all([
    listWorkflowsAction(workspaceId),
    listRepositoriesAction(workspaceId),
    listTasksByWorkspaceAction(workspaceId, {
      page: 1,
      pageSize: 25,
      workflowId: savedWorkflowId,
      repositoryId: savedRepositoryId,
    }),
  ]);

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
    tasks: sortTasksForList(tasksResponse.tasks, tasksListSort),
    total: tasksResponse.total,
    activeWorkflowId,
  };
}

export default async function TasksPage({
  searchParams,
}: {
  searchParams: Promise<{ workspace?: string; sort?: string; group?: string }>;
}) {
  const { workspace: workspaceParam, sort: sortParam, group: groupParam } = await searchParams;

  let workspaces: Workspace[] = [];
  let workflows: Workflow[] = [];
  let repositories: Repository[] = [];
  let tasks: Task[] = [];
  let total = 0;
  let workspaceId = workspaceParam;
  let userSettingsResponse: UserSettingsResponse | null = null;
  let activeWorkflowId: string | null = null;
  let tasksListSort: TasksListSort = parseTasksListSort(null);
  let tasksListGroup: TasksListGroup = parseTasksListGroup(null);

  try {
    const [workspacesResponse, settingsResponse] = await Promise.all([
      listWorkspacesAction(),
      fetchUserSettings({ cache: "no-store" }).catch(() => null),
    ]);
    workspaces = workspacesResponse.workspaces;
    userSettingsResponse = settingsResponse;
    const preferences = resolveTasksListPreferences(sortParam, groupParam, settingsResponse);
    tasksListSort = preferences.sort;
    tasksListGroup = preferences.group;

    // Use workspace from user settings or URL param or first workspace
    workspaceId = resolveTasksPageWorkspaceId(workspaceId, settingsResponse, workspaces);

    if (workspaceId) {
      // Fetch all data in parallel; resolve active filters so the server applies them
      const data = await fetchWorkspaceData(workspaceId, settingsResponse, tasksListSort);
      workflows = data.workflows;
      repositories = data.repositories;
      tasks = data.tasks;
      total = data.total;
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
    userSettings: {
      ...mappedUserSettings,
      workspaceId: workspaceId ?? null,
      tasksListSort,
      tasksListGroup,
    },
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
        initialRepositories={repositories}
        initialTasks={tasks}
        initialTotal={total}
        initialSort={tasksListSort}
        initialGroup={tasksListGroup}
      />
    </>
  );
}
