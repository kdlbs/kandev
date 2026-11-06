import {
  readJourneyRepositories,
  readJourneyWorkspaces,
  readJourneyWorkflows,
  readJourneyUserSettings,
} from "@/hooks/journey-metadata-resources";
/* eslint-disable max-lines -- route dispatch and its bootstrap remain one public boundary */
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
import { TasksWorkspaceError } from "@/app/tasks/tasks-workspace-error";
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
  isTaskListFacetOption,
  sortTasksForList,
} from "@/lib/tasks/tasks-list-options";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { BootRouteData } from "./boot-payload";
import { fetchJson } from "@/lib/api/client";
import { resolveDesiredWorkflowId } from "@/lib/kanban/resolve-workflow";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import {
  PluginErrorBoundary,
  PluginRouteFallback,
} from "@/components/plugins/plugin-error-boundary";
import { PluginPageFrame } from "@/components/plugins/plugin-page";
import { safeDecodePathSegment } from "@/lib/routing/path";
import {
  mapWorkflowItem,
  mapWorkspaceItem,
  promoteLegacyWorkspaceSelection,
  readActiveWorkspaceCookie,
} from "@/lib/routing/route-bootstrap";
import { resolveActiveId } from "@/lib/ssr/resolve-active-id";
import { KanbanRoute } from "./kanban-route";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import {
  classifyWorkspaceContextReadError,
  isCurrentWorkspaceContext,
  retryAfterMilliseconds,
} from "@/lib/state/workspace-context";
import type {
  ListWorkflowStepsResponse,
  Repository,
  Workflow,
  WorkflowStep,
} from "@/lib/types/http";
import { TaskDetailRoute } from "./task-detail-route";
import { CanvasRoute } from "./canvas-route";
import { CoordinatorRoute } from "./coordinator-route";
import { NeedsYouInboxRoute } from "./needs-you-inbox-route";
import { AuthRouteRedirect, RouteLoading } from "./spa-route-chrome";
import { NEEDS_YOU_INBOX_HREF } from "@/lib/navigation/needs-you-inbox-destination";
import { generateUUID } from "@/lib/utils";
import { canUseTasksListBootData, type TasksListMetadataReady } from "@/lib/tasks/tasks-list-query";
import { isIntegratedControlPath } from "@/lib/navigation/integrated-control";
const IntegratedControlPage = lazy(() =>
  import("@/components/integrated-control/integrated-control-page").then((mod) => ({
    default: mod.IntegratedControlPage,
  })),
);

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
const EMPTY_TASKS_METADATA: TasksListMetadataReady = { workflows: false, repositories: false };
const READY_TASKS_METADATA: TasksListMetadataReady = { workflows: true, repositories: true };

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
  | { kind: "canvas"; canvasId: string }
  | { kind: "canvasSettings"; workspaceId: string }
  | {
      kind: "coordinator";
      workspaceId: string;
      coordinatorId: string | null;
      view: "needs-you" | "queue";
    }
  | { kind: "needsYouInbox" }
  | { kind: "integratedControl" }
  | { kind: "settings"; pathname: string }
  | { kind: "office"; pathname: string }
  | { kind: "plugin"; path: string }
  | { kind: "login" }
  | { kind: "setup" }
  | { kind: "invite"; token?: string };

type DataBackedSpaRoute = Exclude<
  SpaRoute,
  {
    kind:
      | "kanban"
      | "canvas"
      | "canvasSettings"
      | "coordinator"
      | "needsYouInbox"
      | "integratedControl"
      | "settings"
      | "office"
      | "login"
      | "setup"
      | "invite"
      | "threads";
  }
>;

type RouteDataState = {
  activeWorkspaceId: string | null;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
};

type RouteBootstrapState = RouteDataState & {
  workspaceStatus: "loading" | "ready" | "not_found" | "error";
  metadataLoaded: TasksListMetadataReady;
};

type SpaRouteOptions = {
  canvasesEnabled?: boolean;
  needsYouInboxEnabled?: boolean;
  coordinatorEnabled?: boolean;
  integratedControlEnabled?: boolean;
};

