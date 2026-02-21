import {
  listWorkspacesAction,
  listWorkflowsAction,
  listTasksByWorkspaceAction,
  listRepositoriesAction,
  listWorkspaceWorkflowStepsAction,
} from "@/app/actions/workspaces";
import { TasksPageClient } from "./tasks-page-client";
import type { Workflow, Task, WorkflowStep, Repository, Workspace } from "@/lib/types/http";

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

  try {
    const workspacesResponse = await listWorkspacesAction();
    workspaces = workspacesResponse.workspaces;

    // Use first workspace if none specified
    if (!workspaceId && workspaces.length > 0) {
      workspaceId = workspaces[0].id;
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
      tasks = tasksResponse.tasks;
      total = tasksResponse.total;
      steps = stepsResponse.steps;
    }
  } catch (error) {
    console.error("Failed to load tasks page data:", error);
  }

  return (
    <TasksPageClient
      workspaces={workspaces}
      initialWorkspaceId={workspaceId}
      initialWorkflows={workflows}
      initialSteps={steps}
      initialRepositories={repositories}
      initialTasks={tasks}
      initialTotal={total}
    />
  );
}
