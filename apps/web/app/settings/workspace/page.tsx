import { fetchWorkspaces } from '@/lib/ssr/http';
import { WorkspacesPageClient } from '@/app/settings/workspace/workspaces-page-client';

export default async function WorkspacesPage() {
  const workspaces = await fetchWorkspaces();
  return <WorkspacesPageClient workspaces={workspaces.workspaces} />;
}
