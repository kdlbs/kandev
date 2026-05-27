"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import {
  APP_SIDEBAR_COLLAPSED_WIDTH,
  APP_SIDEBAR_EXPANDED_WIDTH,
  APP_SIDEBAR_SECTION_IDS,
} from "./app-sidebar-constants";
import { AppSidebarFooter } from "./app-sidebar-footer";
import { AppSidebarHeader } from "./app-sidebar-header";
import { AppSidebarPrimaryNav } from "./app-sidebar-primary-nav";
import { AgentsSection } from "./sections/agents-section";
import { IntegrationsSection } from "./sections/integrations-section";
import { ProjectsSection } from "./sections/projects-section";
import { SettingsSection } from "./sections/settings-section";
import { TasksSection } from "./sections/tasks-section";

const SECTION_ROUTE_MAP: Array<{ id: string; matches: (path: string) => boolean }> = [
  {
    id: APP_SIDEBAR_SECTION_IDS.tasks,
    matches: (p) => p.startsWith("/office/tasks") || p.startsWith("/task/"),
  },
  { id: APP_SIDEBAR_SECTION_IDS.projects, matches: (p) => p.startsWith("/office/projects") },
  { id: APP_SIDEBAR_SECTION_IDS.agents, matches: (p) => p.startsWith("/office/agents") },
  { id: APP_SIDEBAR_SECTION_IDS.settings, matches: (p) => p.startsWith("/settings") },
];

/**
 * Unified app sidebar mounted at the root layout. Replaces the legacy
 * WorkspaceRail + OfficeSidebar + dockview-embedded sidebar surfaces.
 *
 * Width: w-60 expanded / w-14 collapsed, smooth 300ms transition. On mobile
 * (`absolute md:relative`) it overlays content instead of pushing it.
 */
export function AppSidebar() {
  const collapsed = useAppStore((s) => s.appSidebar.collapsed);
  const sectionExpanded = useAppStore((s) => s.appSidebar.sectionExpanded);
  const toggleSection = useAppStore((s) => s.toggleAppSidebarSection);
  const toggleCollapsed = useAppStore((s) => s.toggleAppSidebar);
  const pathname = usePathname();

  useEffect(() => {
    if (!pathname) return;
    for (const entry of SECTION_ROUTE_MAP) {
      if (entry.matches(pathname) && !sectionExpanded[entry.id]) {
        toggleSection(entry.id);
      }
    }
    // Intentionally depend only on the pathname so user-collapses aren't
    // immediately re-expanded by section state churn.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  return (
    <aside
      data-testid="app-sidebar"
      data-collapsed={collapsed ? "true" : "false"}
      className={cn(
        "h-full min-h-0 border-r border-border bg-background flex flex-col shrink-0",
        "absolute md:relative left-0 top-0 z-30",
        "transition-all duration-300 ease-out",
      )}
      style={{
        width: collapsed ? APP_SIDEBAR_COLLAPSED_WIDTH : APP_SIDEBAR_EXPANDED_WIDTH,
      }}
    >
      <AppSidebarHeader collapsed={collapsed} onToggleCollapse={toggleCollapsed} />
      <nav className="flex-1 min-h-0 flex flex-col gap-2 px-2 py-2 overflow-hidden">
        <div className="shrink-0 flex flex-col gap-2 overflow-y-auto">
          <AppSidebarPrimaryNav collapsed={collapsed} />
          <ProjectsSection collapsed={collapsed} />
          <AgentsSection collapsed={collapsed} />
          <IntegrationsSection collapsed={collapsed} />
          <SettingsSection collapsed={collapsed} />
        </div>
        {/* Kanban is the bottom-most flex-grow section so it absorbs all
            remaining vertical space and scrolls internally. */}
        <TasksSection collapsed={collapsed} />
      </nav>
      <AppSidebarFooter collapsed={collapsed} />
    </aside>
  );
}
