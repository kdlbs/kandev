import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import ProjectDetailPage from "@/app/office/projects/[id]/page";
import AgentDetailLayout from "@/app/office/agents/[id]/layout";
import AgentChannelsPage from "@/app/office/agents/[id]/channels/page";
import AgentConfigurationPage from "@/app/office/agents/[id]/configuration/page";
import AgentInstructionsPage from "@/app/office/agents/[id]/instructions/page";
import AgentMemoryPage from "@/app/office/agents/[id]/memory/page";
import AgentPermissionsPage from "@/app/office/agents/[id]/permissions/page";
import AgentSkillsPage from "@/app/office/agents/[id]/skills/page";
import { AgentsPageClient } from "@/app/office/agents/agents-page-client";
import { OfficeTopbar } from "@/app/office/components/office-topbar";
import { InboxPageClient } from "@/app/office/inbox/inbox-page-client";
import { OfficePageClient } from "@/app/office/page-client";
import { ProjectsPageClient } from "@/app/office/projects/projects-page-client";
import { SetupWizard } from "@/app/office/setup/setup-wizard";
import { loadSetupRouteData } from "@/app/office/setup/setup-route-data";
import type { SetupWizardRouteProps } from "@/app/office/setup/setup-route-data";
import ProviderRoutingPage from "@/app/office/workspace/routing/page";
import { RoutinesPageClient } from "@/app/office/routines/routines-page-client";
import SettingsPage from "@/app/office/workspace/settings/page";
import SyncPage from "@/app/office/workspace/settings/sync/page";
import OrgPage from "@/app/office/workspace/org/page";
import IssueDetailPage from "@/app/office/tasks/[id]/page";
import { TasksPageClient as OfficeTasksPageClient } from "@/app/office/tasks/tasks-page-client";
import { ActivityPageClient } from "@/app/office/workspace/activity/activity-page-client";
import { CostsPageClient } from "@/app/office/workspace/costs/costs-page-client";
import { SkillsPageClient } from "@/app/office/workspace/skills/skills-page-client";
import { fetchUserSettings, listWorkspaces } from "@/lib/api";
import {
  getInbox,
  getMeta,
  getOnboardingState,
  listAgentProfiles,
  listProjects,
} from "@/lib/api/domains/office-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useWorkspaces } from "@/hooks/domains/workspace/use-workspaces";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { mapWorkspaceItem, readActiveWorkspaceCookie } from "@/lib/routing/route-bootstrap";
import { qk } from "@/lib/query/keys";
import type { Workspace } from "@/lib/types/http";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import {
  AgentDashboardRoute,
  AgentRunDetailRoute,
  AgentRunsRoute,
} from "./office-agent-client-routes";
import { RoutineDetailRoute } from "./office-routine-client-routes";
import { TooltipProvider } from "@kandev/ui/tooltip";

type RouteRenderer = () => React.ReactNode;

const OFFICE_ROUTES: Record<string, RouteRenderer> = {
  "/office": () => <OfficePageClient initialDashboard={null} />,
  "/office/inbox": () => <InboxPageClient initialItems={[]} initialCount={0} />,
  "/office/tasks": () => <OfficeTasksPageClient initialIssues={[]} />,
  "/office/projects": () => <ProjectsPageClient initialProjects={[]} />,
  "/office/routines": () => <RoutinesPageClient initialRoutines={[]} />,
  "/office/agents": () => <AgentsPageClient initialAgents={[]} />,
  "/office/workspace/activity": () => <ActivityPageClient initialActivity={[]} />,
  "/office/workspace/costs": () => <CostsPageClient initialCostSummary={null} />,
  "/office/workspace/skills": () => <SkillsPageClient initialSkills={[]} />,
  "/office/workspace/routing": () => <ProviderRoutingPage />,
  "/office/workspace/settings": () => <SettingsPage />,
  "/office/workspace/settings/sync": () => <SyncPage />,
  "/office/workspace/org": () => <OrgPage />,
};

