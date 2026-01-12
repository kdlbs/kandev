import { getWorkspaceAction } from '@/app/actions/workspaces';
import { WorkspaceEditClient } from '@/app/settings/workspace/workspace-edit-client';

export default async function WorkspaceEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let workspace = null;

  try {
    workspace = await getWorkspaceAction(id);
  } catch {
    workspace = null;
  }

  return <WorkspaceEditClient workspace={workspace} />;
}
