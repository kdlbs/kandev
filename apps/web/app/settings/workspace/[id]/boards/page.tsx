import {
  getWorkspaceAction,
  listBoardsAction,
  listWorkflowTemplatesAction,
} from '@/app/actions/workspaces';
import { WorkspaceBoardsClient } from '@/app/settings/workspace/workspace-boards-client';
import type { Board, WorkflowTemplate } from '@/lib/types/http';

export default async function WorkspaceBoardsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let workspace = null;
  let boards: Board[] = [];
  let workflowTemplates: WorkflowTemplate[] = [];

  try {
    workspace = await getWorkspaceAction(id);
    const [boardsResponse, templatesResponse] = await Promise.all([
      listBoardsAction(id),
      listWorkflowTemplatesAction(),
    ]);
    boards = boardsResponse.boards;
    workflowTemplates = templatesResponse.templates ?? [];
  } catch {
    workspace = null;
    boards = [];
    workflowTemplates = [];
  }

  return (
    <WorkspaceBoardsClient
      workspace={workspace}
      boards={boards}
      workflowTemplates={workflowTemplates}
    />
  );
}