export function OfficeRoutes({ pathname }: { pathname: string }) {
  const router = useRouter();
  const officeEnabled = useAppStore((state) => state.features.office);
  const { items: workspaceItems, activeId: activeWorkspaceId } = useWorkspaces();
  const normalizedPathname = normalizeOfficePath(pathname);
  const routeWorkspaceId = useSearchParams().get("workspaceId");
  const bootstrap = useOfficeRouteBootstrap(officeEnabled, routeWorkspaceId);
  const setupRedirectHref = resolveOfficeHomeSetupRedirect(
    normalizedPathname,
    bootstrap.complete,
    bootstrap.onboardingComplete,
    workspaceItems,
  );

  useEffect(() => {
    if (!officeEnabled || !setupRedirectHref) return;
    router.replace(setupRedirectHref);
  }, [officeEnabled, router, setupRedirectHref]);

  if (!officeEnabled) {
    return <OfficeUnavailable />;
  }

  if (normalizedPathname === "/office/setup") {
    return <OfficeSetupRoute />;
  }

  if (
    setupRedirectHref ||
    shouldHoldOfficeHomeForBootstrap(
      normalizedPathname,
      bootstrap.complete,
      workspaceItems,
      activeWorkspaceId,
    )
  ) {
    return <OfficeRouteLoading />;
  }

  return (
    <TooltipProvider>
      <div className="flex h-full min-h-0 flex-col">
        <OfficeTopbar />
        <main className="flex-1 min-h-0 overflow-y-auto">
          {renderOfficeRoute(normalizedPathname)}
        </main>
      </div>
    </TooltipProvider>
  );
}

export function officeRouteKey(pathname: string): string {
  return normalizeOfficePath(pathname);
}

export function resolveOfficeHomeSetupRedirect(
  pathname: string,
  bootstrapComplete: boolean,
  onboardingComplete: boolean | null,
  workspaceItems: Workspace[],
): "/office/setup" | "/office/setup?mode=new" | null {
  if (pathname !== "/office" || !bootstrapComplete) return null;
  if (onboardingComplete === false) return "/office/setup";
  return hasOfficeWorkspace(workspaceItems) ? null : "/office/setup?mode=new";
}

function renderOfficeRoute(pathname: string) {
  const agentRoute = matchAgentRoute(pathname);
  if (agentRoute) {
    return renderAgentRoute(agentRoute);
  }

  const projectId = matchSingle(pathname, /^\/office\/projects\/([^/]+)$/);
  if (projectId) {
    return <ProjectDetailPage params={Promise.resolve({ id: projectId })} />;
  }

  const routineId = matchSingle(pathname, /^\/office\/routines\/([^/]+)$/);
  if (routineId) {
    return <RoutineDetailRoute routineId={routineId} />;
  }

  const taskId = matchSingle(pathname, /^\/office\/tasks\/([^/]+)$/);
  if (taskId) {
    return <IssueDetailPage params={Promise.resolve({ id: taskId })} />;
  }

  return OFFICE_ROUTES[pathname]?.() ?? <OfficeRouteFallback pathname={pathname} />;
}

type OfficeBootstrapState = {
  complete: boolean;
  onboardingComplete: boolean | null;
};

