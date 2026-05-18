"use client";

import { use, useCallback, useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { IconInfoCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { listAgentProfiles } from "@/lib/api/domains/office-api";
import { cn } from "@/lib/utils";
import { OfficeTopbarPortal } from "../../components/office-topbar-portal";
import { AgentAvatar } from "../../components/agent-avatar";
import { AgentStatusDot } from "../components/agent-status-dot";
import { AgentRoleBadge } from "../components/agent-role-badge";
import { BudgetGauge } from "../components/budget-gauge";
import { AgentRouteStrip } from "./components/agent-route-strip";

type AgentDetailLayoutProps = {
  children: ReactNode;
  params: Promise<{ id: string }>;
};

const TABS: Array<{ slug: string; label: string }> = [
  { slug: "dashboard", label: "Dashboard" },
  { slug: "instructions", label: "Instructions" },
  { slug: "skills", label: "Skills" },
  { slug: "configuration", label: "Configuration" },
  { slug: "permissions", label: "Permissions" },
  { slug: "runs", label: "Runs" },
  { slug: "memory", label: "Memory" },
  { slug: "channels", label: "Channels" },
];

/**
 * Agent detail layout: renders the agent name into the office topbar
 * and owns the compact identity strip + tab nav. Each tab is a
 * `<Link>` to a sibling sub-route — the URL is the source of truth
 * for the active tab. Page bodies live in the matching
 * `<segment>/page.tsx`.
 */
export default function AgentDetailLayout({ children, params }: AgentDetailLayoutProps) {
  const { id } = use(params);
  const pathname = usePathname();
  const agent = useAppStore((s) => s.office.agentProfiles.find((a) => a.id === id));
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const setOfficeAgentProfiles = useAppStore((s) => s.setOfficeAgentProfiles);

  // Refetch the agents list on mount and on WS "agents" events so this
  // layout recovers when SSR hydrated the store with a stale agent set
  // (e.g. the agent was created after the SSR fetch fired).
  const refetchAgents = useCallback(async () => {
    if (!workspaceId) return;
    const res = await listAgentProfiles(workspaceId).catch(() => ({ agents: [] }));
    setOfficeAgentProfiles(res.agents ?? []);
  }, [workspaceId, setOfficeAgentProfiles]);

  // Fire once on mount to recover from stale SSR hydration.
  useEffect(() => {
    refetchAgents();
  }, [refetchAgents]);

  useOfficeRefetch("agents", refetchAgents);

  const activeSlug = activeSlugFromPath(pathname, id);

  if (!agent) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">Agent not found.</p>
      </div>
    );
  }

  return (
    <>
      <OfficeTopbarPortal>
        <AgentAvatar role={agent.role} name={agent.name} size="sm" />
        <h1 data-testid="agent-topbar-name" className="text-sm font-semibold truncate">
          {agent.name}
        </h1>
      </OfficeTopbarPortal>

      <div className="p-6 space-y-4">
        <div
          className="flex items-center gap-3 rounded-lg border border-border bg-card px-4 py-2.5"
          data-testid="agent-identity-strip"
        >
          <AgentRoleBadge role={agent.role} />
          <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <AgentStatusDot status={agent.status} />
            {agent.status}
          </span>
          <CoordinatorRoutineHint agentId={id} agentRole={agent.role} />
          <div className="ml-auto">
            <BudgetGauge budgetCents={agent.budgetMonthlyCents} />
          </div>
        </div>

        <AgentRouteStrip agentId={id} />

        <nav className="flex border-b border-border gap-1" aria-label="Agent sections">
          {TABS.map((tab) => (
            <Link
              key={tab.slug}
              href={`/office/agents/${id}/${tab.slug}`}
              data-testid={`agent-tab-${tab.slug}`}
              className={cn(
                "px-3 py-2 text-sm cursor-pointer border-b-2 -mb-px transition-colors",
                activeSlug === tab.slug
                  ? "border-foreground text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              {tab.label}
            </Link>
          ))}
        </nav>

        <div data-testid="agent-detail-section">{children}</div>
      </div>
    </>
  );
}

/**
 * Pull the active tab slug from the URL. Examples:
 *   /office/agents/abc/dashboard            → "dashboard"
 *   /office/agents/abc/runs/run-123         → "runs"
 *   /office/agents/abc                      → "dashboard" (default)
 */
function activeSlugFromPath(pathname: string | null, agentId: string): string {
  if (!pathname) return "dashboard";
  const prefix = `/office/agents/${agentId}/`;
  if (!pathname.startsWith(prefix)) return "dashboard";
  const rest = pathname.slice(prefix.length);
  const slug = rest.split("/")[0];
  return slug || "dashboard";
}

/**
 * Inline hint shown next to the status of a CEO/coordinator that has no
 * active routine targeting them. Hovering reveals an explanation; clicking
 * navigates to /office/routines to install one. Workers / specialists
 * don't get this hint since they only run on assignment, not schedule.
 */
function CoordinatorRoutineHint({ agentId, agentRole }: { agentId: string; agentRole: string }) {
  const routines = useAppStore((s) => s.office.routines);
  if (agentRole !== "ceo") return null;
  const hasActive = routines.some(
    (r) => r.assigneeAgentProfileId === agentId && r.status === "active",
  );
  if (hasActive) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Link
          href="/office/routines"
          aria-label="No scheduled wake-ups — manage routines"
          className="cursor-pointer text-amber-600 dark:text-amber-400 hover:text-amber-500"
        >
          <IconInfoCircle className="h-4 w-4" />
        </Link>
      </TooltipTrigger>
      <TooltipContent className="max-w-sm">
        <div className="space-y-2">
          <p>
            This coordinator has no scheduled wake-ups — it only fires on comments, errors, or
            manual triggers.
          </p>
          <p className="font-medium">To set up a routine you&apos;ll need:</p>
          <ol className="list-decimal list-inside space-y-0.5">
            <li>A name (e.g. &quot;Daily standup&quot;)</li>
            <li>A task title + description (what the agent should do each run)</li>
            <li>
              A cron schedule (e.g. <code>0 9 * * MON-FRI</code> for weekdays at 9am)
            </li>
            <li>This agent as the assignee</li>
          </ol>
          <p className="text-muted-foreground">Click to open Routines.</p>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
