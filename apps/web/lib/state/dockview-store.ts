import { create } from "zustand";
import type { DockviewApi, AddPanelOptions, SerializedDockview } from "dockview-react";
import { getSessionLayout, setSessionLayout } from "@/lib/local-storage";
import { applyLayoutFixups, focusOrAddPanel } from "./dockview-layout-builders";
import {
  SIDEBAR_GROUP,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  getPresetLayout,
  applyLayout,
  fromDockviewApi,
  filterEphemeral,
  defaultLayout,
} from "./layout-manager";
import type { BuiltInPreset, LayoutState, LayoutGroupIds } from "./layout-manager";

// Re-export types and constants used by other modules
export type { BuiltInPreset } from "./layout-manager";
export {
  LAYOUT_SIDEBAR_RATIO,
  LAYOUT_RIGHT_RATIO,
  LAYOUT_SIDEBAR_MAX_PX,
  LAYOUT_RIGHT_MAX_PX,
} from "./layout-manager";
export { applyLayoutFixups } from "./dockview-layout-builders";

export type FileEditorState = {
  path: string;
  name: string;
  content: string;
  originalContent: string;
  originalHash: string;
  isDirty: boolean;
  isBinary?: boolean;
  hasRemoteUpdate?: boolean;
  remoteContent?: string;
  remoteOriginalHash?: string;
};

/** Direction relative to a reference panel or group. */
export type PanelDirection = "left" | "right" | "above" | "below";

/** A deferred panel operation applied after the next layout build / restore. */
export type DeferredPanelAction = {
  id: string;
  component: string;
  title: string;
  placement: "tab" | PanelDirection;
  referencePanel?: string;
  params?: Record<string, unknown>;
};

/** Saved layout configuration persisted to user settings. */
export type SavedLayoutConfig = {
  id: string;
  name: string;
  isDefault: boolean;
  layout: Record<string, unknown>;
  createdAt: string;
};

type DockviewStore = {
  api: DockviewApi | null;
  setApi: (api: DockviewApi | null) => void;
  openFiles: Map<string, FileEditorState>;
  setFileState: (path: string, state: FileEditorState) => void;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
  removeFileState: (path: string) => void;
  clearFileStates: () => void;
  buildDefaultLayout: (api: DockviewApi) => void;
  resetLayout: () => void;
  addChatPanel: () => void;
  addChangesPanel: (groupId?: string) => void;
  addFilesPanel: (groupId?: string) => void;
  addDiffViewerPanel: (path?: string, content?: string, groupId?: string) => void;
  addFileDiffPanel: (path: string, content?: string, groupId?: string) => void;
  addCommitDetailPanel: (sha: string, groupId?: string) => void;
  addFileEditorPanel: (path: string, name: string, quiet?: boolean) => void;
  addBrowserPanel: (url?: string, groupId?: string) => void;
  addPlanPanel: (groupId?: string) => void;
  addTerminalPanel: (terminalId?: string, groupId?: string) => void;
  selectedDiff: { path: string; content?: string } | null;
  setSelectedDiff: (diff: { path: string; content?: string } | null) => void;
  activeGroupId: string | null;
  centerGroupId: string;
  rightTopGroupId: string;
  rightBottomGroupId: string;
  sidebarGroupId: string;
  sidebarVisible: boolean;
  rightPanelsVisible: boolean;
  toggleSidebar: () => void;
  toggleRightPanels: () => void;
  setSidebarVisible: (visible: boolean) => void;
  setRightPanelsVisible: (visible: boolean) => void;
  applyBuiltInPreset: (preset: BuiltInPreset) => void;
  applyCustomLayout: (layout: SavedLayoutConfig) => void;
  captureCurrentLayout: () => Record<string, unknown>;
  isRestoringLayout: boolean;
  currentLayoutSessionId: string | null;
  switchSessionLayout: (oldSessionId: string | null, newSessionId: string) => void;
  deferredPanelActions: DeferredPanelAction[];
  queuePanelAction: (action: DeferredPanelAction) => void;
  pinnedWidths: Map<string, number>;
  setPinnedWidth: (columnId: string, width: number) => void;
  userDefaultLayout: LayoutState | null;
  setUserDefaultLayout: (layout: LayoutState | null) => void;
};

