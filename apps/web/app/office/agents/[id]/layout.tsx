"use client";

import { use, type ReactNode } from "react";
import Link from "@/components/routing/app-link";
import { usePathname } from "@/lib/routing/client-router";
import { IconInfoCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfile } from "@/lib/state/slices/office/selectors";
import { cn } from "@/lib/utils";
import { useOfficeTopbar } from "../../components/office-topbar-context";
import { AgentAvatar } from "../../components/agent-avatar";
import { AgentStatusDot } from "../components/agent-status-dot";
import { AgentRoleBadge } from "../components/agent-role-badge";
import { BudgetGauge } from "../components/budget-gauge";
import { AgentRouteStrip } from "./components/agent-route-strip";
import { Trans, useTranslation } from "react-i18next";

type AgentDetailLayoutProps = {
  children: ReactNode;
  params: Promise<{ id: string }>;
};

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale.
// The `slug`s are URL segments and stay untranslated.
const TABS: Array<{ slug: string; labelKey: string }> = [
  { slug: "dashboard", labelKey: "office:dashboard" },
  { slug: "instructions", labelKey: "office:tabInstructions" },
  { slug: "skills", labelKey: "office:skills" },
  { slug: "configuration", labelKey: "office:tabConfiguration" },
  { slug: "permissions", labelKey: "office:tabPermissions" },
  { slug: "runs", labelKey: "office:runs" },
  { slug: "memory", labelKey: "office:tabMemory" },
  { slug: "channels", labelKey: "office:tabChannels" },
];

/**
 * Agent detail layout: renders the agent name into the office topbar
 * and owns the compact identity strip + tab nav. Each tab is a
 * `<Link>` to a sibling sub-route — the URL is the source of truth
 * for the active tab. Page bodies live in the matching
 * `<segment>/page.tsx`.
 */
export default function AgentDetailLayout({ children, params }: AgentDetailLayoutProps) {
  const { t } = useTranslation();
  const { id } = use(params);
  const pathname = usePathname();
  const agent = useAppStore((s) => selectOfficeAgentProfile(s, id));

  const activeSlug = activeSlugFromPath(pathname, id);

  useOfficeTopbar(
    agent
      ? {
          title: agent.name,
          icon: <AgentAvatar role={agent.role} name={agent.name} size="sm" />,
          titleSlot: (
            <span data-testid="agent-topbar-name" className="truncate text-sm font-semibold">
              {agent.name}
            </span>
          ),
          parents: [{ label: t("office:agents"), href: "/office/agents" }],
        }
      : null,
  );

  if (!agent) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">{t("office:agentNotFound")}</p>
      </div>
    );
  }

  return (
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

      <nav className="flex border-b border-border gap-1" aria-label={t("office:agentSections")}>
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
            {t(tab.labelKey)}
          </Link>
        ))}
      </nav>

      <div data-testid="agent-detail-section">{children}</div>
    </div>
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
  const { t } = useTranslation();
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
          aria-label={t("office:noScheduledWakeUpsManageRoutines")}
          className="cursor-pointer text-amber-600 dark:text-amber-400 hover:text-amber-500"
        >
          <IconInfoCircle className="h-4 w-4" />
        </Link>
      </TooltipTrigger>
      <TooltipContent className="max-w-sm">
        <div className="space-y-2">
          <p>{t("office:thisCoordinatorHasNoScheduledWake")}</p>
          <p className="font-medium">{t("office:toSetUpARoutineYou")}</p>
          <ol className="list-decimal list-inside space-y-0.5">
            <li>{t("office:aNameEGDailyStandup")}</li>
            <li>{t("office:aTaskTitleDescriptionWhatThe")}</li>
            {/*
              The cron expression is SYNTAX, not copy: it travels as a value so
              it never reaches the catalog, and the `<code>` stays an element
              child so a translator can move it within the sentence.
            */}
            <li>
              <Trans i18nKey="office:aCronScheduleExample" values={{ cron: "0 9 * * MON-FRI" }}>
                A cron schedule (e.g. <code>cron</code> for weekdays at 9am)
              </Trans>
            </li>
            <li>{t("office:thisAgentAsTheAssignee")}</li>
          </ol>
          <p className="text-muted-foreground">{t("office:clickToOpenRoutines")}</p>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
