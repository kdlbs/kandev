import { GitHubPageClient } from "@/app/github/github-page-client";
import { GitLabPageClient } from "@/app/gitlab/gitlab-page-client";
import { JiraPageClient } from "@/app/jira/jira-page-client";
import { LinearPageClient } from "@/app/linear/linear-page-client";
import { PageClient } from "@/app/page-client";
import { StatsPageClient } from "@/app/stats/stats-page-client";
import { isRangeKey } from "@/app/stats/stats-utils";
import type { RangeKey } from "@/app/stats/stats-utils";
import { TasksPageClient } from "@/app/tasks/tasks-page-client";
import { useAppStore } from "@/components/state-provider";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import type { Repository } from "@/lib/types/http";

type SpaRoute =
  | { kind: "kanban"; taskId?: string; sessionId?: string }
  | { kind: "tasks" }
  | { kind: "github" }
  | { kind: "gitlab" }
  | { kind: "jira" }
  | { kind: "linear" }
  | { kind: "stats"; range?: RangeKey };

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
      return { kind: "kanban" };
  }
}

export function SpaRoutes() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const route = resolveSpaRoute(pathname, searchParams);
  const { activeWorkspaceId, repositories } = useRouteData();

  switch (route.kind) {
    case "tasks":
      return (
        <TasksPageClient
          workspaces={[]}
          initialWorkspaceId={activeWorkspaceId ?? undefined}
          initialWorkflows={[]}
          initialSteps={[]}
          initialRepositories={repositories}
          initialTasks={[]}
          initialTotal={0}
        />
      );
    case "github":
      return (
        <GitHubPageClient
          workspaceId={activeWorkspaceId ?? undefined}
          workflows={[]}
          steps={[]}
          repositories={repositories}
        />
      );
    case "gitlab":
      return <GitLabPageClient workspaceId={activeWorkspaceId ?? undefined} />;
    case "jira":
      return (
        <JiraPageClient workspaceId={activeWorkspaceId ?? undefined} workflows={[]} steps={[]} />
      );
    case "linear":
      return (
        <LinearPageClient workspaceId={activeWorkspaceId ?? undefined} workflows={[]} steps={[]} />
      );
    case "stats":
      return (
        <StatsPageClient
          workspaceId={activeWorkspaceId ?? undefined}
          activeRange={route.range}
          initialError={null}
        />
      );
    case "kanban":
      return <PageClient initialTaskId={route.taskId} initialSessionId={route.sessionId} />;
  }
}

function useRouteData(): {
  activeWorkspaceId: string | null;
  repositories: Repository[];
} {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const repositories = useAppStore((state) =>
    activeWorkspaceId ? (state.repositories.itemsByWorkspaceId[activeWorkspaceId] ?? []) : [],
  );

  return { activeWorkspaceId, repositories };
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