type StoreGet = () => DockviewStore;
type StoreSet = (
  partial: Partial<DockviewStore> | ((s: DockviewStore) => Partial<DockviewStore>),
) => void;

function applyDeferredPanelActions(api: DockviewApi, actions: DeferredPanelAction[]): void {
  for (const action of actions) {
    const ref = action.referencePanel ?? "chat";
    let position: AddPanelOptions["position"];
    if (action.placement === "tab") {
      const groupId = api.getPanel(ref)?.group?.id;
      if (groupId) position = { referenceGroup: groupId };
    } else {
      position = { referencePanel: ref, direction: action.placement };
    }
    focusOrAddPanel(api, {
      id: action.id,
      component: action.component,
      title: action.title,
      position,
      ...(action.params ? { params: action.params } : {}),
    });
  }
}

function performSessionSwitch(
  api: DockviewApi,
  oldSessionId: string | null,
  newSessionId: string,
  buildDefault: (api: DockviewApi) => void,
): void {
  if (oldSessionId) {
    try {
      setSessionLayout(oldSessionId, api.toJSON());
    } catch {
      /* ignore */
    }
  }
  const saved = getSessionLayout(newSessionId);
  if (saved) {
    try {
      api.fromJSON(saved as SerializedDockview);
      return;
    } catch {
      /* fall through */
    }
  }
  buildDefault(api);
}

function buildFileStateActions(set: StoreSet) {
  return {
    setFileState: (path: string, state: FileEditorState) => {
      set((prev) => {
        const m = new Map(prev.openFiles);
        m.set(path, state);
        return { openFiles: m };
      });
    },
    updateFileState: (path: string, updates: Partial<FileEditorState>) => {
      set((prev) => {
        const e = prev.openFiles.get(path);
        if (!e) return prev;
        const m = new Map(prev.openFiles);
        m.set(path, { ...e, ...updates });
        return { openFiles: m };
      });
    },
    removeFileState: (path: string) => {
      set((prev) => {
        const m = new Map(prev.openFiles);
        m.delete(path);
        return { openFiles: m };
      });
    },
    clearFileStates: () => {
      set({ openFiles: new Map() });
    },
  };
}

function applyLayoutAndSet(
  api: DockviewApi,
  state: LayoutState,
  pinnedWidths: Map<string, number>,
  set: StoreSet,
): LayoutGroupIds {
  const ids = applyLayout(api, state, pinnedWidths);
  set(ids);
  return ids;
}

function buildVisibilityActions(set: StoreSet, get: StoreGet) {
  return {
    toggleSidebar: () => {
      const { api, sidebarVisible, pinnedWidths } = get();
      if (!api) return;
      if (sidebarVisible) {
        const current = fromDockviewApi(api);
        const withoutSidebar: LayoutState = {
          columns: current.columns.filter((c) => c.id !== "sidebar"),
        };
        set({ isRestoringLayout: true, sidebarVisible: false });
        applyLayoutAndSet(api, withoutSidebar, pinnedWidths, set);
        requestAnimationFrame(() => set({ isRestoringLayout: false }));
      } else {
        const current = fromDockviewApi(api);
        const sidebarCol = defaultLayout().columns[0];
        const withSidebar: LayoutState = {
          columns: [sidebarCol, ...current.columns],
        };
        set({ isRestoringLayout: true, sidebarVisible: true });
        applyLayoutAndSet(api, withSidebar, pinnedWidths, set);
        requestAnimationFrame(() => set({ isRestoringLayout: false }));
      }
    },
    toggleRightPanels: () => {
      const { api, rightPanelsVisible, pinnedWidths } = get();
      if (!api) return;
      if (rightPanelsVisible) {
        const current = fromDockviewApi(api);
        const centerIdx = current.columns.findIndex((c) => c.id === "center");
        const withoutRight: LayoutState = {
          columns: current.columns.slice(0, centerIdx + 1),
        };
        set({ isRestoringLayout: true, rightPanelsVisible: false });
        applyLayoutAndSet(api, withoutRight, pinnedWidths, set);
        requestAnimationFrame(() => set({ isRestoringLayout: false }));
      } else {
        const defLayout = defaultLayout();
        const rightCol = defLayout.columns.find((c) => c.id === "right");
        if (!rightCol) return;
        const current = fromDockviewApi(api);
        const withRight: LayoutState = {
          columns: [...current.columns, rightCol],
        };
        set({ isRestoringLayout: true, rightPanelsVisible: true });
        applyLayoutAndSet(api, withRight, pinnedWidths, set);
        requestAnimationFrame(() => set({ isRestoringLayout: false }));
      }
    },
    setSidebarVisible: (visible: boolean) => {
      const { sidebarVisible } = get();
      if (sidebarVisible === visible) return;
      get().toggleSidebar();
    },
    setRightPanelsVisible: (visible: boolean) => {
      const { rightPanelsVisible } = get();
      if (rightPanelsVisible === visible) return;
      get().toggleRightPanels();
    },
  };
}

