import type { StatsRange } from "@/lib/api/domains/stats-api";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { listWorkspaces } from "@/lib/api";
import { StatsPageClient } from "./stats-page-client";

type StatsPageProps = {
  searchParams?: Promise<{
    range?: StatsRange;
  }>;
};

export default async function StatsPage({ searchParams }: StatsPageProps) {
  let workspaceId: string | undefined;
  let error: string | null = null;
  const params = searchParams ? await searchParams : undefined;
  const range = params?.range;

  try {
    const [userSettingsResponse, workspacesResponse] = await Promise.all([
      fetchUserSettings({ cache: "no-store" }),
      listWorkspaces({ cache: "no-store" }),
    ]);

    const settingsWorkspaceId = userSettingsResponse?.settings?.workspace_id;
    const workspaces = workspacesResponse?.workspaces ?? [];

    workspaceId = workspaces.find((w) => w.id === settingsWorkspaceId)?.id ?? workspaces[0]?.id;
  } catch (e) {
    error = e instanceof Error ? e.message : "Failed to resolve workspace";
  }

  return <StatsPageClient workspaceId={workspaceId} activeRange={range} initialError={error} />;
}
