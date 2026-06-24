import { cookies } from "next/headers";
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
import { GitHubPageClient } from "./github-page-client";
import type {
  Workflow,
  WorkflowStep,
  Repository,
  Workspace,
  UserSettingsResponse,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

type PageProps = {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
};

function resolveParam(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

export default async function GitHubPage({ searchParams }: PageProps = {}) {
  let workspaces: Workspace[] = [];
  let workflows: Workflow[] = [];
  let steps: WorkflowStep[] = [];
  let repositories: Repository[] = [];
  let workspaceId: string | undefined;
  let workspaceDataLoaded = false;
  let userSettingsResponse: UserSettingsResponse | null = null;

  try {
    const resolvedParams = searchParams ? await searchParams : {};
    const workspaceIdParam = resolveParam(resolvedParams.workspaceId);
    const [workspacesResponse, settingsResponse, cookieStore] = await Promise.all([
      listWorkspacesAction(),
      fetchUserSettings({ cache: "no-store" }).catch(() => null),
      cookies().catch((error) => {
        console.error("Failed to read cookies on GitHub page:", error);
        return null;
      }),
    ]);
    workspaces = workspacesResponse.workspaces;
    userSettingsResponse = settingsResponse;
    const settingsWorkspaceId = settingsResponse?.settings?.workspace_id || null;
    const cookieWorkspaceId = cookieStore?.get("office-active-workspace")?.value ?? null;
    workspaceId =
      resolveActiveId(workspaces, workspaceIdParam, cookieWorkspaceId, settingsWorkspaceId) ??
      undefined;

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