function useOfficeRouteBootstrap(
  officeEnabled: boolean,
  routeWorkspaceId: string | null,
): OfficeBootstrapState {
  const store = useAppStoreApi();
  const queryClient = useQueryClient();
  const [bootstrap, setBootstrap] = useState<OfficeBootstrapState>({
    complete: false,
    onboardingComplete: null,
  });

  useEffect(() => {
    if (!officeEnabled) {
      setBootstrap({ complete: false, onboardingComplete: null });
      return;
    }
    let cancelled = false;
    setBootstrap({ complete: false, onboardingComplete: null });

    async function loadBootstrapState() {
      const [onboardingResponse, workspacesResponse, userSettingsResponse, metaResponse] =
        await Promise.all([
          getOnboardingState({ cache: "no-store" }).catch(() => ({ completed: true })),
          listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
          fetchUserSettings({ cache: "no-store" }).catch(() => null),
          getMeta({ cache: "no-store" }).catch(() => null),
        ]);
      if (cancelled) return;

      const onboardingComplete = onboardingResponse.completed;
      if (!onboardingComplete) {
        setBootstrap({ complete: true, onboardingComplete: false });
        return;
      }

      const workspaceItems = workspacesResponse.workspaces.map(mapWorkspaceItem);
      queryClient.setQueryData(qk.workspaces.all(), workspaceItems);
      const officeWorkspaceItems = workspaceItems.filter(
        (workspace) => workspace.office_workflow_id,
      );
      const activeWorkspaceId = resolveActiveOfficeWorkspaceId(
        officeWorkspaceItems,
        routeWorkspaceId,
        readActiveWorkspaceCookie(),
        userSettingsResponse?.settings?.workspace_id ?? null,
      );

      store.getState().hydrate({
        workspaces: { activeId: activeWorkspaceId },
        userSettings: {
          ...mapUserSettingsResponse(userSettingsResponse),
          workspaceId: activeWorkspaceId,
        },
      });
      store.getState().setMeta(metaResponse);

      if (!activeWorkspaceId) {
        setBootstrap({ complete: true, onboardingComplete });
        return;
      }

      const [agentsResponse, projectsResponse, inboxResponse] = await Promise.all([
        listAgentProfiles(activeWorkspaceId, { cache: "no-store" }).catch(() => ({ agents: [] })),
        listProjects(activeWorkspaceId, { cache: "no-store" }).catch(() => ({ projects: [] })),
        getInbox(activeWorkspaceId, { cache: "no-store" }).catch(() => ({
          items: [],
          total_count: 0,
        })),
      ]);
      if (cancelled) return;

      store.getState().setOfficeAgentProfiles(agentsResponse.agents);
      store.getState().setProjects(projectsResponse.projects);
      store.getState().setInboxItems(inboxResponse.items);
      store.getState().setInboxCount(inboxResponse.total_count);
      setBootstrap({ complete: true, onboardingComplete });
    }

    void loadBootstrapState().catch(() => {
      if (!cancelled) setBootstrap({ complete: true, onboardingComplete: true });
    });
    return () => {
      cancelled = true;
    };
  }, [officeEnabled, queryClient, routeWorkspaceId, store]);

  return bootstrap;
}

function resolveActiveOfficeWorkspaceId(
  workspaceItems: { id: string }[],
  routeWorkspaceId: string | null,
  cookieWorkspaceId: string | null,
  settingsWorkspaceId: string | null,
): string | null {
  return (
    workspaceItems.find((workspace) => workspace.id === routeWorkspaceId)?.id ??
    workspaceItems.find((workspace) => workspace.id === cookieWorkspaceId)?.id ??
    workspaceItems.find((workspace) => workspace.id === settingsWorkspaceId)?.id ??
    workspaceItems[0]?.id ??
    null
  );
}

type AgentRouteMatch = {
  id: string;
  tab: string;
  runId?: string;
  bare?: boolean;
};

function renderAgentRoute(route: AgentRouteMatch) {
  const params = Promise.resolve({ id: route.id });
  if (route.bare) {
    return (
      <AgentDetailLayout params={params}>
        <AgentBareRouteRedirect agentId={route.id} />
      </AgentDetailLayout>
    );
  }

  return (
    <AgentDetailLayout params={params}>{renderAgentRouteBody(route, params)}</AgentDetailLayout>
  );
}

function AgentBareRouteRedirect({ agentId }: { agentId: string }) {
  const router = useRouter();

  useEffect(() => {
    router.replace(`/office/agents/${encodeURIComponent(agentId)}/dashboard`);
  }, [agentId, router]);

  return <AgentDashboardRoute agentId={agentId} />;
}

