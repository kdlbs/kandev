import {
  getWorkspaceAction,
  listWorkflowsAction,
  listWorkflowTemplatesAction,
} from '@/app/actions/workspaces';
import { WorkspaceWorkflowsClient } from '@/app/settings/workspace/workspace-workflows-client';
import type { Workflow, WorkflowTemplate } from '@/lib/types/http';

export default async function WorkspaceWorkflowsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let workspace = null;
  let workflows: Workflow[] = [];
  let workflowTemplates: WorkflowTemplate[] = [];

  try {
    workspace = await getWorkspaceAction(id);
    const [workflowsResponse, templatesResponse] = await Promise.all([
      listWorkflowsAction(id),
      listWorkflowTemplatesAction(),
    ]);
    workflows = workflowsResponse.workflows;
    workflowTemplates = templatesResponse.templates ?? [];
  } catch {
    workspace = null;
    workflows = [];
    workflowTemplates = [];
  }

  return (
    <WorkspaceWorkflowsClient
      workspace={workspace}
      workflows={workflows}
      workflowTemplates={workflowTemplates}
    />
  );
}