function buildPresetActions(set: StoreSet, get: StoreGet) {
  return {
    applyBuiltInPreset: (preset: BuiltInPreset) => {
      const { api, pinnedWidths } = get();
      if (!api) return;
      set({ isRestoringLayout: true });
      const state = getPresetLayout(preset);
      const ids = applyLayout(api, state, pinnedWidths);
      set({
        ...ids,
        sidebarVisible: true,
        rightPanelsVisible: preset === "default",
      });
      requestAnimationFrame(() => {
        api.layout(api.width, api.height);
        set({ isRestoringLayout: false });
      });
    },
    applyCustomLayout: (layout: SavedLayoutConfig) => {
      const { api, pinnedWidths } = get();
      if (!api) return;
      set({ isRestoringLayout: true });
      const state = layout.layout as unknown as LayoutState;
      if (!state?.columns) {
        try {
          api.fromJSON(layout.layout as unknown as SerializedDockview);
          set(applyLayoutFixups(api));
        } catch (e) {
          console.warn("applyCustomLayout: old-format restore failed:", e);
        }
      } else {
        const ids = applyLayout(api, state, pinnedWidths);
        set(ids);
      }
      const hasSidebar = !!api.getPanel("sidebar");
      const colCount = state?.columns?.length ?? api.groups.length;
      const sidebarCols = hasSidebar ? 1 : 0;
      const hasRight = colCount > sidebarCols + 1;
      set({ sidebarVisible: hasSidebar, rightPanelsVisible: hasRight });
      requestAnimationFrame(() => {
        api.layout(api.width, api.height);
        set({ isRestoringLayout: false });
      });
    },
    captureCurrentLayout: (): Record<string, unknown> => {
      const { api } = get();
      if (!api) return {};
      const state = fromDockviewApi(api);
      const filtered = filterEphemeral(state);
      return filtered as unknown as Record<string, unknown>;
    },
  };
}

function buildPanelActions(set: StoreSet, get: StoreGet) {
  const getFileName = (path: string) => path.split("/").pop() || path;

  return {
    addChatPanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "chat",
        component: "chat",
        tabComponent: "permanentTab",
        title: "Agent",
        position: { referenceGroup: centerGroupId },
      });
    },
    addChangesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "changes",
        component: "changes",
        title: "Changes",
        position: { referenceGroup: groupId ?? rightTopGroupId },
      });
    },
    addFilesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "files",
        component: "files",
        title: "Files",
        position: { referenceGroup: groupId ?? rightTopGroupId },
      });
    },
    addDiffViewerPanel: (path?: string, content?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      if (path) set({ selectedDiff: { path, content } });
      focusOrAddPanel(api, {
        id: "diff-viewer",
        component: "diff-viewer",
        title: "Diff Viewer",
        params: { kind: "all" },
        position: { referenceGroup: groupId ?? centerGroupId },
      });
    },
    addFileDiffPanel: (path: string, content?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const id = `diff:file:${path}`;
      focusOrAddPanel(api, {
        id,
        component: "diff-viewer",
        title: `Diff [${getFileName(path)}]`,
        params: { kind: "file", path, content },
        position: { referenceGroup: groupId ?? centerGroupId },
      });
    },
    addCommitDetailPanel: (sha: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: `commit:${sha}`,
        component: "commit-detail",
        title: sha.slice(0, 7),
        params: { commitSha: sha },
        position: { referenceGroup: groupId ?? centerGroupId },
      });
    },
    addFileEditorPanel: (path: string, name: string, quiet?: boolean) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(
        api,
        {
          id: `file:${path}`,
          component: "file-editor",
          title: name,
          params: { path },
          position: { referenceGroup: centerGroupId },
        },
        quiet,
      );
    },
    addBrowserPanel: (url?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const browserId = url ? `browser:${url}` : `browser:${Date.now()}`;
      focusOrAddPanel(api, {
        id: browserId,
        component: "browser",
        title: "Browser",
        params: { url: url ?? "" },
        position: { referenceGroup: groupId ?? centerGroupId },
      });
    },
    addPlanPanel: (groupId?: string) => {
      const { api } = get();
      if (!api) return;
      const position = groupId
        ? { referenceGroup: groupId }
        : { referencePanel: "chat" as const, direction: "right" as const };
      focusOrAddPanel(api, { id: "plan", component: "plan", title: "Plan", position });
    },
    addTerminalPanel: (terminalId?: string, groupId?: string) => {
      const { api, rightBottomGroupId } = get();
      if (!api) return;
      const id = terminalId ?? `terminal-${Date.now()}`;
      focusOrAddPanel(api, {
        id,
        component: "terminal",
        title: "Terminal",
        params: { terminalId: id },
        position: { referenceGroup: groupId ?? rightBottomGroupId },
      });
    },
  };
}

