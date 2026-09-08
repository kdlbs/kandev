"use client";

import { useTranslation } from "react-i18next";
import { IconBolt, IconListDetails, IconLoader2 } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import {
  STATE_DOT_CLASS,
  STATE_LABEL_KEY,
  buildAutomationRows,
} from "@/components/runs/automation-rows";
import type { AutomationRow } from "@/components/runs/automation-rows";
import { useAutomationSummaries } from "@/components/runs/use-automation-summaries";
import { useLiveRefresh } from "@/components/runs/use-live-refresh";
import { useWorkspaceAutomations } from "@/components/runs/use-workspace-automations";
import { AUTOMATIONS_HREF } from "@/components/runs/runs-view";
import { usePathname } from "@/lib/routing/client-router";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useNow } from "@/hooks/use-now";
import { cn, formatRelativeTime } from "@/lib/utils";
import {
  APP_SIDEBAR_SECTION_IDS,
  SIDEBAR_ITEM_ACTIVE,
  SIDEBAR_ITEM_INACTIVE,
} from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

const NEW_AUTOMATION_HREF = "/settings/automations";

/**
 * The full list, reachable from the section header. These rows carry a name and
 * a dot; the list page adds the next firing and what each one last said, which
 * is more than a sidebar row should try to hold.
 */
function OpenListShortcut() {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Link
          href={AUTOMATIONS_HREF}
          aria-label={t("automations:openAutomations")}
          data-testid="automations-all-runs"
          className="flex h-5 w-5 items-center justify-center rounded text-muted-foreground/70 hover:bg-muted/60 hover:text-foreground cursor-pointer transition-colors"
        >
          <IconListDetails className="h-3.5 w-3.5" />
        </Link>
      </TooltipTrigger>
      <TooltipContent side="right">{t("automations:openAutomations")}</TooltipContent>
    </Tooltip>
  );
}

/**
 * One automation. The dot answers "is this thing okay" at a glance — the same
 * question the runs list answers, from the same derivation, so the sidebar and
 * the list cannot disagree about an automation's health.
 *
 * The trailing time answers the other one: an automation is a thing that is
 * supposed to keep happening, and "when did it last happen" is what tells you
 * whether it is. It comes from the same `formatRelativeTime` the runs rail
 * prints beside each run, so the two surfaces phrase an age identically.
 */
function AutomationRowLink({ row, active }: { row: AutomationRow; active: boolean }) {
  const { automation, state, lastRun } = row;
  const { t } = useTranslation();
  // "4m ago" is the one thing here the websocket pipeline will never push an
  // update for: nothing about the automation changed, the clock did. An idle
  // sidebar re-renders only when its data does, so without a tick this label
  // freezes at whatever it said when the section was first opened — and a
  // stale age is worse than no age, because it reads as a fresh reading.
  useNow(30_000);
  return (
    <Link
      href={`${AUTOMATIONS_HREF}/${automation.id}`}
      data-testid={`sidebar-automation-${automation.id}`}
      className={cn(
        "flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
        active ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE,
      )}
    >
      {state === "running" ? (
        <IconLoader2
          className="h-3.5 w-3.5 shrink-0 animate-spin text-blue-500"
          aria-hidden="true"
          data-testid={`sidebar-automation-running-${automation.id}`}
        />
      ) : (
        <span
          className={cn("h-1.5 w-1.5 shrink-0 rounded-full", STATE_DOT_CLASS[state])}
          aria-hidden="true"
        />
      )}
      {/* The dot is decorative, so the state reaches a screen reader here. */}
      <span className="sr-only">{`${t(STATE_LABEL_KEY[state])}.`}</span>
      <span className="min-w-0 flex-1 truncate">{automation.name}</span>
      {lastRun && (
        <span
          className="shrink-0 text-[11px] font-normal tabular-nums text-muted-foreground/70"
          data-testid={`sidebar-automation-last-run-${automation.id}`}
          title={t("automations:lastRan", { when: formatRelativeTime(lastRun.created_at) })}
        >
          {formatRelativeTime(lastRun.created_at)}
        </span>
      )}
    </Link>
  );
}

function EmptyRow() {
  const { t } = useTranslation();
  return (
    <Link
      href={NEW_AUTOMATION_HREF}
      data-testid="sidebar-automations-empty"
      className={cn(
        "px-2.5 py-1.5 text-[13px] rounded-md cursor-pointer",
        "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
      )}
    >
      {t("automations:setUpAnAutomation")}
    </Link>
  );
}

export function AutomationsSection({ collapsed }: { collapsed: boolean }) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { isMobile } = useResponsiveBreakpoint();
  // Folded until asked for. Automations are a background concern — they run
  // whether or not anyone is looking — so they should not push the tasks
  // someone came here to work on off the bottom of the rail.
  const expanded = useAppStore(
    (s) => s.appSidebar.sectionExpanded[APP_SIDEBAR_SECTION_IDS.automations] ?? false,
  );

  // The sidebar is mounted on every page, so the fetches are gated on what is
  // actually being shown. The names list is cheap and a folded section still
  // has to say how many it is hiding — a section that starts shut and shows
  // nothing reads as empty, and nobody opens it. Health summaries are the
  // heavier read and buy nothing until the rows themselves are on screen.
  const showing = !collapsed && expanded && !isMobile;
  const listScope = collapsed ? undefined : (workspaceId ?? undefined);
  const summaryScope = showing ? (workspaceId ?? undefined) : undefined;
  const { automations } = useWorkspaceAutomations(listScope);
  const { summaries, refresh } = useAutomationSummaries(summaryScope);
  useLiveRefresh(showing, refresh);
  const rows = buildAutomationRows(automations, summaries);

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.automations}
      label={t("automations:automations")}
      collapsed={collapsed}
      icon={IconBolt}
      headerAction={<OpenListShortcut />}
      headerActionVisibility="always"
      defaultExpanded={false}
      collapsedSummary={rows.length > 0 ? rows.length : undefined}
    >
      {rows.length === 0 ? (
        <EmptyRow />
      ) : (
        rows.map((row) => (
          <AutomationRowLink
            key={row.automation.id}
            row={row}
            active={pathname === `${AUTOMATIONS_HREF}/${row.automation.id}`}
          />
        ))
      )}
    </AppSidebarSection>
  );
}
