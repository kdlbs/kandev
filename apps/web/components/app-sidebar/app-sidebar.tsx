"use client";

import { useCallback, useEffect, useRef } from "react";
import { usePathname } from "@/lib/routing/client-router";
import { useAppStore } from "@/components/state-provider";
import { useEnsureWorkspaceWorkflows } from "@/hooks/use-workflows";
import { useInOffice } from "@/hooks/use-in-office";
import { cn } from "@/lib/utils";
import {
  APP_SIDEBAR_COLLAPSED_WIDTH,
  APP_SIDEBAR_EXPANDED_WIDTH,
  APP_SIDEBAR_SECTION_IDS,
} from "./app-sidebar-constants";
import { AppSidebarFooter } from "./app-sidebar-footer";
import { AppSidebarHeader } from "./app-sidebar-header";
import { AppSidebarPrimaryNav } from "./app-sidebar-primary-nav";
import { AppSidebarResizeHandle } from "./app-sidebar-resize-handle";
import { AppSidebarSettingsMode } from "./app-sidebar-settings-mode";
import { AgentsSection } from "./sections/agents-section";
import { IntegrationsSection } from "./sections/integrations-section";
import { OfficeNavigationSection } from "./sections/office-navigation-section";
import { ProjectsSection } from "./sections/projects-section";
import { TasksSection } from "./sections/tasks-section";

const SECTION_ROUTE_MAP: Array<{ id: string; matches: (path: string) => boolean }> = [
  {
    id: APP_SIDEBAR_SECTION_IDS.tasks,
    matches: (p) => p.startsWith("/t/"),
  },
  {
    id: APP_SIDEBAR_SECTION_IDS.officeWork,
    matches: (p) => p.startsWith("/office/tasks") || p.startsWith("/office/routines"),
  },
  {
    id: APP_SIDEBAR_SECTION_IDS.officeWorkspace,
    matches: (p) => p.startsWith("/office/workspace"),
  },
  { id: APP_SIDEBAR_SECTION_IDS.projects, matches: (p) => p.startsWith("/office/projects") },
  { id: APP_SIDEBAR_SECTION_IDS.agents, matches: (p) => p.startsWith("/office/agents") },
];

function isSettingsRoute(pathname: string | null): boolean {
  return pathname === "/settings" || Boolean(pathname?.startsWith("/settings/"));
}

/**
 * Unified app sidebar mounted at the root layout. Replaces the legacy
 * WorkspaceRail + OfficeSidebar + dockview-embedded sidebar surfaces.
 *
 * Width: w-60 expanded / w-14 collapsed, smooth 300ms transition. Desktop-only
 * (`hidden md:flex`) — mobile surfaces carry their own nav (mobile headers and
 * menu sheets), so the global rail never overlays mobile content.
 */
export function AppSidebar() {
  const collapsed = useAppStore((s) => s.appSidebar.collapsed);
  const settingsMode = useAppStore((s) => s.appSidebar.settingsMode);
  const sectionExpanded = useAppStore((s) => s.appSidebar.sectionExpanded);
  const storedWidth = useAppStore((s) => s.appSidebar.width);
  const toggleSection = useAppStore((s) => s.toggleAppSidebarSection);
  const toggleCollapsed = useAppStore((s) => s.toggleAppSidebar);
  const toggleSettingsMode = useAppStore((s) => s.toggleAppSidebarSettingsMode);
  const setWidth = useAppStore((s) => s.setAppSidebarWidth);
  const pathname = usePathname();
  const inOffice = useInOffice();

  // Keep the active workspace's workflow query warm at the top of the
  // always-mounted sidebar. Hoisting this above the collapsible Tasks section
  // keeps workspace switches fresh even when that section is collapsed.
  useEnsureWorkspaceWorkflows();

  const handleResize = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      const startX = e.clientX;
      const startWidth = storedWidth;
      const maxWidth = Math.floor(window.innerWidth * 0.3);

      const onMove = (moveEvent: MouseEvent) => {
        const next = Math.min(
          maxWidth,
          Math.max(APP_SIDEBAR_EXPANDED_WIDTH, startWidth + (moveEvent.clientX - startX)),
        );
        setWidth(next);
      };
      const onUp = () => {
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };
      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    },
    [storedWidth, setWidth],
  );

  const expandedWidth = Math.max(APP_SIDEBAR_EXPANDED_WIDTH, storedWidth);

  // Keep the transient settings takeover aligned with route ownership. It is
  // intentionally not persisted, so direct reloads on `/settings/...` need to
  // enter it from the current pathname. Key on actual pathname changes so a
  // user can still close the takeover while staying on a settings page.
  const prevPathnameRef = useRef<string | null>(null);
  useEffect(() => {
    if (!pathname || prevPathnameRef.current === pathname) return;
    prevPathnameRef.current = pathname;
    if (isSettingsRoute(pathname)) {
      if (!settingsMode) toggleSettingsMode();
      return;
    }
    if (settingsMode) {
      toggleSettingsMode();
    }
  }, [pathname, settingsMode, toggleSettingsMode]);

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
        // Desktop-only: mobile uses its own per-surface nav (mobile headers +
        // menu sheets), so the global rail is hidden below md to avoid an
        // always-on overlay covering page content. `md:relative` anchors the
        // absolute resize handle.
        "h-full min-h-0 border-r border-border bg-background hidden md:flex flex-col shrink-0",
        "md:relative",
        // Animate only the width: `transition-all` makes the browser watch
        // every animatable property, and any incidental property change on the
        // aside (border, background) would also animate during open/close.
        "transition-[width] duration-300 ease-out",
      )}
      style={{
        width: collapsed ? APP_SIDEBAR_COLLAPSED_WIDTH : expandedWidth,
      }}
    >
      <AppSidebarHeader collapsed={collapsed} onToggleCollapse={toggleCollapsed} />
      <nav className="flex-1 min-h-0 flex flex-col gap-2 px-2 py-2 overflow-hidden">
        {settingsMode && !collapsed ? (
          <AppSidebarSettingsMode />
        ) : (
          <>
            <div className="shrink-0 flex flex-col gap-2 overflow-y-auto">
              <AppSidebarPrimaryNav collapsed={collapsed} />
              {inOffice && <OfficeNavigationSection collapsed={collapsed} section="work" />}
              <ProjectsSection collapsed={collapsed} />
              <AgentsSection collapsed={collapsed} />
              {inOffice && <OfficeNavigationSection collapsed={collapsed} section="office" />}
              {!inOffice && <IntegrationsSection collapsed={collapsed} />}
            </div>
            {/* In regular kanban mode, Tasks is the flex-grow middle section so
                it absorbs remaining vertical space and scrolls internally.
                Office has a dedicated /office/tasks page, so the sidebar only
                renders a lightweight Tasks nav row above. */}
            {!inOffice && <TasksSection collapsed={collapsed} />}
          </>
        )}
      </nav>
      <AppSidebarFooter collapsed={collapsed} />
      {!collapsed && <AppSidebarResizeHandle onMouseDown={handleResize} />}
    </aside>
  );
}
