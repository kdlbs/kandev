"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePathname } from "@/lib/routing/client-router";
import { isSettingsRoute } from "./app-sidebar-route";
import { useAppStore } from "@/components/state-provider";
import { useEnsureWorkspaceWorkflows } from "@/hooks/use-workflows";
import { useOfficeModeState } from "@/hooks/use-in-office";
import type { WorkspaceMode } from "@/components/workspace-scope-provider";
import { useOfficeWorkspaceData } from "@/hooks/use-office-workspace-data";
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
import { AutomationsSection } from "./sections/automations-section";
import { CanvasesSection } from "./sections/canvases-section";
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

type AppSidebarNavigationProps = {
  collapsed: boolean;
  mode: WorkspaceMode;
  settingsMode: boolean;
};

/**
 * Mode comes from the active workspace, which is not known on the first frame
 * of a boot whose payload ships no workspace list. Rendering only the
 * mode-independent rows — rather than assuming kanban — is what stops the
 * sidebar painting one mode's sections and swapping to the other's a tick
 * later.
 */
function AppSidebarUnresolvedNav({ collapsed }: { collapsed: boolean }) {
  return (
    <div className="flex flex-col gap-1" data-testid="app-sidebar-scroll">
      <AppSidebarPrimaryNav collapsed={collapsed} />
      <PluginNavItems collapsed={collapsed} />
    </div>
  );
}

function AppSidebarModeNav({ collapsed, inOffice }: { collapsed: boolean; inOffice: boolean }) {
  return (
    <>
      <div
        className={cn(
          "flex flex-col gap-1 overflow-y-auto",
          inOffice ? "flex-1 min-h-0 pb-8 scroll-pb-8" : "shrink-0",
        )}
        data-testid="app-sidebar-scroll"
      >
        <AppSidebarPrimaryNav collapsed={collapsed} />
        {/* Directly under New Task: an automation is a thing you keep, the
            same weight as a project, and the list IS the nav — picking one
            opens its history rather than a settings form. */}
        {!inOffice && <AutomationsSection collapsed={collapsed} />}
        {!inOffice && <CanvasesSection collapsed={collapsed} />}
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
  );
}

function AppSidebarNavigation({ collapsed, mode, settingsMode }: AppSidebarNavigationProps) {
  function content() {
    if (settingsMode && !collapsed) return <AppSidebarSettingsMode />;
    if (mode === "unknown") return <AppSidebarUnresolvedNav collapsed={collapsed} />;
    return <AppSidebarModeNav collapsed={collapsed} inOffice={mode === "office"} />;
  }

  return (
    <nav className="relative flex-1 min-h-0 flex flex-col gap-2 px-2 py-2 overflow-hidden">
      {content()}
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
/**
 * The transient settings takeover, kept aligned with route ownership. It is
 * intentionally not persisted, so a direct reload on `/settings/...` has to
 * enter it from the current pathname.
 *
 * Keyed on actual pathname changes, and only forced when *entering* the settings
 * surface: moving within it must not reopen a takeover the user closed. Leaving
 * it closes the takeover unless the user's own toggle was the thing that
 * navigated (`togglePathnameRef`), because history updates before React renders
 * the new pathname.
 */
function useSettingsTakeover(pathname: string | null) {
  const settingsMode = useAppStore((s) => s.appSidebar.settingsMode);
  const setSettingsMode = useAppStore((s) => s.setAppSidebarSettingsMode);
  const toggleSettingsMode = useAppStore((s) => s.toggleAppSidebarSettingsMode);
  const togglePathnameRef = useRef<string | null>(null);
  const prevPathnameRef = useRef<string | null>(null);

  const toggle = useCallback(() => {
    togglePathnameRef.current = window.location.pathname;
    toggleSettingsMode();
  }, [toggleSettingsMode]);

  useEffect(() => {
    if (!pathname || prevPathnameRef.current === pathname) return;
    const cameFromSettings = isSettingsRoute(prevPathnameRef.current);
    prevPathnameRef.current = pathname;
    if (isSettingsRoute(pathname)) {
      togglePathnameRef.current = null;
      // `/settings` handing off to the last page visited (see `SettingsIndex`)
      // is a settings-to-settings navigation, and so is every tree click.
      if (!settingsMode && !cameFromSettings) setSettingsMode(true);
      return;
    }
    if (settingsMode && togglePathnameRef.current !== pathname) {
      setSettingsMode(false);
    }
  }, [pathname, settingsMode, setSettingsMode]);

  return { settingsMode, toggleSettingsMode: toggle };
}

export function AppSidebar() {
  const collapsed = useAppStore((s) => s.appSidebar.collapsed);
  const sectionExpanded = useAppStore((s) => s.appSidebar.sectionExpanded);
  const storedWidth = useAppStore((s) => s.appSidebar.width);
  const toggleSection = useAppStore((s) => s.toggleAppSidebarSection);
  const toggleCollapsed = useAppStore((s) => s.toggleAppSidebar);
  const setWidth = useAppStore((s) => s.setAppSidebarWidth);
  const pathname = usePathname();
  const mode = useOfficeModeState();
  const [isResizing, setIsResizing] = useState(false);

  // Keep `state.workflows.items` in sync with the active workspace at the top
  // of the always-mounted sidebar. Downstream consumers (the workspace picker,
  // Tasks section, kanban board) all assume this state exists for the current
  // workspace — hoisting it above the collapsible Tasks section is required so
  // a user with the section collapsed still gets fresh workflows on a switch.
  useEnsureWorkspaceWorkflows();
  // The office collections the sidebar renders (Projects, Agents, the inbox
  // badge) load here so they follow the active workspace rather than the
  // `/office` route family. No-ops for a kanban workspace.
  useOfficeWorkspaceData();

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
  const { settingsMode, toggleSettingsMode: handleToggleSettingsMode } =
    useSettingsTakeover(pathname);

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
        <AppSidebarHeader collapsed={collapsed} onToggleCollapse={toggleCollapsed} />
        <div
          data-testid="app-sidebar-content"
          className="flex min-h-0 flex-1 flex-col overflow-hidden"
        >
          <AppSidebarNavigation collapsed={collapsed} mode={mode} settingsMode={settingsMode} />
          <AppSidebarFooter collapsed={collapsed} onToggleSettingsMode={handleToggleSettingsMode} />
        </div>
        {!collapsed && <AppSidebarResizeHandle onMouseDown={handleResize} />}
      </aside>
    </div>
  );
}
