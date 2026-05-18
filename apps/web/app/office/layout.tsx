import { notFound } from "next/navigation";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateHydrator } from "@/components/state-hydrator";
import { getFeatureFlagsAction } from "@/app/actions/features";
import { listWorkspaces, fetchUserSettings } from "@/lib/api";
import {
  getInbox,
  getMeta,
  getOnboardingState,
  listAgentProfiles,
  listProjects,
} from "@/lib/api/domains/office-api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { AppState } from "@/lib/state/store";
import { WorkspaceRail } from "./components/workspace-rail";
import { OfficeSidebar } from "./components/office-sidebar";
import { OfficeTopbar } from "./components/office-topbar";

function mapWorkspaceItem(ws: {
  id: string;
  name: string;
  description?: string | null;
  owner_id: string;
  default_executor_id?: string | null;
  default_environment_id?: string | null;
  default_agent_profile_id?: string | null;
  default_config_agent_profile_id?: string | null;
  created_at: string;
  updated_at: string;
}) {
  return {
    id: ws.id,
    name: ws.name,
    description: ws.description ?? null,
    owner_id: ws.owner_id,
    default_executor_id: ws.default_executor_id ?? null,
    default_environment_id: ws.default_environment_id ?? null,
    default_agent_profile_id: ws.default_agent_profile_id ?? null,
    default_config_agent_profile_id: ws.default_config_agent_profile_id ?? null,
    created_at: ws.created_at,
    updated_at: ws.updated_at,
  };
}

export default async function OfficeLayout({ children }: { children: React.ReactNode }) {
  // Feature gate: production releases ship with features.office=false and
  // the backend's /api/v1/office/* routes are not registered. Return 404
  // for every Office page so even a guessed URL looks like a non-existent
  // route, not "you don't have permission".
  // See docs/decisions/0007-runtime-feature-flags.md.
  const { office: officeEnabled } = await getFeatureFlagsAction();
  if (!officeEnabled) {
    notFound();
  }

  // Check onboarding before rendering the office chrome. When not complete,
  // render only the children (the setup wizard) without the workspace rail,
  // sidebar, or topbar — prevents a flash of stale workspace UI.
  const onboarding = await getOnboardingState({ cache: "no-store" }).catch(() => ({
    completed: false,
    fsWorkspaces: [],
  }));
  if (!onboarding.completed) {
    return <>{children}</>;
  }

  const [workspacesResponse, userSettingsResponse, metaResponse] = await Promise.all([
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
    getMeta({ cache: "no-store" }).catch(() => null),
  ]);

  // Only show office workspaces (those with an office_workflow_id) in the rail.
  // The kanban's "Default Workspace" has no office_workflow_id and should not appear.
  const officeWorkspaces = workspacesResponse.workspaces.filter((ws) => ws.office_workflow_id);
  const workspaceItems = officeWorkspaces.map(mapWorkspaceItem);
  const settingsWorkspaceId = userSettingsResponse?.settings?.workspace_id || null;
  const activeWorkspaceId =
    workspaceItems.find((w) => w.id === settingsWorkspaceId)?.id ?? workspaceItems[0]?.id ?? null;

  // Fetch agents + projects + inbox for the active workspace so the
  // sidebar renders them — including the inbox count badge — on first
  // paint without a client refetch.
  const [agentsResponse, projectsResponse, inboxResponse] = activeWorkspaceId
    ? await Promise.all([
        listAgentProfiles(activeWorkspaceId, { cache: "no-store" }).catch(() => ({
          agents: [],
        })),
        listProjects(activeWorkspaceId, { cache: "no-store" }).catch(() => ({
          projects: [],
        })),
        getInbox(activeWorkspaceId, { cache: "no-store" }).catch(() => ({
          items: [],
          total_count: 0,
        })),
      ])
    : [{ agents: [] }, { projects: [] }, { items: [], total_count: 0 }];

  const initialState: Partial<AppState> = {
    workspaces: {
      items: workspaceItems,
      activeId: activeWorkspaceId,
    },
    userSettings: {
      ...mapUserSettingsResponse(userSettingsResponse),
      workspaceId: activeWorkspaceId,
    },
    office: {
      agentProfiles: agentsResponse.agents as AppState["office"]["agentProfiles"],
      skills: [],
      projects: projectsResponse.projects as AppState["office"]["projects"],
      approvals: [],
      activity: [],
      costSummary: null,
      budgetPolicies: [],
      routines: [],
      inboxItems: inboxResponse.items as AppState["office"]["inboxItems"],
      inboxCount: inboxResponse.total_count,
      runs: [],
      dashboard: null,
      tasks: {
        items: [],
        filters: { statuses: [], priorities: [], assigneeIds: [], projectIds: [], search: "" },
        viewMode: "list",
        sortField: "updated",
        sortDir: "desc",
        groupBy: "none",
        nestingEnabled: true,
        isLoading: false,
      },
      meta: metaResponse,
      isLoading: false,
      refetchTrigger: null,
      routing: { byWorkspace: {}, knownProviders: [], preview: { byWorkspace: {} } },
      providerHealth: { byWorkspace: {} },
      runAttempts: { byRunId: {} },
      agentRouting: { byAgentId: {} },
    },
  };

  return (
    <TooltipProvider>
      <StateHydrator initialState={initialState} />
      <div className="flex h-screen">
        <WorkspaceRail workspaces={workspaceItems} activeWorkspaceId={activeWorkspaceId} />
        <OfficeSidebar
          workspaceName={
            workspaceItems.find((w) => w.id === activeWorkspaceId)?.name || "Workspace"
          }
        />
        <div className="flex-1 min-w-0 flex flex-col">
          <OfficeTopbar />
          <main className="flex-1 min-h-0 overflow-y-auto">{children}</main>
        </div>
      </div>
    </TooltipProvider>
  );
}