function renderAgentRouteBody(route: AgentRouteMatch, params: Promise<{ id: string }>) {
  switch (route.tab) {
    case "dashboard":
      return <AgentDashboardRoute agentId={route.id} />;
    case "instructions":
      return <AgentInstructionsPage params={params} />;
    case "skills":
      return <AgentSkillsPage params={params} />;
    case "configuration":
      return <AgentConfigurationPage params={params} />;
    case "permissions":
      return <AgentPermissionsPage params={params} />;
    case "runs":
      if (route.runId) {
        return <AgentRunDetailRoute agentId={route.id} runId={route.runId} />;
      }
      return <AgentRunsRoute agentId={route.id} />;
    case "memory":
      return <AgentMemoryPage params={params} />;
    case "channels":
      return <AgentChannelsPage params={params} />;
    default:
      return <AgentDashboardRoute agentId={route.id} />;
  }
}

function matchAgentRoute(pathname: string): AgentRouteMatch | null {
  const match = pathname.match(/^\/office\/agents\/([^/]+)(?:\/([^/]+))?(?:\/([^/]+))?$/);
  if (!match?.[1]) return null;
  const id = decodeURIComponent(match[1]);
  const bare = !match[2];
  const tab = bare ? "dashboard" : decodeURIComponent(match[2]);
  const runId = tab === "runs" && match[3] ? decodeURIComponent(match[3]) : undefined;
  return { id, tab, runId, bare };
}

function OfficeUnavailable() {
  return (
    <div className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground">
      Office is not enabled for this runtime.
    </div>
  );
}

function OfficeRouteLoading() {
  return (
    <div className="flex h-full items-center justify-center">
      <span className="text-sm text-muted-foreground">Loading...</span>
    </div>
  );
}

function shouldHoldOfficeHomeForBootstrap(
  pathname: string,
  bootstrapComplete: boolean,
  workspaceItems: Workspace[],
  activeWorkspaceId: string | null,
): boolean {
  return (
    pathname === "/office" &&
    !bootstrapComplete &&
    (!hasOfficeWorkspace(workspaceItems) || !activeWorkspaceId)
  );
}

function hasOfficeWorkspace(workspaceItems: Workspace[]): boolean {
  return workspaceItems.some((workspace) => Boolean(workspace.office_workflow_id));
}

type OfficeSetupState =
  | { status: "loading" }
  | { status: "ready"; props: SetupWizardRouteProps }
  | { status: "error"; message: string };

function OfficeSetupRoute() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const mode = searchParams.get("mode") ?? undefined;
  const [state, setState] = useState<OfficeSetupState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setState({ status: "loading" });
      try {
        const data = await loadSetupRouteData(mode);
        if (cancelled) return;
        if (data.kind === "redirect") {
          router.replace(data.href);
          return;
        }
        setState({ status: "ready", props: data.props });
      } catch (error) {
        if (cancelled) return;
        setState({
          status: "error",
          message: error instanceof Error ? error.message : "Failed to load setup",
        });
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [mode, router]);

  if (state.status === "ready") {
    return <SetupWizard {...state.props} />;
  }

  if (state.status === "error") {
    return (
      <div className="flex h-full items-center justify-center p-6 text-sm text-destructive">
        {state.message}
      </div>
    );
  }

  return <OfficeRouteLoading />;
}

function OfficeRouteFallback({ pathname }: { pathname: string }) {
  return (
    <div className="p-6 text-sm text-muted-foreground">
      This Office route is handled by the SPA shell, but its dedicated client page is still being
      ported: <span className="font-mono">{pathname}</span>
    </div>
  );
}

function matchSingle(pathname: string, pattern: RegExp): string | null {
  const match = pathname.match(pattern);
  return match?.[1] ? decodeURIComponent(match[1]) : null;
}

function normalizeOfficePath(pathname: string): string {
  if (!pathname || pathname === "/office/") return "/office";
  return pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;
}
