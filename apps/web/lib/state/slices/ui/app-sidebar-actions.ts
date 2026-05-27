import type { StateCreator } from "zustand";
import {
  getStoredAppSidebarCollapsed,
  getStoredAppSidebarSectionExpanded,
  getStoredAppSidebarWidth,
  setStoredAppSidebarCollapsed,
  setStoredAppSidebarSectionExpanded,
  setStoredAppSidebarWidth,
} from "@/lib/local-storage";
import { APP_SIDEBAR_EXPANDED_WIDTH } from "@/components/app-sidebar/app-sidebar-constants";
import type { AppSidebarState, UISlice } from "./types";

/** Tasks expanded by default; other sections collapsed. Mirrors the
 *  "open question / risks" note in the spec: keep the unified sidebar from
 *  defaulting too tall on first open. */
export const DEFAULT_SECTION_EXPANDED: Record<string, boolean> = {
  tasks: true,
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
    toggleAppSidebarSection: (sectionId: string) =>
      set((draft) => {
        const current = draft.appSidebar.sectionExpanded[sectionId] ?? false;
        draft.appSidebar.sectionExpanded[sectionId] = !current;
        setStoredAppSidebarSectionExpanded({ ...draft.appSidebar.sectionExpanded });
      }),
    setAppSidebarWidth: (width: number) =>
      set((draft) => {
        if (draft.appSidebar.width === width) return;
        draft.appSidebar.width = width;
        setStoredAppSidebarWidth(width);
      }),
  };
}
