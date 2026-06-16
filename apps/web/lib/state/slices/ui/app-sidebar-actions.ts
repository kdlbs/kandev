import type { StateCreator } from "zustand";
import {
  getStoredAppSidebarCollapsed,
  getStoredAppSidebarSectionExpanded,
  getStoredAppSidebarWidth,
  setStoredAppSidebarCollapsed,
  setStoredAppSidebarSectionExpanded,
  setStoredAppSidebarWidth,
} from "@/lib/local-storage-app-sidebar";
import { APP_SIDEBAR_EXPANDED_WIDTH } from "@/components/app-sidebar/app-sidebar-constants";
import type { AppSidebarState, UISlice } from "./types";

/** Keep high-value navigation open by default; dynamic entity sections stay
 *  collapsed so the unified sidebar does not default too tall on first open. */
export const DEFAULT_SECTION_EXPANDED: Record<string, boolean> = {
  tasks: true,
  "office-work": true,
  "office-workspace": true,
  projects: false,
  agents: false,
  integrations: false,
  settings: false,
};

export function loadAppSidebarState(): AppSidebarState {
  return {
    collapsed: getStoredAppSidebarCollapsed(false),
    sectionExpanded: getStoredAppSidebarSectionExpanded(DEFAULT_SECTION_EXPANDED),
    width: getStoredAppSidebarWidth(APP_SIDEBAR_EXPANDED_WIDTH),
    // Transient — always starts off, never read from / written to storage.
    settingsMode: false,
  };
}

type ImmerSet = Parameters<StateCreator<UISlice, [["zustand/immer", never]], [], UISlice>>[0];

export function buildAppSidebarActions(set: ImmerSet) {
  return {
    toggleAppSidebar: () =>
      set((draft) => {
        const next = !draft.appSidebar.collapsed;
        draft.appSidebar.collapsed = next;
        setStoredAppSidebarCollapsed(next);
      }),
    setAppSidebarCollapsed: (collapsed: boolean) =>
      set((draft) => {
        if (draft.appSidebar.collapsed === collapsed) return;
        draft.appSidebar.collapsed = collapsed;
        setStoredAppSidebarCollapsed(collapsed);
      }),
    toggleAppSidebarSection: (sectionId: string, defaultExpanded = false) =>
      set((draft) => {
        const current = draft.appSidebar.sectionExpanded[sectionId] ?? defaultExpanded;
        draft.appSidebar.sectionExpanded[sectionId] = !current;
        setStoredAppSidebarSectionExpanded({ ...draft.appSidebar.sectionExpanded });
      }),
    setAppSidebarWidth: (width: number) =>
      set((draft) => {
        if (draft.appSidebar.width === width) return;
        draft.appSidebar.width = width;
        setStoredAppSidebarWidth(width);
      }),
    toggleAppSidebarSettingsMode: () =>
      set((draft) => {
        const next = !draft.appSidebar.settingsMode;
        draft.appSidebar.settingsMode = next;
        // Entering settings mode while collapsed would render an empty rail —
        // the tree needs the expanded width — so force-expand on the way in.
        if (next && draft.appSidebar.collapsed) {
          draft.appSidebar.collapsed = false;
          setStoredAppSidebarCollapsed(false);
        }
      }),
  };
}
