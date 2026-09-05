import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { GitHubPageClient } from "@/app/github/github-page-client";
import { GitLabPageClient } from "@/app/gitlab/gitlab-page-client";
import { AzureDevOpsPageClient } from "@/app/azure-devops/azure-devops-page-client";
import { JiraPageClient } from "@/app/jira/jira-page-client";
import { LinearPageClient } from "@/app/linear/linear-page-client";
import { StatsPageClient } from "@/app/stats/stats-page-client";
import { isRangeKey } from "@/app/stats/stats-utils";
import type { RangeKey } from "@/app/stats/stats-utils";
import { TasksPageClient } from "@/app/tasks/tasks-page-client";
import { AutomationDetailPage } from "@/components/runs/automation-detail-page";
import { RunsListPage } from "@/components/runs/runs-list-page";
import { RunsPageClient } from "@/components/runs/runs-page-client";
import {
  AUTOMATIONS_HREF,
  LEGACY_RUNS_PREFIX,
  parseDetailTab,
  RUNS_FEED_VIEW,
} from "@/components/runs/runs-view";
import {
  parseTasksListGroup,
  parseTasksListSort,
  sortTasksForList,
} from "@/lib/tasks/tasks-list-options";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { BootRouteData } from "./boot-payload";
import { fetchJson } from "@/lib/api/client";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { listRepositories, listWorkspaces } from "@/lib/api/domains/workspace-api";
import { resolveDesiredWorkflowId } from "@/lib/kanban/resolve-workflow";
import { useRouter, usePathname, useSearchParams } from "@/lib/routing/client-router";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import {
  PluginErrorBoundary,
  PluginRouteFallback,
} from "@/components/plugins/plugin-error-boundary";
import { PluginPageFrame } from "@/components/plugins/plugin-page";
import { safeDecodePathSegment } from "@/lib/routing/path";
import {
  mapWorkspaceItem,
  promoteLegacyWorkspaceSelection,
  readActiveWorkspaceCookie,
} from "@/lib/routing/route-bootstrap";
import { resolveActiveId } from "@/lib/ssr/resolve-active-id";
import { KanbanRoute } from "./kanban-route";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type {
  ListWorkflowStepsResponse,
  Repository,
  Workflow,
  WorkflowStep,
} from "@/lib/types/http";
import { TaskDetailRoute } from "./task-detail-route";
import { useTranslation } from "react-i18next";

const OfficeRoutes = lazy(() =>
  import("./office-routes").then((mod) => ({ default: mod.OfficeRoutes })),
);
const SettingsRoutes = lazy(() =>
  import("./settings-routes").then((mod) => ({ default: mod.SettingsRoutes })),
);
// Threads mounts a live chat panel per column, so it stays off the initial
// bundle for the boards and lists that never open it.
const ThreadsPageClient = lazy(() =>
  import("@/app/threads/threads-page-client").then((mod) => ({
    default: mod.ThreadsPageClient,
  })),
);

const EMPTY_REPOSITORIES: Repository[] = [];

type SpaRoute =
  | {
      kind: "kanban";
      workspaceId?: string;
      workflowId?: string;
      taskId?: string;
      sessionId?: string;
    }
  | {
      kind: "taskDetail";
      taskId: string;
      sessionId?: string;
      layout?: string | null;
      simple?: string;
      mode?: string;
    }
  | { kind: "tasks" }
  | { kind: "threads" }
  | { kind: "github" }
  | { kind: "gitlab" }
  | { kind: "azure-devops" }
  | { kind: "jira" }
  | { kind: "linear" }
  | { kind: "stats"; range?: RangeKey }
  | { kind: "runs"; view?: string }
  | { kind: "runDetail"; automationId: string; tab?: string; runId?: string }
  | { kind: "settings"; pathname: string }
  | { kind: "office"; pathname: string }
  | { kind: "plugin"; path: string }
  | { kind: "login" }
  | { kind: "setup" }
  | { kind: "invite"; token?: string };

type DataBackedSpaRoute = Exclude<
  SpaRoute,
  { kind: "kanban" | "settings" | "office" | "login" | "setup" | "invite" | "threads" }
>;

type RouteDataState = {
  activeWorkspaceId: string | null;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
};

