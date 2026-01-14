import { WorkspaceEditClient } from '@/app/settings/workspace/workspace-edit-client';

export default async function WorkspaceEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <WorkspaceEditClient workspaceId={id} />;
}
