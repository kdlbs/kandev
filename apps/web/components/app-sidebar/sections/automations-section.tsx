"use client";

import { useTranslation } from "react-i18next";
import { IconBolt, IconListDetails } from "@tabler/icons-react";
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
import { useWorkspaceAutomations } from "@/components/runs/use-workspace-automations";
import { AUTOMATIONS_HREF } from "@/components/runs/runs-view";
import { usePathname } from "@/lib/routing/client-router";
import { cn } from "@/lib/utils";
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
 */
function AutomationRowLink({ row, active }: { row: AutomationRow; active: boolean }) {
  const { automation, state } = row;
  const { t } = useTranslation();
  return (
    <Link
      href={`${AUTOMATIONS_HREF}/${automation.id}`}
      data-testid={`sidebar-automation-${automation.id}`}
      className={cn(
        "flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
        active ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE,
      )}
    >
      <span
        className={cn("h-1.5 w-1.5 shrink-0 rounded-full", STATE_DOT_CLASS[state])}
        aria-hidden="true"
      />
      {/* The dot is decorative, so the state reaches a screen reader here. */}
      <span className="sr-only">{`${t(STATE_LABEL_KEY[state])}.`}</span>
      <span className="flex-1 truncate">{automation.name}</span>
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
  const expanded = useAppStore(
    (s) => s.appSidebar.sectionExpanded[APP_SIDEBAR_SECTION_IDS.automations] ?? true,
  );

  // The sidebar is mounted on every page, so the fetches are gated on the list
  // actually being on screen. A collapsed rail or a folded section asks for
  // nothing.
  const showing = !collapsed && expanded;
  const scope = showing ? (workspaceId ?? undefined) : undefined;
  const { automations } = useWorkspaceAutomations(scope);
  const { summaries } = useAutomationSummaries(scope);
  const rows = buildAutomationRows(automations, summaries);

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.automations}
      label={t("automations:automations")}
      collapsed={collapsed}
      icon={IconBolt}
      headerAction={<OpenListShortcut />}
      headerActionVisibility="always"
      defaultExpanded
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
