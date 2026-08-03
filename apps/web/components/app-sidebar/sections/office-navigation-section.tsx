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
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarNavItem } from "../app-sidebar-nav-item";
import { AppSidebarSection } from "../app-sidebar-section";

type OfficeNavigationSectionProps = {
  collapsed: boolean;
  section?: "all" | "work" | "office";
};

// `labelKey`, not `label`: these tables are module scope, so a `t()` here would
// resolve once at import and freeze at the boot locale. Resolve at render below.
const workItems = [
  { icon: IconCircleDot, labelKey: "sidebar:tasks", href: "/office/tasks" },
  { icon: IconRepeat, labelKey: "sidebar:routines", href: "/office/routines" },
] as const;

const workspaceItems = [
  { icon: IconBoxMultiple, labelKey: "sidebar:skills", href: "/office/workspace/skills" },
  { icon: IconCurrencyDollar, labelKey: "sidebar:costs", href: "/office/workspace/costs" },
  { icon: IconHistory, labelKey: "sidebar:activity", href: "/office/workspace/activity" },
  { icon: IconRoute, labelKey: "sidebar:routing", href: "/office/workspace/routing" },
  { icon: IconSettings, labelKey: "sidebar:preferences", href: "/office/workspace/settings" },
] as const;

export function OfficeNavigationSection({
  collapsed,
  section = "all",
}: OfficeNavigationSectionProps) {
  const { t } = useTranslation();
  const dashboard = useAppStore((s) => s.office.dashboard);
  const taskCount = dashboard?.task_count ?? 0;
  const routineCount = dashboard?.routine_count ?? 0;
  const skillCount = dashboard?.skill_count ?? 0;

  return (
    <>
      {(section === "all" || section === "work") && (
        <AppSidebarSection
          id={APP_SIDEBAR_SECTION_IDS.officeWork}
          label={t("sidebar:work")}
          collapsed={collapsed}
          icon={IconCircleDot}
          defaultExpanded
        >
          {workItems.map((item) => (
            <AppSidebarNavItem
              key={item.href}
              icon={item.icon}
              label={t(item.labelKey)}
              href={item.href}
              badge={getWorkBadge(item.href, taskCount, routineCount)}
              collapsed={collapsed}
            />
          ))}
        </AppSidebarSection>
      )}
      {(section === "all" || section === "office") && (
        <AppSidebarSection
          id={APP_SIDEBAR_SECTION_IDS.officeWorkspace}
          label={t("sidebar:office")}
          collapsed={collapsed}
          icon={IconSettings}
          defaultExpanded
        >
          {workspaceItems.map((item) => (
            <AppSidebarNavItem
              key={item.href}
              icon={item.icon}
              label={t(item.labelKey)}
              href={item.href}
              badge={getWorkspaceBadge(item.href, skillCount)}
              badgeVariant={item.href === "/office/workspace/skills" ? "muted" : "primary"}
              collapsed={collapsed}
            />
          ))}
        </AppSidebarSection>
      )}
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