export function resolveSpaRoute(pathname: string, searchParams: URLSearchParams): SpaRoute {
  const normalized = normalizePath(pathname);
  return (
    resolveTaskDetailRoute(normalized, searchParams) ??
    resolveRunsRoute(normalized, searchParams) ??
    resolveTopLevelRoute(normalized, searchParams) ??
    resolveNestedRoute(normalized) ??
    resolvePluginRoute(normalized) ??
    resolveKanbanRoute(searchParams)
  );
}

/**
 * Dynamic plugin routes (`registry.registerRoute(path, Component)`) — consulted
 * after every static/nested route and before the kanban catch-all, so a plugin
 * can never shadow a first-class route but does own any otherwise-unmatched path.
 */
function resolvePluginRoute(normalized: string): SpaRoute | null {
  const match = pluginRegistry.getRoutes().find((route) => route.path === normalized);
  return match ? { kind: "plugin", path: normalized } : null;
}

/**
 * `/automations` is a list and `/automations/<id>` is that automation's
 * history. The flat cross-automation feed is a view of the list rather than a
 * sibling path, so nothing has to be reserved out of the automation id space.
 *
 * `/runs` is the name this destination shipped under and still resolves, so
 * links already shared or bookmarked keep working.
 */
const AUTOMATION_PREFIXES = [`${AUTOMATIONS_HREF}/`, `${LEGACY_RUNS_PREFIX}/`];

function resolveRunsRoute(normalized: string, searchParams: URLSearchParams): SpaRoute | null {
  if (normalized === AUTOMATIONS_HREF || normalized === LEGACY_RUNS_PREFIX) {
    return { kind: "runs", view: searchParams.get("view") ?? undefined };
  }
  const prefix = AUTOMATION_PREFIXES.find((candidate) => normalized.startsWith(candidate));
  if (!prefix) return null;
  const raw = normalized.slice(prefix.length);
  if (!raw || raw.includes("/")) return null;
  // A malformed escape ("/automations/%") makes decodeURIComponent throw, which
  // would take down route resolution for the whole SPA rather than 404 the one
  // bad link.
  const automationId = safeDecodePathSegment(raw);
  if (!automationId) return null;
  return {
    kind: "runDetail",
    automationId,
    tab: searchParams.get("tab") ?? undefined,
    runId: searchParams.get("run") ?? undefined,
  };
}

function resolveTaskDetailRoute(
  normalized: string,
  searchParams: URLSearchParams,
): SpaRoute | null {
  const taskId = readTaskId(normalized);
  if (!taskId) return null;
  return {
    kind: "taskDetail",
    taskId,
    sessionId: searchParams.get("sessionId") ?? undefined,
    layout: searchParams.get("layout"),
    simple: searchParams.get("simple") ?? undefined,
    mode: searchParams.get("mode") ?? undefined,
  };
}

function resolveTopLevelRoute(normalized: string, searchParams: URLSearchParams): SpaRoute | null {
  switch (normalized) {
    case "/tasks":
      return { kind: "tasks" };
    case "/threads":
      return { kind: "threads" };
    case "/github":
      return { kind: "github" };
    case "/gitlab":
      return { kind: "gitlab" };
    case "/azure-devops":
      return { kind: "azure-devops" };
    case "/jira":
      return { kind: "jira" };
    case "/linear":
      return { kind: "linear" };
    case "/login":
      return { kind: "login" };
    case "/setup":
      return { kind: "setup" };
    case "/invite":
      return { kind: "invite", token: searchParams.get("token") ?? undefined };
    case "/stats": {
      const range = searchParams.get("range");
      return { kind: "stats", range: range && isRangeKey(range) ? range : undefined };
    }
    default:
      return null;
  }
}

function resolveNestedRoute(normalized: string): SpaRoute | null {
  if (normalized === "/settings" || normalized.startsWith("/settings/")) {
    return { kind: "settings", pathname: normalized };
  }
  if (normalized === "/office" || normalized.startsWith("/office/")) {
    return { kind: "office", pathname: normalized };
  }
  return null;
}

function resolveKanbanRoute(searchParams: URLSearchParams): SpaRoute {
  return {
    kind: "kanban",
    workspaceId: searchParams.get("workspaceId") ?? undefined,
    workflowId: searchParams.get("workflowId") ?? undefined,
    taskId: searchParams.get("taskId") ?? undefined,
    sessionId: searchParams.get("sessionId") ?? undefined,
  };
}

