import {
  listWorkspacesAction,
  listWorkflowsAction,
  listRepositoriesAction,
  listWorkspaceWorkflowStepsAction,
} from "@/app/actions/workspaces";
import { fetchUserSettings } from "@/lib/api";
import { StateHydrator } from "@/components/state-hydrator";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { resolveActiveId } from "@/lib/ssr/resolve-active-id";
import { readCookies } from "@/lib/server/cookies";
import {
  ACTIVE_WORKSPACE_COOKIE,
  LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
} from "@/lib/routing/route-bootstrap";
import { GitHubPageClient } from "./github-page-client";
import type {
  Workflow,
  WorkflowStep,
  Repository,
  Workspace,
  UserSettingsResponse,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

type GitHubPageData = {
  workspaces: Workspace[];
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  workspaceId: string | undefined;
  workspaceDataLoaded: boolean;
  userSettingsResponse: UserSettingsResponse | null;
};

const EMPTY_GITHUB_PAGE_DATA: GitHubPageData = {
  workspaces: [],
  workflows: [],
  steps: [],
  repositories: [],
  workspaceId: undefined,
  workspaceDataLoaded: false,
  userSettingsResponse: null,
};

async function loadGitHubPageData(): Promise<GitHubPageData> {
  try {
    const [workspacesResponse, settingsResponse, cookieStore] = await Promise.all([
      listWorkspacesAction(),
      fetchUserSettings({ cache: "no-store" }).catch(() => null),
      readCookies().catch((error) => {
        console.error("Failed to read cookies on GitHub page:", error);
        return null;
      }),
    ]);
    const workspaces = workspacesResponse.workspaces;
    const cookieWorkspaceId =
      cookieStore?.get(ACTIVE_WORKSPACE_COOKIE)?.value ??
      cookieStore?.get(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE)?.value ??
      null;
    const workspaceId =
      resolveActiveId(workspaces, cookieWorkspaceId, settingsResponse?.settings?.workspace_id) ??
      undefined;

    if (!workspaceId) {
      return { ...EMPTY_GITHUB_PAGE_DATA, workspaces, userSettingsResponse: settingsResponse };
    }

    const [workflowsRes, reposRes, stepsRes] = await Promise.all([
      listWorkflowsAction(workspaceId),
      listRepositoriesAction(workspaceId),
      listWorkspaceWorkflowStepsAction(workspaceId),
    ]);
    return {
      workspaces,
      workflows: workflowsRes.workflows,
      steps: stepsRes.steps,
      repositories: reposRes.repositories,
      workspaceId,
      workspaceDataLoaded: true,
      userSettingsResponse: settingsResponse,
    };
  } catch (error) {
    console.error("Failed to load GitHub page data:", error);
    return EMPTY_GITHUB_PAGE_DATA;
  }
}

export default async function GitHubPage() {
  const {
    workspaces,
    workflows,
    steps,
    repositories,
    workspaceId,
    workspaceDataLoaded,
    userSettingsResponse,
  } = await loadGitHubPageData();

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
