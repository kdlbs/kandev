import { notFound } from "@/lib/routing/server-navigation";
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
import { readCookies } from "@/lib/server/cookies";
import type { QuerySeedInitialState } from "@/lib/query/seed";
import type { Workspace } from "@/lib/types/http";
import { OfficeTopbar } from "./components/office-topbar";

function resolveActiveOfficeWorkspaceId(
  workspaceItems: { id: string }[],
  cookieWorkspaceId: string | null,
  settingsWorkspaceId: string | null,
): string | null {
  return (
    workspaceItems.find((w) => w.id === cookieWorkspaceId)?.id ??
    workspaceItems.find((w) => w.id === settingsWorkspaceId)?.id ??
    workspaceItems[0]?.id ??
    null
  );
}

function mapWorkspaceItem(ws: Workspace): Workspace {
  return {
    id: ws.id,
    name: ws.name,
    description: ws.description ?? null,
    owner_id: ws.owner_id,
    default_executor_id: ws.default_executor_id ?? null,
    default_environment_id: ws.default_environment_id ?? null,
    default_agent_profile_id: ws.default_agent_profile_id ?? null,
    default_config_agent_profile_id: ws.default_config_agent_profile_id ?? null,
    office_workflow_id: ws.office_workflow_id ?? undefined,
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

  const [workspacesResponse, userSettingsResponse, metaResponse, cookieStore] = await Promise.all([
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
    getMeta({ cache: "no-store" }).catch(() => null),
    readCookies().catch(() => null),
  ]);

  // Only office workspaces can be active inside /office, but hydrate every
  // workspace into the shared sidebar picker so users can switch back to Kanban.
  const officeWorkspaceItems = workspacesResponse.workspaces
    .filter((ws) => ws.office_workflow_id)
    .map(mapWorkspaceItem);
  const workspaceItems = workspacesResponse.workspaces.map(mapWorkspaceItem);
  const settingsWorkspaceId = userSettingsResponse?.settings?.workspace_id || null;
  // office-active-workspace cookie is set by the setup wizard and workspace rail so the
  // layout can load the correct workspace even when userSettings.workspace_id still points
  // to a kanban workspace (we never write to userSettings from office).
  const cookieWorkspaceId = cookieStore?.get("office-active-workspace")?.value ?? null;
  const activeWorkspaceId = resolveActiveOfficeWorkspaceId(
    officeWorkspaceItems,
    cookieWorkspaceId,
    settingsWorkspaceId,
  );

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

  const initialState: QuerySeedInitialState = {
    workspaces: {
      items: workspaceItems,
      activeId: activeWorkspaceId,
    },
    userSettings: {
      ...mapUserSettingsResponse(userSettingsResponse),
      workspaceId: activeWorkspaceId,
    },
    office: {
      agents: agentsResponse.agents,
      projects: projectsResponse.projects,
      inboxItems: inboxResponse.items,
      inboxCount: inboxResponse.total_count,
      meta: metaResponse,
    },
  };

  return (
    <TooltipProvider>
      <StateHydrator initialState={initialState} />
      <div className="flex h-full min-h-0 flex-col">
        <OfficeTopbar />
        <main className="flex-1 min-h-0 overflow-y-auto">{children}</main>
      </div>
    </TooltipProvider>
  );
}