export function resolveSpaRoute(
  pathname: string,
  searchParams: URLSearchParams,
  options: SpaRouteOptions = {},
): SpaRoute {
  const normalized = normalizePath(pathname);
  return (
    (options.integratedControlEnabled && isIntegratedControlPath(normalized)
      ? { kind: "integratedControl" as const }
      : null) ??
    resolveTaskDetailRoute(normalized, searchParams) ??
    resolveRunsRoute(normalized, searchParams) ??
    resolveTopLevelRoute(normalized, searchParams) ??
    resolveCanvasRoute(normalized, options.canvasesEnabled === true) ??
    resolveNeedsYouInboxRoute(normalized, options.needsYouInboxEnabled === true) ??
    resolveCoordinatorRoute(normalized, options.coordinatorEnabled === true) ??
    resolveNestedRoute(normalized) ??
    resolvePluginRoute(normalized) ??
    resolveKanbanRoute(searchParams)
  );
}

// The destination resolves only where the flag is enabled; disabled falls
// through to the kanban catch-all like an unrecognized path would.
function resolveNeedsYouInboxRoute(normalized: string, enabled: boolean): SpaRoute | null {
  if (!enabled) return null;
  return normalized === NEEDS_YOU_INBOX_HREF ? { kind: "needsYouInbox" } : null;
}

// The destination resolves only where the flag is enabled; disabled falls
// through to the kanban catch-all like an unrecognized path would (same
// shape as resolveNeedsYouInboxRoute).
function resolveCoordinatorRoute(normalized: string, enabled: boolean): SpaRoute | null {
  if (!enabled) return null;

  const queueMatch = normalized.match(/^\/workspaces\/([^/]+)\/coordinator\/([^/]+)\/queue$/);
  if (queueMatch) {
    const workspaceId = safeDecodePathSegment(queueMatch[1]);
    const coordinatorId = safeDecodePathSegment(queueMatch[2]);
    if (!workspaceId || !coordinatorId) return null;
    return { kind: "coordinator", workspaceId, coordinatorId, view: "queue" };
  }

  const withId = normalized.match(/^\/workspaces\/([^/]+)\/coordinator\/([^/]+)$/);
  if (withId) {
    const workspaceId = safeDecodePathSegment(withId[1]);
    const coordinatorId = safeDecodePathSegment(withId[2]);
    if (!workspaceId || !coordinatorId) return null;
    return { kind: "coordinator", workspaceId, coordinatorId, view: "needs-you" };
  }

  const generic = normalized.match(/^\/workspaces\/([^/]+)\/coordinator$/);
  if (generic) {
    const workspaceId = safeDecodePathSegment(generic[1]);
    return workspaceId
      ? { kind: "coordinator", workspaceId, coordinatorId: null, view: "needs-you" }
      : null;
  }

  return null;
}

