import {
  listWorkspacesAction,
  listBoardsAction,
  listTasksByWorkspaceAction,
  listRepositoriesAction,
  listWorkflowStepsAction,
} from '@/app/actions/workspaces';
import { TasksPageClient } from './tasks-page-client';
import type { Board, Task, WorkflowStep, Repository, Workspace } from '@/lib/types/http';

export default async function TasksPage({
  searchParams,
}: {
  searchParams: Promise<{ workspace?: string }>;
}) {
  const { workspace: workspaceParam } = await searchParams;

  let workspaces: Workspace[] = [];
  let boards: Board[] = [];
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
      // Fetch all data in parallel
      const [boardsResponse, repositoriesResponse, tasksResponse] = await Promise.all([
        listBoardsAction(workspaceId),
        listRepositoriesAction(workspaceId),
        listTasksByWorkspaceAction(workspaceId, 1, 25),
      ]);

      boards = boardsResponse.boards;
      repositories = repositoriesResponse.repositories;
      tasks = tasksResponse.tasks;
      total = tasksResponse.total;

      // Fetch workflow steps for each board
      const stepsResponses = await Promise.all(
        boards.map((board) => listWorkflowStepsAction(board.id))
      );
      steps = stepsResponses.flatMap((r) => r.steps);
    }
  } catch (error) {
    console.error('Failed to load tasks page data:', error);
  }

  return (
    <TasksPageClient
      workspaces={workspaces}
      initialWorkspaceId={workspaceId}
      initialBoards={boards}
      initialSteps={steps}
      initialRepositories={repositories}
      initialTasks={tasks}
      initialTotal={total}
    />
  );
}
