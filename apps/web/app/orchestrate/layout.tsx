import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateHydrator } from "@/components/state-hydrator";
import { listWorkspaces, fetchUserSettings } from "@/lib/api";
import { getMeta } from "@/lib/api/domains/orchestrate-api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { AppState } from "@/lib/state/store";
import { WorkspaceRail } from "./components/workspace-rail";
import { OrchestrateSidebar } from "./components/orchestrate-sidebar";
import { OrchestrateTopbar } from "./components/orchestrate-topbar";

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

export default async function OrchestrateLayout({ children }: { children: React.ReactNode }) {
  const [workspacesResponse, userSettingsResponse, metaResponse] = await Promise.all([
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
    getMeta({ cache: "no-store" }).catch(() => null),
  ]);

  const workspaceItems = workspacesResponse.workspaces.map(mapWorkspaceItem);
  const settingsWorkspaceId = userSettingsResponse?.settings?.workspace_id || null;
  const activeWorkspaceId =
    workspaceItems.find((w) => w.id === settingsWorkspaceId)?.id ?? workspaceItems[0]?.id ?? null;

  const initialState: Partial<AppState> = {
    workspaces: {
      items: workspaceItems,
      activeId: activeWorkspaceId,
    },
    userSettings: {
      ...mapUserSettingsResponse(userSettingsResponse),
      workspaceId: activeWorkspaceId,
    },
    orchestrate: {
      agentInstances: [],
      skills: [],
      projects: [],
      approvals: [],
      activity: [],
      costSummary: null,
      budgetPolicies: [],
      routines: [],
      inboxItems: [],
      inboxCount: 0,
      wakeups: [],
      dashboard: null,
      issues: {
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
    },
  };

  return (
    <TooltipProvider>
      <StateHydrator initialState={initialState} />
      <div className="flex h-screen">
        <WorkspaceRail workspaces={workspaceItems} activeWorkspaceId={activeWorkspaceId} />
        <OrchestrateSidebar
          workspaceName={
            workspaceItems.find((w) => w.id === activeWorkspaceId)?.name || "Workspace"
          }
        />
        <div className="flex-1 min-w-0 flex flex-col">
          <OrchestrateTopbar />
          <main className="flex-1 min-h-0 overflow-y-auto">{children}</main>
        </div>
      </div>
    </TooltipProvider>
  );
}
