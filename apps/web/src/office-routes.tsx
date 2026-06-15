import ProjectDetailPage from "@/app/office/projects/[id]/page";
import { AgentsPageClient } from "@/app/office/agents/agents-page-client";
import { OfficeTopbar } from "@/app/office/components/office-topbar";
import { InboxPageClient } from "@/app/office/inbox/inbox-page-client";
import { OfficePageClient } from "@/app/office/page-client";
import { ProjectsPageClient } from "@/app/office/projects/projects-page-client";
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
import { useAppStore } from "@/components/state-provider";
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
  const officeEnabled = useAppStore((state) => state.features.office);

  if (!officeEnabled) {
    return <OfficeUnavailable />;
  }

  return (
    <TooltipProvider>
      <div className="flex h-full min-h-0 flex-col">
        <OfficeTopbar />
        <main className="flex-1 min-h-0 overflow-y-auto">
          {renderOfficeRoute(normalizeOfficePath(pathname))}
        </main>
      </div>
    </TooltipProvider>
  );
}

export function officeRouteKey(pathname: string): string {
  return normalizeOfficePath(pathname);
}

function renderOfficeRoute(pathname: string) {
  const projectId = matchSingle(pathname, /^\/office\/projects\/([^/]+)$/);
  if (projectId) {
    return <ProjectDetailPage params={Promise.resolve({ id: projectId })} />;
  }

  const taskId = matchSingle(pathname, /^\/office\/tasks\/([^/]+)$/);
  if (taskId) {
    return <IssueDetailPage params={Promise.resolve({ id: taskId })} />;
  }

  return OFFICE_ROUTES[pathname]?.() ?? <OfficeRouteFallback pathname={pathname} />;
}

function OfficeUnavailable() {
  return (
    <div className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground">
      Office is not enabled for this runtime.
    </div>
  );
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
