import {
  listWorkspacesAction,
  listWorkflowsAction,
  listWorkspaceWorkflowStepsAction,
} from "@/app/actions/workspaces";
import { fetchUserSettings } from "@/lib/api";
import { StateHydrator } from "@/components/state-hydrator";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { LinearPageClient } from "./linear-page-client";
import type { Workflow, WorkflowStep, Workspace, UserSettingsResponse } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

export default async function LinearPage() {
  let workspaces: Workspace[] = [];
  let workflows: Workflow[] = [];
  let steps: WorkflowStep[] = [];
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
      const [workflowsRes, stepsRes] = await Promise.all([
        listWorkflowsAction(workspaceId),
        listWorkspaceWorkflowStepsAction(workspaceId),
      ]);
      workflows = workflowsRes.workflows;
      steps = stepsRes.steps;
    }
  } catch (error) {
    console.error("Failed to load Linear page data:", error);
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
  };

  return (
    <>
      <StateHydrator initialState={initialState} />
      <LinearPageClient workspaceId={workspaceId} workflows={workflows} steps={steps} />
    </>
  );
}