export function SpaRoutes({ routeData }: { routeData?: BootRouteData }) {
  // Subscribe so a plugin route registered after first paint (async bundle
  // load) re-resolves without requiring a navigation.
  usePluginRegistry();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const route = resolveSpaRoute(pathname, searchParams);

  // Reaching /login, /setup, or /invite here means the pre-auth gate in
  // main.tsx already decided the app shell should render (authenticated, or
  // auth disabled) — those paths are stale, so bounce to the kanban home.
  if (route.kind === "login" || route.kind === "setup" || route.kind === "invite") {
    return <AuthRouteRedirect />;
  }
  if (route.kind === "plugin") {
    return <PluginRoute path={route.path} />;
  }
  if (route.kind === "kanban") {
    return <KanbanRoute route={route} fallback={<RouteLoading routeNameKey="sidebar:office" />} />;
  }
  if (route.kind === "threads") {
    return (
      <Suspense fallback={<RouteLoading routeNameKey="threads:title" />}>
        <ThreadsPageClient />
      </Suspense>
    );
  }
  if (route.kind === "taskDetail") {
    return (
      <TaskDetailRoute
        taskId={route.taskId}
        sessionId={route.sessionId}
        layout={route.layout}
        simple={route.simple}
        mode={route.mode}
        initialData={routeData?.taskDetail}
      />
    );
  }
  if (route.kind === "settings") {
    return (
      <Suspense fallback={<RouteLoading routeNameKey="common:settings" />}>
        <SettingsRoutes pathname={route.pathname} />
      </Suspense>
    );
  }
  if (route.kind === "office") {
    return (
      <Suspense fallback={<RouteLoading routeNameKey="sidebar:office" />}>
        <OfficeRoutes pathname={route.pathname} />
      </Suspense>
    );
  }

  return <DataBackedRoute route={route} routeData={routeData} />;
}

// Takes the route name as a catalog KEY, not resolved copy: a `t()` at the call
// site would sit in a plain route-dispatch function with no hook of its own.
function RouteLoading({ routeNameKey }: { routeNameKey: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex h-full min-h-0 w-full items-center justify-center bg-background">
      <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
        {t("common:loadingRoute", { routeName: t(routeNameKey) })}
      </p>
    </div>
  );
}

function AuthRouteRedirect() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/");
  }, [router]);
  return null;
}

/**
 * Renders the plugin-registered component for a `kind: "plugin"` route,
 * inside the normal app shell. `PluginPageFrame` gives it the same title-bar
 * chrome first-party pages have (configurable per registration), and the
 * `PluginErrorBoundary` makes a throwing plugin route render a fallback
 * instead of white-screening the rest of the SPA.
 */
function PluginRoute({ path }: { path: string }) {
  const match = pluginRegistry.getRoutes().find((route) => route.path === path);
  if (!match) return null;
  const Component = match.Component;
  return (
    <PluginErrorBoundary context={`route "${path}"`} fallback={<PluginRouteFallback />}>
      <PluginPageFrame registration={match}>
        <Component />
      </PluginPageFrame>
    </PluginErrorBoundary>
  );
}

function DataBackedRoute({
  route,
  routeData,
}: {
  route: DataBackedSpaRoute;
  routeData?: BootRouteData;
}) {
  const tasksPage = route.kind === "tasks" ? routeData?.tasksPage : undefined;
  const routeContext = routeData?.routeContext;
  const bootstrapped = useRouteData({
    skipBootstrap: Boolean(tasksPage || routeContext),
  });
  if (route.kind === "tasks") {
    return <TasksDataRoute bootstrapped={bootstrapped} tasksPage={tasksPage} />;
  }

  const effectiveData = resolveEffectiveRouteData(routeContext, bootstrapped);
  return <ExternalDataRoute route={route} data={effectiveData} />;
}

function TasksDataRoute({
  bootstrapped,
  tasksPage,
}: {
  bootstrapped: RouteDataState;
  tasksPage?: BootRouteData["tasksPage"];
}) {
  const searchParams = useSearchParams();
  const userSettings = useAppStore((state) => state.userSettings);
  const { initialSort, initialGroup } = resolveTasksDataRoutePreferences(
    searchParams,
    tasksPage,
    userSettings,
  );
  const initialData = resolveTasksDataRouteInitialData(bootstrapped, tasksPage, initialSort);
  return (
    <TasksPageClient
      workspaces={[]}
      {...initialData}
      initialSort={initialSort}
      initialGroup={initialGroup}
    />
  );
}

