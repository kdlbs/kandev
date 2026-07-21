import {
  listWorkspacesAction,
  listWorkflowsAction,
  listRepositoriesAction,
  listWorkspaceWorkflowStepsAction,
} from "@/app/actions/workspaces";
import { fetchUserSettings } from "@/lib/api";
import { StateHydrator } from "@/components/state-hydrator";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { GitLabPageClient } from "./gitlab-page-client";
import type {
  Repository,
  Workflow,
  WorkflowStep,
  Workspace,
  UserSettingsResponse,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

export default async function GitLabPage() {
  let workspaces: Workspace[] = [];
  let workflows: Workflow[] = [];
  let steps: WorkflowStep[] = [];
  let repositories: Repository[] = [];
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
    if (workspaceId) {
      const [workflowsResponse, repositoriesResponse, stepsResponse] = await Promise.all([
        listWorkflowsAction(workspaceId),
        listRepositoriesAction(workspaceId),
        listWorkspaceWorkflowStepsAction(workspaceId),
      ]);
      workflows = workflowsResponse.workflows;
      repositories = repositoriesResponse.repositories;
      steps = stepsResponse.steps;
    }
  } catch (error) {
    console.error("Failed to load GitLab page data:", error);
  }

  const mappedUserSettings = mapUserSettingsResponse(userSettingsResponse);

  const initialState: Partial<AppState> = {
    workspaces: { items: workspaces, activeId: workspaceId ?? null },
    workflows: {
      items: workflows.map((workflow) => ({
        id: workflow.id,
        workspaceId: workflow.workspace_id,
        name: workflow.name,
        description: workflow.description ?? null,
        sortOrder: workflow.sort_order ?? 0,
        ...(workflow.agent_profile_id ? { agent_profile_id: workflow.agent_profile_id } : {}),
      })),
      activeId: workflows[0]?.id ?? null,
    },
    userSettings: { ...mappedUserSettings, workspaceId: workspaceId ?? null },
  };

  return (
    <>
      <StateHydrator initialState={initialState} />
      <GitLabPageClient
        workspaceId={workspaceId}
        workflows={workflows}
        steps={steps}
        repositories={repositories}
      />
    </>
  );
}
