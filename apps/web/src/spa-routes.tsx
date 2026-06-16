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
import { fetchJson } from "@/lib/api/client";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { listRepositories, listWorkspaces } from "@/lib/api/domains/workspace-api";
import { resolveDesiredWorkflowId } from "@/lib/kanban/resolve-workflow";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import { resolveActiveId } from "@/lib/ssr/resolve-active-id";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type {
  ListWorkflowStepsResponse,
  ListWorkspacesResponse,
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

export function SpaRoutes() {
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

  return <DataBackedRoute route={route} />;
}

function KanbanRoute({ route }: { route: Extract<SpaRoute, { kind: "kanban" }> }) {
  useKanbanRouteBootstrap(route);
  return <PageClient initialTaskId={route.taskId} initialSessionId={route.sessionId} />;
}

function useKanbanRouteBootstrap(route: Extract<SpaRoute, { kind: "kanban" }>) {
  const store = useAppStoreApi();

  useEffect(() => {
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
        readCookie("office-active-workspace"),
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
}: {
  route: Exclude<SpaRoute, { kind: "kanban" | "settings" | "office" }>;
}) {
  const { activeWorkspaceId, workflows, steps, repositories } = useRouteData();
  switch (route.kind) {
    case "tasks":
      return (
        <TasksPageClient
          workspaces={[]}
          initialWorkspaceId={activeWorkspaceId ?? undefined}
          initialWorkflows={workflows}
          initialSteps={steps}
          initialRepositories={repositories}
          initialTasks={[]}
          initialTotal={0}
        />
      );
    case "github":
      return (
        <GitHubPageClient
          workspaceId={activeWorkspaceId ?? undefined}
          workflows={workflows}
          steps={steps}
          repositories={repositories}
        />
      );
    case "gitlab":
      return <GitLabPageClient workspaceId={activeWorkspaceId ?? undefined} />;
    case "jira":
      return (
        <JiraPageClient
          workspaceId={activeWorkspaceId ?? undefined}
          workflows={workflows}
          steps={steps}
        />
      );
    case "linear":
      return (
        <LinearPageClient
          workspaceId={activeWorkspaceId ?? undefined}
          workflows={workflows}
          steps={steps}
        />
      );
    case "stats":
      return (
        <StatsPageClient
          workspaceId={activeWorkspaceId ?? undefined}
          activeRange={route.range}
          initialError={null}
        />
      );
  }
}

function useRouteData(): {
  activeWorkspaceId: string | null;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
} {
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
      store.getState().hydrate({
        workspaces: { items: workspaces, activeId: workspaceId },
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

      store.getState().setWorkflows(
        workflowsResponse.workflows.map((workflow) => ({
          id: workflow.id,
          workspaceId: workflow.workspace_id,
          name: workflow.name,
          description: workflow.description ?? null,
          sortOrder: workflow.sort_order ?? 0,
          ...(workflow.agent_profile_id ? { agent_profile_id: workflow.agent_profile_id } : {}),
          ...(workflow.hidden !== undefined ? { hidden: workflow.hidden } : {}),
        })),
      );
      store.getState().setRepositories(workspaceId, repositoriesResponse.repositories);
      setRouteWorkflows(workflowsResponse.workflows);
      setSteps(stepsResponse.steps);
    }

    void bootstrap();
    return () => {
      cancelled = true;
      bootstrappedRef.current = false;
    };
  }, [store]);

  return { activeWorkspaceId, workflows, steps, repositories };
}

function listWorkspaceWorkflowSteps(workspaceId: string) {
  return fetchJson<ListWorkflowStepsResponse>(`/api/v1/workspaces/${workspaceId}/workflow-steps`, {
    cache: "no-store",
  });
}

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];

function mapWorkspaceItem(ws: WorkspaceItem) {
  return {
    id: ws.id,
    name: ws.name,
    description: ws.description ?? null,
    owner_id: ws.owner_id,
    default_executor_id: ws.default_executor_id ?? null,
    default_environment_id: ws.default_environment_id ?? null,
    default_agent_profile_id: ws.default_agent_profile_id ?? null,
    default_config_agent_profile_id: ws.default_config_agent_profile_id ?? null,
    office_workflow_id: ws.office_workflow_id ?? null,
    created_at: ws.created_at,
    updated_at: ws.updated_at,
  };
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

function readCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const encodedName = `${encodeURIComponent(name)}=`;
  const entry = document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(encodedName));
  return entry ? decodeURIComponent(entry.slice(encodedName.length)) : null;
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