function resolveTasksDataRoutePreferences(
  searchParams: URLSearchParams,
  tasksPage: BootRouteData["tasksPage"] | undefined,
  userSettings: { tasksListSort?: string | null; tasksListGroup?: string | null },
) {
  return {
    initialSort: parseTasksListSort(
      searchParams.get("sort") ?? tasksPage?.tasksListSort ?? userSettings.tasksListSort,
    ),
    initialGroup: parseTasksListGroup(
      searchParams.get("group") ?? tasksPage?.tasksListGroup ?? userSettings.tasksListGroup,
    ),
  };
}

function resolveTasksDataRouteInitialData(
  bootstrapped: RouteDataState,
  tasksPage: BootRouteData["tasksPage"] | undefined,
  initialSort: ReturnType<typeof parseTasksListSort>,
) {
  return {
    initialWorkspaceId: tasksPage?.activeWorkspaceId ?? bootstrapped.activeWorkspaceId ?? undefined,
    initialWorkflows: tasksPage?.workflows ?? bootstrapped.workflows,
    initialRepositories: tasksPage?.repositories ?? bootstrapped.repositories,
    initialTasks: sortTasksForList(tasksPage?.tasks ?? [], initialSort),
    initialTotal: tasksPage?.total ?? 0,
    initialDataLoaded: Boolean(tasksPage),
  };
}

function ExternalDataRoute({
  route,
  data,
}: {
  route: Exclude<DataBackedSpaRoute, { kind: "tasks" }>;
  data: RouteDataState;
}) {
  const workspaceId = data.activeWorkspaceId ?? undefined;
  switch (route.kind) {
    case "github":
      return (
        <GitHubPageClient
          workspaceId={workspaceId}
          workflows={data.workflows}
          steps={data.steps}
          repositories={data.repositories}
        />
      );
    case "gitlab":
      return (
        <GitLabPageClient
          workspaceId={workspaceId}
          workflows={data.workflows}
          steps={data.steps}
          repositories={data.repositories}
        />
      );
    case "azure-devops":
      return (
        <AzureDevOpsPageClient
          workspaceId={workspaceId}
          workflows={data.workflows}
          steps={data.steps}
          repositories={data.repositories}
        />
      );
    case "jira":
      return (
        <JiraPageClient workspaceId={workspaceId} workflows={data.workflows} steps={data.steps} />
      );
    case "linear":
      return (
        <LinearPageClient workspaceId={workspaceId} workflows={data.workflows} steps={data.steps} />
      );
    case "stats":
      return (
        <StatsPageClient workspaceId={workspaceId} activeRange={route.range} initialError={null} />
      );
    case "runs":
      // The flat feed is demoted to a lens over the list, not deleted: with
      // many automations "what happened overnight" is a real question a
      // per-automation view cannot answer.
      return route.view === RUNS_FEED_VIEW ? (
        <RunsPageClient workspaceId={workspaceId} />
      ) : (
        <RunsListPage workspaceId={workspaceId} />
      );
    case "runDetail":
      return (
        <AutomationDetailPage
          automationId={route.automationId}
          tab={parseDetailTab(route.tab)}
          runId={route.runId}
        />
      );
  }
}

function resolveEffectiveRouteData(
  routeContext: BootRouteData["routeContext"],
  fallback: RouteDataState,
): RouteDataState {
  return {
    activeWorkspaceId: routeContext?.activeWorkspaceId ?? fallback.activeWorkspaceId,
    workflows: routeContext?.workflows ?? fallback.workflows,
    steps: routeContext?.steps ?? fallback.steps,
    repositories: routeContext?.repositories ?? fallback.repositories,
  };
}