export const useDockviewStore = create<DockviewStore>((set, get) => ({
  api: null,
  setApi: (api) => set({ api }),
  activeGroupId: null,
  selectedDiff: null,
  setSelectedDiff: (diff) => set({ selectedDiff: diff }),
  openFiles: new Map(),
  ...buildFileStateActions(set),
  centerGroupId: CENTER_GROUP,
  rightTopGroupId: RIGHT_TOP_GROUP,
  rightBottomGroupId: RIGHT_BOTTOM_GROUP,
  sidebarGroupId: SIDEBAR_GROUP,
  sidebarVisible: true,
  rightPanelsVisible: true,
  pinnedWidths: new Map(),
  setPinnedWidth: (columnId, width) => {
    set((prev) => {
      const m = new Map(prev.pinnedWidths);
      m.set(columnId, width);
      return { pinnedWidths: m };
    });
  },
  userDefaultLayout: null,
  setUserDefaultLayout: (layout) => set({ userDefaultLayout: layout }),
  ...buildVisibilityActions(set, get),
  ...buildPresetActions(set, get),
  isRestoringLayout: false,
  currentLayoutSessionId: null,
  deferredPanelActions: [],
  queuePanelAction: (action) =>
    set((prev) => ({
      deferredPanelActions: [...prev.deferredPanelActions, action],
    })),
  switchSessionLayout: (oldSessionId, newSessionId) => {
    const { api, currentLayoutSessionId } = get();
    if (!api || currentLayoutSessionId === newSessionId) return;
    set({ isRestoringLayout: true, currentLayoutSessionId: newSessionId });
    try {
      performSessionSwitch(api, oldSessionId, newSessionId, (a) => get().buildDefaultLayout(a));
      if (getSessionLayout(newSessionId)) set(applyLayoutFixups(api));
    } finally {
      set({ isRestoringLayout: false });
    }
  },
  buildDefaultLayout: (api) => {
    const { userDefaultLayout } = get();
    // Clear pinned overrides so fresh default gets clean preset widths
    const freshPinned = new Map<string, number>();
    set({ isRestoringLayout: true, pinnedWidths: freshPinned });
    const state = userDefaultLayout ?? getPresetLayout("default");
    const ids = applyLayout(api, state, freshPinned);
    const hasSidebar = state.columns.some((c) => c.id === "sidebar");
    const hasRight = state.columns.length > (hasSidebar ? 2 : 1);
    set({ ...ids, sidebarVisible: hasSidebar, rightPanelsVisible: hasRight });
    const pending = get().deferredPanelActions;
    if (pending.length > 0) {
      set({ deferredPanelActions: [] });
      applyDeferredPanelActions(api, pending);
    }
    requestAnimationFrame(() => set({ isRestoringLayout: false }));
  },
  resetLayout: () => {
    const { api } = get();
    if (api) get().buildDefaultLayout(api);
  },
  ...buildPanelActions(set, get),
}));

/** Perform a layout switch between sessions. */
export function performLayoutSwitch(oldSessionId: string | null, newSessionId: string): void {
  useDockviewStore.getState().switchSessionLayout(oldSessionId, newSessionId);
}
