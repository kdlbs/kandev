import { fetchStats } from '@/lib/api/domains/stats-api';
import { fetchUserSettings } from '@/lib/api/domains/settings-api';
import { listWorkspaces } from '@/lib/api';
import { StatsPageClient } from './stats-page-client';

export default async function StatsPage() {
  let stats = null;
  let error = null;
  let workspaceId: string | undefined;

  try {
    // Get user settings to find active workspace
    const [userSettingsResponse, workspacesResponse] = await Promise.all([
      fetchUserSettings({ cache: 'no-store' }),
      listWorkspaces({ cache: 'no-store' }),
    ]);

    const settingsWorkspaceId = userSettingsResponse?.settings?.workspace_id;
    const workspaces = workspacesResponse?.workspaces ?? [];

    // Find active workspace: user setting > first workspace
    workspaceId =
      workspaces.find((w) => w.id === settingsWorkspaceId)?.id ??
      workspaces[0]?.id;

    if (workspaceId) {
      stats = await fetchStats(workspaceId, { cache: 'no-store' });
    }
  } catch (e) {
    error = e instanceof Error ? e.message : 'Failed to fetch stats';
  }

  return <StatsPageClient stats={stats} error={error} workspaceId={workspaceId} />;
}

