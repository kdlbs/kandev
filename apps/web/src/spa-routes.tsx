import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { GitHubPageClient } from "@/app/github/github-page-client";
import { GitLabPageClient } from "@/app/gitlab/gitlab-page-client";
import { JiraPageClient } from "@/app/jira/jira-page-client";
import { LinearPageClient } from "@/app/linear/linear-page-client";
import { PageClient } from "@/app/page-client";
import { StatsPageClient } from "@/app/stats/stats-page-client";
import { isRangeKey } from "@/app/stats/stats-utils";
import type { RangeKey } from "@/app/stats/stats-utils";
import { TasksPageClient } from "@/app/tasks/tasks-page-client";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { BootRouteData } from "./boot-payload";
import { fetchJson } from "@/lib/api/client";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { listRepositories, listWorkspaces } from "@/lib/api/domains/workspace-api";
import { resolveDesiredWorkflowId } from "@/lib/kanban/resolve-workflow";
import { hasHydratedKanbanRouteState } from "@/lib/routing/kanban-route-hydration";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import { mapWorkspaceItem, readActiveWorkspaceCookie } from "@/lib/routing/route-bootstrap";
import { resolveActiveId } from "@/lib/ssr/resolve-active-id";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type {
  ListWorkflowStepsResponse,
  Repository,
  Workflow,
  WorkflowStep,
} from "@/lib/types/http";
import { TaskDetailRoute } from "./task-detail-route";

const OfficeRoutes = lazy(() =>
  import("./office-routes").then((mod) => ({ default: mod.OfficeRoutes })),
);
const SettingsRoutes = lazy(() =>
  import("./settings-routes").then((mod) => ({ default: mod.SettingsRoutes })),
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
  | { kind: "github" }
  | { kind: "gitlab" }
  | { kind: "jira" }
  | { kind: "linear" }
  | { kind: "stats"; range?: RangeKey }
  | { kind: "settings"; pathname: string }
  | { kind: "office"; pathname: string };

type DataBackedSpaRoute = Exclude<SpaRoute, { kind: "kanban" | "settings" | "office" }>;

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
    resolveTopLevelRoute(normalized, searchParams) ??
    resolveNestedRoute(normalized) ??
    resolveKanbanRoute(searchParams)
  );
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
    case "/github":
      return { kind: "github" };
    case "/gitlab":
      return { kind: "gitlab" };
    case "/jira":
      return { kind: "jira" };
    case "/linear":
      return { kind: "linear" };
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
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const route = resolveSpaRoute(pathname, searchParams);

  if (route.kind === "kanban") {
    return <KanbanRoute route={route} />;
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
      <Suspense fallback={null}>
        <SettingsRoutes pathname={route.pathname} />
      </Suspense>
    );
  }
  if (route.kind === "office") {
    return (
      <Suspense fallback={null}>
        <OfficeRoutes pathname={route.pathname} />
      </Suspense>
    );
  }

  return <DataBackedRoute route={route} routeData={routeData} />;
}

function KanbanRoute({ route }: { route: Extract<SpaRoute, { kind: "kanban" }> }) {
  useKanbanRouteBootstrap(route);
  return <PageClient initialTaskId={route.taskId} initialSessionId={route.sessionId} />;
}

function useKanbanRouteBootstrap(route: Extract<SpaRoute, { kind: "kanban" }>) {
  const store = useAppStoreApi();

  useEffect(() => {
    if (hasHydratedKanbanRouteState(store.getState(), route)) return;

    let cancelled = false;

    async function bootstrap() {
      const [workspacesResponse, settingsResponse] = await Promise.all([
        listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [], total: 0 })),
        fetchUserSettings({ cache: "no-store" }).catch(() => null),
      ]);
      if (cancelled) return;

      const settingsWorkspaceId = settingsResponse?.settings?.workspace_id || null;
      const settingsWorkflowId = settingsResponse?.settings?.workflow_filter_id || null;
      const workspaceItems = workspacesResponse.workspaces.map(mapWorkspaceItem);
      const activeWorkspaceId = resolveActiveId(
        workspaceItems,
        route.workspaceId,
        readActiveWorkspaceCookie(),
        settingsWorkspaceId,
      );

      store.getState().hydrate({
        workspaces: { items: workspaceItems, activeId: activeWorkspaceId },
        userSettings: {
          ...mapUserSettingsResponse(settingsResponse),
          workspaceId: activeWorkspaceId,
        },
      });

      if (!activeWorkspaceId) return;

      const [workflowsResponse, repositoriesResponse] = await Promise.all([
        listWorkflows(activeWorkspaceId, { cache: "no-store", includeHidden: true }).catch(() => ({
          workflows: [],
        })),
        listRepositories(activeWorkspaceId, undefined, { cache: "no-store" }).catch(() => ({
          repositories: [],
        })),
      ]);
      if (cancelled) return;

      const workflowId = resolveDesiredWorkflowId({
        activeWorkflowId: route.workflowId ?? null,
        settingsWorkflowId,
        workspaceWorkflows: workflowsResponse.workflows,
      });

      store.getState().hydrate({
        userSettings: {
          ...mapUserSettingsResponse(settingsResponse),
          workspaceId: activeWorkspaceId,
          workflowId,
        },
        workflows: {
          items: workflowsResponse.workflows.map(mapWorkflowItem),
          activeId: workflowId,
        },
      });
      store.getState().setRepositories(activeWorkspaceId, repositoriesResponse.repositories);
    }

    void bootstrap();
    return () => {
      cancelled = true;
    };
  }, [route.workspaceId, route.workflowId, store]);
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
  return (
    <TasksPageClient
      workspaces={[]}
      initialWorkspaceId={
        tasksPage?.activeWorkspaceId ?? bootstrapped.activeWorkspaceId ?? undefined
      }
      initialWorkflows={tasksPage?.workflows ?? bootstrapped.workflows}
      initialSteps={tasksPage?.steps ?? bootstrapped.steps}
      initialRepositories={tasksPage?.repositories ?? bootstrapped.repositories}
      initialTasks={tasksPage?.tasks ?? []}
      initialTotal={tasksPage?.total ?? 0}
      initialDataLoaded={Boolean(tasksPage)}
    />
  );
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
      return <GitLabPageClient workspaceId={workspaceId} />;
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
    if (skipBootstrap) return;
    bootstrappedRef.current = true;
    let cancelled = false;

    async function bootstrap() {
      const [workspacesResponse, settingsResponse] = await Promise.all([
        listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
        fetchUserSettings({ cache: "no-store" }).catch(() => null),
      ]);
      if (cancelled) return;

      const workspaces = workspacesResponse.workspaces;
      const workspaceId = settingsResponse?.settings?.workspace_id || workspaces[0]?.id || null;
      const settingsWorkflowId = settingsResponse?.settings?.workflow_filter_id || null;
      store.getState().hydrate({
        workspaces: { items: workspaces, activeId: workspaceId },
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
