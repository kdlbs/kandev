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
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type {
  ListWorkflowStepsResponse,
  Repository,
  Workflow,
  WorkflowStep,
} from "@/lib/types/http";

const OfficeRoutes = lazy(() =>
  import("./office-routes").then((mod) => ({ default: mod.OfficeRoutes })),
);
const SettingsRoutes = lazy(() =>
  import("./settings-routes").then((mod) => ({ default: mod.SettingsRoutes })),
);

const EMPTY_REPOSITORIES: Repository[] = [];

type SpaRoute =
  | { kind: "kanban"; taskId?: string; sessionId?: string }
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
  const taskId = readTaskId(normalized);

  if (taskId) {
    return {
      kind: "kanban",
      taskId,
      sessionId: searchParams.get("sessionId") ?? undefined,
    };
  }

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
      if (normalized === "/settings" || normalized.startsWith("/settings/")) {
        return { kind: "settings", pathname: normalized };
      }
      if (normalized === "/office" || normalized.startsWith("/office/")) {
        return { kind: "office", pathname: normalized };
      }
      return { kind: "kanban" };
  }
}

export function SpaRoutes() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const route = resolveSpaRoute(pathname, searchParams);

  if (route.kind === "kanban") {
    return <PageClient initialTaskId={route.taskId} initialSessionId={route.sessionId} />;
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
    };
  }, [store]);

  return { activeWorkspaceId, workflows, steps, repositories };
}

function listWorkspaceWorkflowSteps(workspaceId: string) {
  return fetchJson<ListWorkflowStepsResponse>(`/api/v1/workspaces/${workspaceId}/workflow-steps`, {
    cache: "no-store",
  });
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
