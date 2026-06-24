import {
  listWorkspacesAction,
  listWorkflowsAction,
  listRepositoriesAction,
  listWorkspaceWorkflowStepsAction,
} from "@/app/actions/workspaces";
import { fetchUserSettings } from "@/lib/api";
import { StateHydrator } from "@/components/state-hydrator";
import {
  ACTIVE_WORKSPACE_COOKIE,
  LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
} from "@/lib/routing/route-bootstrap";
import { readCookies, type CookieStore } from "@/lib/server/cookies";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { resolveActiveId } from "@/lib/ssr/resolve-active-id";
import { GitHubPageClient } from "./github-page-client";
import type {
  Workflow,
  WorkflowStep,
  Repository,
  Workspace,
  UserSettingsResponse,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

function resolveGithubWorkspaceId(
  workspaces: Workspace[],
  settingsResponse: UserSettingsResponse | null,
  cookieStore: CookieStore | null,
): string | undefined {
  return (
    resolveActiveId(
      workspaces,
      cookieStore?.get(ACTIVE_WORKSPACE_COOKIE)?.value,
      cookieStore?.get(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE)?.value,
      settingsResponse?.settings?.workspace_id,
    ) ?? undefined
  );
}

export default async function GitHubPage() {
  let workspaces: Workspace[] = [];
  let workflows: Workflow[] = [];
  let steps: WorkflowStep[] = [];
  let repositories: Repository[] = [];
  let workspaceId: string | undefined;
  let workspaceDataLoaded = false;
  let userSettingsResponse: UserSettingsResponse | null = null;

  try {
    const [workspacesResponse, settingsResponse] = await Promise.all([
      listWorkspacesAction(),
      fetchUserSettings({ cache: "no-store" }).catch(() => null),
    ]);
    const cookieStore = await readCookies().catch(() => null);
    workspaces = workspacesResponse.workspaces;
    userSettingsResponse = settingsResponse;
    workspaceId = resolveGithubWorkspaceId(workspaces, settingsResponse, cookieStore);

    if (workspaceId) {
      const [workflowsRes, reposRes, stepsRes] = await Promise.all([
        listWorkflowsAction(workspaceId),
        listRepositoriesAction(workspaceId),
        listWorkspaceWorkflowStepsAction(workspaceId),
      ]);
      workflows = workflowsRes.workflows;
      repositories = reposRes.repositories;
      steps = stepsRes.steps;
      workspaceDataLoaded = true;
    }
  } catch (error) {
    console.error("Failed to load GitHub page data:", error);
  }

  const mappedUserSettings = mapUserSettingsResponse(userSettingsResponse);

  const initialState: Partial<AppState> = {
    workspaces: { items: workspaces, activeId: workspaceId ?? null },
    workflows: {
      items: workflows.map((w) => ({
        id: w.id,
        workspaceId: w.workspace_id,
        name: w.name,
        description: w.description ?? null,
        sortOrder: w.sort_order ?? 0,
        ...(w.agent_profile_id ? { agent_profile_id: w.agent_profile_id } : {}),
      })),
      activeId: workflows[0]?.id ?? null,
    },
    userSettings: { ...mappedUserSettings, workspaceId: workspaceId ?? null },
    ...(workspaceId && workspaceDataLoaded
      ? {
          repositories: {
            itemsByWorkspaceId: { [workspaceId]: repositories },
            loadingByWorkspaceId: { [workspaceId]: false },
            loadedByWorkspaceId: { [workspaceId]: true },
          },
        }
      : {}),
  };

  return (
    <>
      <StateHydrator initialState={initialState} />
      <GitHubPageClient
        workspaceId={workspaceId}
        workflows={workflows}
        steps={steps}
        repositories={repositories}
      />
    </>
  );
}