function useRouteData({
  skipBootstrap = false,
}: {
  skipBootstrap?: boolean;
} = {}): RouteDataState {
  const store = useAppStoreApi();
  const bootstrappedRef = useRef(false);
  const [workflows, setRouteWorkflows] = useState<Workflow[]>([]);
  const [steps, setSteps] = useState<WorkflowStep[]>([]);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const repositories = useAppStore((state) =>
    activeWorkspaceId
      ? (state.repositories.itemsByWorkspaceId[activeWorkspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );

  useEffect(() => {
    if (bootstrappedRef.current) return;
    promoteLegacyWorkspaceSelection(store.getState().workspaces.items);
    if (skipBootstrap) return;
    bootstrappedRef.current = true;
    let cancelled = false;

    async function bootstrap() {
      const [workspacesResponse, settingsResponse] = await Promise.all([
        listWorkspaces({ cache: "no-store" }).catch(() => null),
        fetchUserSettings({ cache: "no-store" }).catch(() => null),
      ]);
      if (cancelled) return;

      const settingsWorkspaceId = settingsResponse?.settings?.workspace_id || null;
      const settingsWorkflowId = settingsResponse?.settings?.workflow_filter_id || null;
      const storeWorkspaceId = store.getState().workspaces.activeId;
      const cookieWorkspaceId = readActiveWorkspaceCookie();
      const workspaceItems =
        workspacesResponse?.workspaces.map(mapWorkspaceItem) ?? store.getState().workspaces.items;
      // One-time migration: a ported instance that has no scoped cookie yet
      // (fresh upgrade) falls back to the legacy name on every boot; once the
      // boot validates a legacy value, copy it into the scoped cookie so
      // later boots are decoupled (legacy name itself stays untouched).
      promoteLegacyWorkspaceSelection(workspaceItems);
      const workspaceId =
        workspaceItems.length > 0
          ? resolveActiveId(
              workspaceItems,
              storeWorkspaceId,
              cookieWorkspaceId,
              settingsWorkspaceId,
            )
          : firstKnownWorkspaceId(storeWorkspaceId, cookieWorkspaceId, settingsWorkspaceId);
      store.getState().hydrate({
        workspaces: { items: workspaceItems, activeId: workspaceId },
        workflows: { items: store.getState().workflows.items, activeId: settingsWorkflowId },
        userSettings: { ...mapUserSettingsResponse(settingsResponse), workspaceId },
      });
      if (!workspaceId) return;

      const [workflowsResponse, repositoriesResponse, stepsResponse] = await Promise.all([
        listWorkflows(workspaceId, { cache: "no-store" }).catch(() => ({ workflows: [] })),
        listRepositories(workspaceId, undefined, { cache: "no-store" }).catch(() => ({
          repositories: [],
        })),
        listWorkspaceWorkflowSteps(workspaceId).catch(() => ({ steps: [], total: 0 })),
      ]);
      if (cancelled) return;

      const workflowItems = workflowsResponse.workflows.map(mapWorkflowItem);
      const activeWorkflowId = resolveDesiredWorkflowId({
        activeWorkflowId: store.getState().workflows.activeId,
        settingsWorkflowId,
        workspaceWorkflows: workflowItems,
      });

      store.getState().hydrate({
        workflows: { items: workflowItems, activeId: activeWorkflowId },
      });
      store.getState().setRepositories(workspaceId, repositoriesResponse.repositories);
      setRouteWorkflows(workflowsResponse.workflows);
      setSteps(stepsResponse.steps);
    }

    void bootstrap();
    return () => {
      cancelled = true;
      bootstrappedRef.current = false;
    };
  }, [skipBootstrap, store]);

  return { activeWorkspaceId, workflows, steps, repositories };
}

function listWorkspaceWorkflowSteps(workspaceId: string) {
  return fetchJson<ListWorkflowStepsResponse>(`/api/v1/workspaces/${workspaceId}/workflow-steps`, {
    cache: "no-store",
  });
}

function firstKnownWorkspaceId(...ids: (string | null | undefined)[]): string | null {
  for (const id of ids) {
    const value = id?.trim();
    if (value) return value;
  }
  return null;
}

function mapWorkflowItem(workflow: Workflow) {
  return {
    id: workflow.id,
    workspaceId: workflow.workspace_id,
    name: workflow.name,
    description: workflow.description ?? null,
    sortOrder: workflow.sort_order ?? 0,
    ...(workflow.agent_profile_id ? { agent_profile_id: workflow.agent_profile_id } : {}),
    ...(workflow.hidden !== undefined ? { hidden: workflow.hidden } : {}),
    ...(workflow.style !== undefined ? { style: workflow.style } : {}),
  };
}

function normalizePath(pathname: string): string {
  if (!pathname || pathname === "/") return "/";
  return pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;
}

function readTaskId(pathname: string): string | undefined {
  for (const prefix of ["/t/", "/tasks/"]) {
    if (!pathname.startsWith(prefix)) continue;
    const suffix = pathname.slice(prefix.length);
    if (!suffix || suffix.includes("/")) return undefined;
    return decodeURIComponent(suffix);
  }
  return undefined;
}
