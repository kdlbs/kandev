"use client";

import {
  IconBoxMultiple,
  IconCircleDot,
  IconCurrencyDollar,
  IconHistory,
  IconRepeat,
  IconRoute,
  IconSettings,
  IconSitemap,
} from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarNavItem } from "../app-sidebar-nav-item";
import { AppSidebarSection } from "../app-sidebar-section";

type OfficeNavigationSectionProps = {
  collapsed: boolean;
};

const workItems = [
  { icon: IconCircleDot, label: "Tasks", href: "/office/tasks" },
  { icon: IconRepeat, label: "Routines", href: "/office/routines" },
] as const;

const workspaceItems = [
  { icon: IconSitemap, label: "Agent Topology", href: "/office/workspace/org" },
  { icon: IconBoxMultiple, label: "Skills", href: "/office/workspace/skills" },
  { icon: IconCurrencyDollar, label: "Costs", href: "/office/workspace/costs" },
  { icon: IconHistory, label: "Activity", href: "/office/workspace/activity" },
  { icon: IconRoute, label: "Routing", href: "/office/workspace/routing" },
  { icon: IconSettings, label: "Office Settings", href: "/office/workspace/settings" },
] as const;

export function OfficeNavigationSection({ collapsed }: OfficeNavigationSectionProps) {
  const dashboard = useAppStore((s) => s.office.dashboard);
  const taskCount = dashboard?.task_count ?? 0;
  const routineCount = dashboard?.routine_count ?? 0;
  const skillCount = dashboard?.skill_count ?? 0;

  return (
    <>
      <AppSidebarSection
        id={APP_SIDEBAR_SECTION_IDS.officeWork}
        label="Work"
        collapsed={collapsed}
        icon={IconCircleDot}
        defaultExpanded
      >
        {workItems.map((item) => (
          <AppSidebarNavItem
            key={item.href}
            icon={item.icon}
            label={item.label}
            href={item.href}
            badge={getWorkBadge(item.label, taskCount, routineCount)}
            collapsed={collapsed}
          />
        ))}
      </AppSidebarSection>
      <AppSidebarSection
        id={APP_SIDEBAR_SECTION_IDS.officeWorkspace}
        label="Workspace"
        collapsed={collapsed}
        icon={IconSitemap}
        defaultExpanded
      >
        {workspaceItems.map((item) => (
          <AppSidebarNavItem
            key={item.href}
            icon={item.icon}
            label={item.label}
            href={item.href}
            badge={item.label === "Skills" && skillCount > 0 ? skillCount : undefined}
            collapsed={collapsed}
          />
        ))}
      </AppSidebarSection>
    </>
  );
}

function getWorkBadge(label: string, taskCount: number, routineCount: number): number | undefined {
  if (label === "Tasks" && taskCount > 0) return taskCount;
  if (label === "Routines" && routineCount > 0) return routineCount;
  return undefined;
}
