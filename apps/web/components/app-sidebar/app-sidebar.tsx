"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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
import { PluginNavItems } from "@/components/plugins/plugin-nav-items";
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

type AppSidebarNavigationProps = {
  collapsed: boolean;
  inOffice: boolean;
  settingsMode: boolean;
};

function AppSidebarNavigation({ collapsed, inOffice, settingsMode }: AppSidebarNavigationProps) {
  return (
    <nav className="relative flex-1 min-h-0 flex flex-col gap-2 px-2 py-2 overflow-hidden">
      {settingsMode && !collapsed ? (
        <AppSidebarSettingsMode />
      ) : (
        <>
          <div
            className={cn(
              "flex flex-col gap-2 overflow-y-auto",
              inOffice ? "flex-1 min-h-0 pb-8 scroll-pb-8" : "shrink-0",
            )}
            data-testid="app-sidebar-scroll"
          >
            <AppSidebarPrimaryNav collapsed={collapsed} />
            <PluginNavItems collapsed={collapsed} />
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
          {inOffice && (
            <div
              aria-hidden="true"
              className="pointer-events-none absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-background to-transparent"
              data-testid="app-sidebar-bottom-fade"
            />
          )}
        </>
      )}
    </nav>
  );
}

/**
 * Unified app sidebar mounted at the root layout. Replaces the legacy
 * WorkspaceRail + OfficeSidebar + dockview-embedded sidebar surfaces.
 *
 * Width: at least 320px expanded (user-resizable) / 56px collapsed. The flex
 * reservation snaps between states so the workbench performs one layout pass,
 * while the absolutely-positioned visual panel keeps the 300ms width animation.
 * Desktop-only (`hidden md:block`) — mobile surfaces carry their own nav (mobile
 * headers and menu sheets), so the global rail never overlays mobile content.
 */
export function AppSidebar() {
  const collapsed = useAppStore((s) => s.appSidebar.collapsed);
  const settingsMode = useAppStore((s) => s.appSidebar.settingsMode);
  const sectionExpanded = useAppStore((s) => s.appSidebar.sectionExpanded);
  const storedWidth = useAppStore((s) => s.appSidebar.width);
  const toggleSection = useAppStore((s) => s.toggleAppSidebarSection);
  const toggleCollapsed = useAppStore((s) => s.toggleAppSidebar);
  const toggleSettingsMode = useAppStore((s) => s.toggleAppSidebarSettingsMode);
  const setSettingsMode = useAppStore((s) => s.setAppSidebarSettingsMode);
  const setWidth = useAppStore((s) => s.setAppSidebarWidth);
  const pathname = usePathname();
  const inOffice = useInOffice();
  const [isResizing, setIsResizing] = useState(false);

  // Keep `state.workflows.items` in sync with the active workspace at the top
  // of the always-mounted sidebar. Downstream consumers (the workspace picker,
  // Tasks section, kanban board) all assume this state exists for the current
  // workspace — hoisting it above the collapsible Tasks section is required so
  // a user with the section collapsed still gets fresh workflows on a switch.
  useEnsureWorkspaceWorkflows();

  const handleResize = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsResizing(true);
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
        setIsResizing(false);
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };
      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    },
    [storedWidth, setWidth],
  );

  const expandedWidth = Math.max(APP_SIDEBAR_EXPANDED_WIDTH, storedWidth);
  const targetWidth = collapsed ? APP_SIDEBAR_COLLAPSED_WIDTH : expandedWidth;
  const settingsModeTogglePathnameRef = useRef<string | null>(null);

  const handleToggleSettingsMode = useCallback(() => {
    // History updates before React renders the new pathname; remember which
    // route the user actually clicked on so delayed route sync cannot undo it.
    settingsModeTogglePathnameRef.current = window.location.pathname;
    toggleSettingsMode();
  }, [toggleSettingsMode]);

  // Keep the transient settings takeover aligned with route ownership. It is
  // intentionally not persisted, so direct reloads on `/settings/...` need to
  // enter it from the current pathname. Key on actual pathname changes so a
  // user can still close the takeover while staying on a settings page.
  const prevPathnameRef = useRef<string | null>(null);
  useEffect(() => {
    if (!pathname || prevPathnameRef.current === pathname) return;
    prevPathnameRef.current = pathname;
    if (isSettingsRoute(pathname)) {
      settingsModeTogglePathnameRef.current = null;
      if (!settingsMode) setSettingsMode(true);
      return;
    }
    if (settingsMode && settingsModeTogglePathnameRef.current !== pathname) {
      setSettingsMode(false);
    }
  }, [pathname, settingsMode, setSettingsMode]);

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
    <div
      data-testid="app-sidebar-layout"
      className="relative z-30 hidden h-full min-h-0 shrink-0 overflow-visible md:block"
      style={{ width: targetWidth }}
    >
      <aside
        data-testid="app-sidebar"
        data-collapsed={collapsed ? "true" : "false"}
        className={cn(
          "absolute inset-y-0 left-0 flex min-h-0 flex-col border-r border-border bg-background",
          // The panel is outside root flex flow, so its transition cannot make
          // the workbench or Dockview reflow on intermediate animation frames.
          // On collapse it briefly overdraws the snapped layout slot and stays
          // interactive so the new rail can be expanded again immediately.
          isResizing
            ? "transition-none"
            : "transition-[width] duration-300 ease-out motion-reduce:transition-none",
        )}
        style={{ width: targetWidth }}
      >
        <div
          data-testid="app-sidebar-content"
          className="flex min-h-0 flex-1 flex-col overflow-hidden"
        >
          <AppSidebarHeader collapsed={collapsed} onToggleCollapse={toggleCollapsed} />
          <AppSidebarNavigation
            collapsed={collapsed}
            inOffice={inOffice}
            settingsMode={settingsMode}
          />
          <AppSidebarFooter collapsed={collapsed} onToggleSettingsMode={handleToggleSettingsMode} />
        </div>
        {!collapsed && <AppSidebarResizeHandle onMouseDown={handleResize} />}
      </aside>
    </div>
  );
}
