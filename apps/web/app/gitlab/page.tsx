import { listWorkspacesAction } from "@/app/actions/workspaces";
import { fetchUserSettings } from "@/lib/api";
import { StateHydrator } from "@/components/state-hydrator";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { GitLabPageClient } from "./gitlab-page-client";
import type { Workspace, UserSettingsResponse } from "@/lib/types/http";
import type { SsrInitialState } from "@/lib/ssr/initial-state";

// Minimal SSR entrypoint for /gitlab. v1 surface: list the current user's
// open MRs + issues and let them click through to GitLab. The browse-and-
// trigger flow (review/issue watchers, presets) is parallel to /github and
// will land alongside the watchers backend in a follow-up.
export default async function GitLabPage() {
  let workspaces: Workspace[] = [];
  let workspaceId: string | undefined;
  let userSettingsResponse: UserSettingsResponse | null = null;

  try {
    const [workspacesResponse, settingsResponse] = await Promise.all([
      listWorkspacesAction(),
      fetchUserSettings({ cache: "no-store" }).catch(() => null),
    ]);
    workspaces = workspacesResponse.workspaces;
    userSettingsResponse = settingsResponse;
    workspaceId = settingsResponse?.settings?.workspace_id || workspaces[0]?.id;
  } catch (error) {
    console.error("Failed to load GitLab page data:", error);
  }

  const mappedUserSettings = mapUserSettingsResponse(userSettingsResponse);

  const initialState: SsrInitialState = {
    workspaces: { items: workspaces, activeId: workspaceId ?? null },
    userSettings: { ...mappedUserSettings, workspaceId: workspaceId ?? null },
  };

  return (
    <>
      <StateHydrator initialState={initialState} />
      <GitLabPageClient workspaceId={workspaceId} />
    </>
  );
}