function resolveCanvasRoute(normalized: string, canvasesEnabled: boolean): SpaRoute | null {
  if (!canvasesEnabled) return null;
  const direct = normalized.match(/^\/canvases\/([^/]+)$/);
  if (direct) {
    const canvasId = safeDecodePathSegment(direct[1]);
    return canvasId ? { kind: "canvas", canvasId } : null;
  }

  const settings = normalized.match(/^\/settings\/workspaces\/([^/]+)\/canvases$/);
  if (settings) {
    const workspaceId = safeDecodePathSegment(settings[1]);
    return workspaceId ? { kind: "canvasSettings", workspaceId } : null;
  }
  return null;
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

/** The location resolved with the feature options the router itself uses, so
 *  every reader of "what page is this" agrees with `SpaRoutes`. */
export function useResolvedSpaRoute(): SpaRoute {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const canvasesEnabled = useFeature("canvases");
  const needsYouInboxEnabled = useFeature("needsYouInbox");
  const coordinatorEnabled = useFeature("coordinator");
  const integratedControlEnabled = useFeature("integratedControl");
  return resolveSpaRoute(pathname, searchParams, {
    canvasesEnabled,
    needsYouInboxEnabled,
    coordinatorEnabled,
    integratedControlEnabled,
  });
}

export function SpaRoutes({ routeData }: { routeData?: BootRouteData }) {
  // Subscribe so a plugin route registered after first paint (async bundle
  // load) re-resolves without requiring a navigation.
  usePluginRegistry();
  const route = useResolvedSpaRoute();
  const tasksPage = useTasksBootPage(route, routeData?.tasksPage);
  if (route.kind === "integratedControl") {
    return (
      <Suspense fallback={<RouteLoading routeNameKey="sidebar:integratedControl.title" />}>
        <IntegratedControlPage />
      </Suspense>
    );
  }
  return <StandardSpaRoutes route={route} routeData={routeData && { ...routeData, tasksPage }} />;
}

function useTasksBootPage(route: SpaRoute, tasksPage: BootRouteData["tasksPage"]) {
  const bootPage = useRef(tasksPage);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const requestedWorkspaceId = useSearchParams().get("workspace");
  if (
    route.kind !== "tasks" ||
    bootPage.current?.activeWorkspaceId !== activeWorkspaceId ||
    (requestedWorkspaceId !== null && requestedWorkspaceId !== activeWorkspaceId)
  ) {
    bootPage.current = undefined;
  }
  return bootPage.current;
}

function StandardSpaRoutes({
  route,
  routeData,
}: {
  route: Exclude<SpaRoute, { kind: "integratedControl" }>;
  routeData?: BootRouteData;
}) {
  const canvasesEnabled = useFeature("canvases");
  const needsYouInboxEnabled = useFeature("needsYouInbox");
  const coordinatorEnabled = useFeature("coordinator");

  // Reaching /login, /setup, or /invite here means the pre-auth gate in
  // main.tsx already decided the app shell should render (authenticated, or
  // auth disabled) — those paths are stale, so bounce to the kanban home.
  if (route.kind === "login" || route.kind === "setup" || route.kind === "invite") {
    return <AuthRouteRedirect />;
  }
  if (route.kind === "canvas" || route.kind === "canvasSettings") {
    return <CanvasRoute route={route} enabled={canvasesEnabled} />;
  }
  if (route.kind === "needsYouInbox") {
    return <NeedsYouInboxRoute enabled={needsYouInboxEnabled} />;
  }
  if (route.kind === "coordinator") {
    return (
      <CoordinatorRoute
        enabled={coordinatorEnabled}
        view={route.view}
        workspaceId={route.workspaceId}
        coordinatorId={route.coordinatorId}
      />
    );
  }
  if (route.kind === "plugin") {
    return <PluginRoute path={route.path} />;
  }
  if (route.kind === "kanban") {
    return <KanbanRoute route={route} fallback={<RouteLoading routeNameKey="sidebar:home" />} />;
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
  const searchParams = useSearchParams();
  const bootstrapped = useRouteData({
    skipBootstrap: Boolean(tasksPage || (route.kind !== "tasks" && routeContext)),
    requestedWorkspaceId: route.kind === "tasks" ? searchParams.get("workspace") : null,
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
  bootstrapped: RouteBootstrapState;
  tasksPage?: BootRouteData["tasksPage"];
}) {
  const searchParams = useSearchParams();
  const userSettings = useAppStore((state) => state.userSettings);
  const { initialSort, initialGroup } = resolveTasksDataRoutePreferences(
    searchParams,
    tasksPage,
    userSettings,
  );
  const initialData = resolveTasksDataRouteInitialData(
    bootstrapped,
    tasksPage,
    initialSort,
    searchParams,
  );
  if (bootstrapped.workspaceStatus === "error" || bootstrapped.workspaceStatus === "not_found") {
    return <TasksWorkspaceError missing={bootstrapped.workspaceStatus === "not_found"} />;
  }
  const requestedWorkspaceId = searchParams.get("workspace");
  if (
    bootstrapped.workspaceStatus !== "ready" ||
    (requestedWorkspaceId !== null && requestedWorkspaceId !== bootstrapped.activeWorkspaceId)
  ) {
    return <RouteLoading routeNameKey="common:tasks" />;
  }
  return (
    <TasksPageClient
      key={initialData.initialWorkspaceId}
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
  bootstrapped: RouteBootstrapState,
  tasksPage: BootRouteData["tasksPage"] | undefined,
  initialSort: ReturnType<typeof parseTasksListSort>,
  searchParams: URLSearchParams,
) {
  const seed = tasksPage ?? bootstrapped;
  const workspaceId = searchParams.get("workspace") ?? seed.activeWorkspaceId;
  const eligible =
    tasksPage !== undefined &&
    canUseTasksListBootData(searchParams, seed.activeWorkspaceId, workspaceId);
  const tasks = eligible ? (tasksPage.tasks ?? []) : [];
  return {
    initialWorkspaceId: seed.activeWorkspaceId ?? undefined,
    initialRepositories: seed.repositories ?? bootstrapped.repositories,
    initialTasks: isTaskListFacetOption(initialSort)
      ? [...tasks]
      : sortTasksForList(tasks, initialSort),
    initialTotal: eligible ? (tasksPage.total ?? 0) : 0,
    initialDataLoaded: eligible,
    initialMetadataLoaded: tasksPage ? READY_TASKS_METADATA : bootstrapped.metadataLoaded,
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

// eslint-disable-next-line max-lines-per-function, complexity -- route bootstrap owns one guarded async lifecycle
function useRouteData({
  skipBootstrap = false,
  requestedWorkspaceId = null,
}: {
  skipBootstrap?: boolean;
  requestedWorkspaceId?: string | null;
} = {}): RouteBootstrapState {
  const store = useAppStoreApi();
  const bootstrappedRef = useRef(false);
  const [workflows, setRouteWorkflows] = useState<Workflow[]>([]);
  const [steps, setSteps] = useState<WorkflowStep[]>([]);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceContextRetryVersion = useAppStore(
    (state) => state.workspaceContextRead.retryVersion,
  );
  const [resolution, setResolution] = useState({
    requestedWorkspaceId,
    retryVersion: workspaceContextRetryVersion,
    status: "loading" as RouteBootstrapState["workspaceStatus"],
    metadataLoaded: EMPTY_TASKS_METADATA,
  });
  const repositories = useAppStore((state) =>
    activeWorkspaceId
      ? (state.repositories.itemsByWorkspaceId[activeWorkspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );

  // eslint-disable-next-line max-lines-per-function -- route bootstrap keeps its cancellation guard with its effect
  useEffect(() => {
    if (bootstrappedRef.current) return;
    promoteLegacyWorkspaceSelection(store.getState().workspaces.items);
    if (skipBootstrap) return;
    bootstrappedRef.current = true;
    let cancelled = false;
    const controller = new AbortController();
    const requestIds = new Map<
      "workflows" | "repositories" | "steps",
      { workspaceId: string; generation: number; requestId: string }
    >();

    // eslint-disable-next-line max-lines-per-function, complexity -- each branch preserves a distinct workspace read outcome
    async function bootstrap() {
      const [workspacesResponse, settingsResponse] = await Promise.all([
        readJourneyWorkspaces(store, { signal: controller.signal }).catch(() => null),
        readJourneyUserSettings(store, { signal: controller.signal }).catch(() => null),
      ]);
      if (cancelled) return;

      const nextResolution = {
        requestedWorkspaceId,
        retryVersion: workspaceContextRetryVersion,
        metadataLoaded: EMPTY_TASKS_METADATA,
      };
      if (requestedWorkspaceId !== null) {
        if (!workspacesResponse) {
          setResolution({ ...nextResolution, status: "error" });
          return;
        }
        if (!workspacesResponse.workspaces.some((item) => item.id === requestedWorkspaceId)) {
          setResolution({ ...nextResolution, status: "not_found" });
          return;
        }
      }

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
        requestedWorkspaceId ??
        (workspaceItems.length > 0
          ? resolveActiveId(
              workspaceItems,
              storeWorkspaceId,
              cookieWorkspaceId,
              settingsWorkspaceId,
            )
          : firstKnownWorkspaceId(storeWorkspaceId, cookieWorkspaceId, settingsWorkspaceId));
      const workspaceBeforeHydration = store.getState().workspaces.activeId;
      store.getState().hydrate({
        workspaces: { items: workspaceItems, activeId: workspaceBeforeHydration },
        workflows: { items: store.getState().workflows.items, activeId: settingsWorkflowId },
        userSettings: { ...mapUserSettingsResponse(settingsResponse), workspaceId },
      });
      if (workspaceId !== workspaceBeforeHydration) {
        store.getState().setActiveWorkspace(workspaceId);
      }
      if (!workspaceId) {
        setResolution({ ...nextResolution, status: "ready" });
        return;
      }

      const generation = store.getState().workspaceContextGeneration;
      for (const collection of ["workflows", "repositories", "steps"] as const) {
        const requestId = generateUUID();
        requestIds.set(collection, { workspaceId, generation, requestId });
        store
          .getState()
          .setWorkspaceContextRead(
            collection,
            workspaceId,
            generation,
            "pending",
            undefined,
            requestId,
          );
      }
      const [workflowsResult, repositoriesResult, stepsResult] = await Promise.all([
        settleRouteRead(readJourneyWorkflows(store, workspaceId, { signal: controller.signal })),
        settleRouteRead(
          readJourneyRepositories(store, workspaceId, {
            signal: controller.signal,
            retryTransient: false,
          }),
        ),
        settleRouteRead(listWorkspaceWorkflowSteps(workspaceId)),
      ]);
      if (cancelled || !isCurrentWorkspaceContext(store.getState(), workspaceId, generation)) {
        return;
      }

      const currentState = store.getState();
      const readResults = [
        ["workflows", workflowsResult],
        ["repositories", repositoriesResult],
        ["steps", stepsResult],
      ] as const;
      for (const [collection, result] of readResults) {
        const requestId = requestIds.get(collection)?.requestId;
        if (result.ok) {
          currentState.setWorkspaceContextRead(
            collection,
            workspaceId,
            generation,
            "success",
            undefined,
            requestId,
          );
        } else {
          currentState.setWorkspaceContextRead(
            collection,
            workspaceId,
            generation,
            classifyWorkspaceContextReadError(result.error),
            retryAfterMilliseconds(result.error),
            requestId,
          );
        }
      }

      const workflowItems = workflowsResult.ok
        ? workflowsResult.value.workflows.map(mapWorkflowItem)
        : currentState.workflows.items.filter((workflow) => workflow.workspaceId === workspaceId);
      const activeWorkflowId = resolveDesiredWorkflowId({
        activeWorkflowId: currentState.workflows.activeId,
        settingsWorkflowId,
        workspaceWorkflows: workflowItems,
      });

      if (workflowsResult.ok) {
        store.getState().hydrate({
          workflows: { items: workflowItems, activeId: activeWorkflowId },
        });
        setRouteWorkflows(workflowsResult.value.workflows);
      }
      if (repositoriesResult.ok) {
        store.getState().setRepositories(workspaceId, repositoriesResult.value.repositories);
      }
      if (stepsResult.ok) setSteps(stepsResult.value.steps);
      setResolution({
        ...nextResolution,
        status: "ready",
        metadataLoaded: { workflows: workflowsResult.ok, repositories: repositoriesResult.ok },
      });
    }

    void bootstrap();
    return () => {
      cancelled = true;
      controller.abort();
      const state = store.getState();
      for (const [collection, request] of requestIds) {
        state.setWorkspaceContextRead(
          collection,
          request.workspaceId,
          request.generation,
          "cancelled",
          undefined,
          request.requestId,
        );
      }
      bootstrappedRef.current = false;
    };
  }, [skipBootstrap, requestedWorkspaceId, store, workspaceContextRetryVersion]);

  const currentResolution =
    resolution.requestedWorkspaceId === requestedWorkspaceId &&
    resolution.retryVersion === workspaceContextRetryVersion;
  let workspaceStatus: RouteBootstrapState["workspaceStatus"] = "loading";
  let metadataLoaded = EMPTY_TASKS_METADATA;
  if (currentResolution) workspaceStatus = resolution.status;
  if (currentResolution) metadataLoaded = resolution.metadataLoaded;
  if (skipBootstrap) workspaceStatus = "ready";
  if (skipBootstrap) metadataLoaded = READY_TASKS_METADATA;
  return {
    activeWorkspaceId,
    workflows,
    steps,
    repositories,
    workspaceStatus,
    metadataLoaded,
  };
}

type RouteReadResult<T> = { ok: true; value: T } | { ok: false; error: unknown };

async function settleRouteRead<T>(promise: Promise<T>): Promise<RouteReadResult<T>> {
  try {
    return { ok: true, value: await promise };
  } catch (error) {
    return { ok: false, error };
  }
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
