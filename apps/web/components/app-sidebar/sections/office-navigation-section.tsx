"use client";

import {
  IconBoxMultiple,
  IconCircleDot,
  IconCurrencyDollar,
  IconHistory,
  IconRepeat,
  IconRoute,
  IconSettings,
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
  { icon: IconBoxMultiple, label: "Skills", href: "/office/workspace/skills" },
  { icon: IconCurrencyDollar, label: "Costs", href: "/office/workspace/costs" },
  { icon: IconHistory, label: "Activity", href: "/office/workspace/activity" },
  { icon: IconRoute, label: "Routing", href: "/office/workspace/routing" },
  { icon: IconSettings, label: "Preferences", href: "/office/workspace/settings" },
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
            badge={getWorkBadge(item.href, taskCount, routineCount)}
            collapsed={collapsed}
          />
        ))}
      </AppSidebarSection>
      <AppSidebarSection
        id={APP_SIDEBAR_SECTION_IDS.officeWorkspace}
        label="Office"
        collapsed={collapsed}
        icon={IconSettings}
        defaultExpanded
      >
        {workspaceItems.map((item) => (
          <AppSidebarNavItem
            key={item.href}
            icon={item.icon}
            label={item.label}
            href={item.href}
            badge={getWorkspaceBadge(item.href, skillCount)}
            collapsed={collapsed}
          />
        ))}
      </AppSidebarSection>
    </>
  );
}

function getWorkBadge(
  href: (typeof workItems)[number]["href"],
  taskCount: number,
  routineCount: number,
): number | undefined {
  if (href === "/office/tasks" && taskCount > 0) return taskCount;
  if (href === "/office/routines" && routineCount > 0) return routineCount;
  return undefined;
}

function getWorkspaceBadge(
  href: (typeof workspaceItems)[number]["href"],
  skillCount: number,
): number | undefined {
  if (href === "/office/workspace/skills" && skillCount > 0) return skillCount;
  return undefined;
}
