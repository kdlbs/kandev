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
  getRootSplitview,
  fromDockviewApi,
  filterEphemeral,
  defaultLayout,
  mergeCurrentPanelsIntoPreset,
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
  addVscodePanel: () => void;
  openInternalVscode: (goto_: { file: string; line: number; col: number } | null) => void;
  addPlanPanel: (groupId?: string) => void;
  addPRPanel: () => void;
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
  activeFilePath: string | null;
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

/** Read live column widths from dockview's splitview and persist them as pinned overrides.
 *  Only syncs widths for columns identified as "sidebar" or "right" to avoid
 *  capturing plan/preview/vscode column widths as stale "right" overrides. */
function syncPinnedWidthsFromApi(api: DockviewApi, set: StoreSet): void {
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  try {
    const state = fromDockviewApi(api);
    const updates = new Map<string, number>();
    for (let i = 0; i < state.columns.length; i++) {
      const col = state.columns[i];
      if (col.id === "sidebar" || col.id === "right") {
        const w = sv.getViewSize(i);
        if (w > 50) updates.set(col.id, w);
      }
    }
    if (updates.size > 0) {
      set((prev) => {
        const m = new Map(prev.pinnedWidths);
        for (const [k, v] of updates) m.set(k, v);
        return { pinnedWidths: m };
      });
    }
  } catch {
    /* noop */
  }
}

/** Capture the live sidebar/right pixel widths into pinnedWidths before a layout rebuild. */
function captureLiveWidths(api: DockviewApi, set: StoreSet): Map<string, number> {
  syncPinnedWidthsFromApi(api, set);
  return useDockviewStore.getState().pinnedWidths;
}

function performSessionSwitch(
  api: DockviewApi,
  oldSessionId: string | null,
  newSessionId: string,
  safeWidth: number,
  safeHeight: number,
  buildDefault: (api: DockviewApi) => void,
): LayoutGroupIds | null {
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
      // Force correct dimensions — fromJSON may use a stale container size
      api.layout(safeWidth, safeHeight);
      return applyLayoutFixups(api);
    } catch {
      /* fall through */
    }
  }
  buildDefault(api);
  return null;
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
      const { api, sidebarVisible } = get();
      if (!api) return;
      const liveWidths = captureLiveWidths(api, set);
      if (sidebarVisible) {
        const current = fromDockviewApi(api);
        const withoutSidebar: LayoutState = {
          columns: current.columns.filter((c) => c.id !== "sidebar"),
        };
        set({ isRestoringLayout: true, sidebarVisible: false });
        applyLayoutAndSet(api, withoutSidebar, liveWidths, set);
        requestAnimationFrame(() => {
          syncPinnedWidthsFromApi(api, set);
          set({ isRestoringLayout: false });
        });
      } else {
        const current = fromDockviewApi(api);
        const sidebarCol = defaultLayout().columns[0];
        const withSidebar: LayoutState = {
          columns: [sidebarCol, ...current.columns],
        };
        set({ isRestoringLayout: true, sidebarVisible: true });
        applyLayoutAndSet(api, withSidebar, liveWidths, set);
        requestAnimationFrame(() => {
          syncPinnedWidthsFromApi(api, set);
          set({ isRestoringLayout: false });
        });
      }
    },
    toggleRightPanels: () => {
      const { api, rightPanelsVisible } = get();
      if (!api) return;
      const liveWidths = captureLiveWidths(api, set);
      if (rightPanelsVisible) {
        const current = fromDockviewApi(api);
        const withoutRight: LayoutState = {
          columns: current.columns.filter(
            (c) => !c.groups.some((g) => g.panels.some((p) => p.id === "files" || p.id === "changes")),
          ),
        };
        set({ isRestoringLayout: true, rightPanelsVisible: false });
        applyLayoutAndSet(api, withoutRight, liveWidths, set);
        requestAnimationFrame(() => {
          syncPinnedWidthsFromApi(api, set);
          set({ isRestoringLayout: false });
        });
      } else {
        const defLayout = defaultLayout();
        const rightCol = defLayout.columns.find((c) => c.id === "right");
        if (!rightCol) return;
        const current = fromDockviewApi(api);
        const withRight: LayoutState = {
          columns: [...current.columns, rightCol],
        };
        set({ isRestoringLayout: true, rightPanelsVisible: true });
        applyLayoutAndSet(api, withRight, liveWidths, set);
        requestAnimationFrame(() => {
          syncPinnedWidthsFromApi(api, set);
          set({ isRestoringLayout: false });
        });
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
      const { api } = get();
      if (!api) return;
      const liveWidths = captureLiveWidths(api, set);
      // Capture dimensions before layout change — api.width can become stale
      // inside the rAF callback after dockview serialization
      const safeWidth = api.width;
      const safeHeight = api.height;
      set({ isRestoringLayout: true });
      const presetState = getPresetLayout(preset);
      const state = mergeCurrentPanelsIntoPreset(api, presetState);
      // Remove stale pinned overrides for columns absent in the target layout
      const targetColumnIds = new Set(state.columns.map((c) => c.id));
      const cleanedWidths = new Map(liveWidths);
      for (const key of cleanedWidths.keys()) {
        if (!targetColumnIds.has(key)) cleanedWidths.delete(key);
      }
      const ids = applyLayout(api, state, cleanedWidths);
      set({
        ...ids,
        sidebarVisible: true,
        rightPanelsVisible: preset === "default",
        pinnedWidths: cleanedWidths,
      });
      requestAnimationFrame(() => {
        api.layout(safeWidth, safeHeight);
        syncPinnedWidthsFromApi(api, set);
        set({ isRestoringLayout: false });
      });
    },
    applyCustomLayout: (layout: SavedLayoutConfig) => {
      const { api } = get();
      if (!api) return;
      const liveWidths = captureLiveWidths(api, set);
      const safeWidth = api.width;
      const safeHeight = api.height;
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
        const ids = applyLayout(api, state, liveWidths);
        set(ids);
      }
      const hasSidebar = !!api.getPanel("sidebar");
      const colCount = state?.columns?.length ?? api.groups.length;
      const sidebarCols = hasSidebar ? 1 : 0;
      const hasRight = colCount > sidebarCols + 1;
      set({ sidebarVisible: hasSidebar, rightPanelsVisible: hasRight });
      requestAnimationFrame(() => {
        api.layout(safeWidth, safeHeight);
        syncPinnedWidthsFromApi(api, set);
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

type SimplePanelOpts = {
  id: string;
  component: string;
  title: string;
  params?: Record<string, unknown>;
};

function addSimplePanel(api: DockviewApi, groupId: string, opts: SimplePanelOpts): void {
  focusOrAddPanel(api, { ...opts, position: { referenceGroup: groupId } });
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
      addSimplePanel(api, groupId ?? rightTopGroupId, {
        id: "changes",
        component: "changes",
        title: "Changes",
      });
    },
    addFilesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? rightTopGroupId, {
        id: "files",
        component: "files",
        title: "Files",
      });
    },
    addDiffViewerPanel: (path?: string, content?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      if (path) set({ selectedDiff: { path, content } });
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: "diff-viewer",
        component: "diff-viewer",
        title: "Diff Viewer",
        params: { kind: "all" },
      });
    },
    addFileDiffPanel: (path: string, content?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: `diff:file:${path}`,
        component: "diff-viewer",
        title: `Diff [${getFileName(path)}]`,
        params: { kind: "file", path, content },
      });
    },
    addCommitDetailPanel: (sha: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: `commit:${sha}`,
        component: "commit-detail",
        title: sha.slice(0, 7),
        params: { commitSha: sha },
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
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: browserId,
        component: "browser",
        title: "Browser",
        params: { url: url ?? "" },
      });
    },
  };
}

function buildExtraPanelActions(_set: StoreSet, get: StoreGet) {
  return {
    addVscodePanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: "VS Code",
        position: { referenceGroup: centerGroupId },
      });
    },
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    openInternalVscode: (_goto: { file: string; line: number; col: number } | null) => {
      const { api } = get();
      if (!api) return;

      const existing = api.getPanel("vscode");
      if (existing) {
        existing.api.setActive();
        return;
      }

      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: "VS Code",
        position: { referencePanel: "chat", direction: "right" },
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
    addPRPanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "pr-detail",
        component: "pr-detail",
        title: "Pull Request",
        position: { referenceGroup: centerGroupId },
      });
    },
    addTerminalPanel: (terminalId?: string, groupId?: string) => {
      const { api, rightBottomGroupId } = get();
      if (!api) return;
      const id = terminalId ?? `terminal-${Date.now()}`;
      addSimplePanel(api, groupId ?? rightBottomGroupId, {
        id,
        component: "terminal",
        title: "Terminal",
        params: { terminalId: id },
      });
    },
  };
}

export const useDockviewStore = create<DockviewStore>((set, get) => ({
  api: null,
  activeFilePath: null,
  setApi: (api) => {
    set({ api, activeFilePath: null });
    if (api) {
      api.onDidActivePanelChange((event) => {
        const id = event?.id;
        set({ activeFilePath: id?.startsWith("file:") ? id.slice(5) : null });
      });
    }
  },
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
    const safeWidth = api.width;
    const safeHeight = api.height;
    set({ isRestoringLayout: true, currentLayoutSessionId: newSessionId });
    try {
      const ids = performSessionSwitch(api, oldSessionId, newSessionId, safeWidth, safeHeight, (a) => get().buildDefaultLayout(a));
      if (ids) set(ids);
    } finally {
      set({ isRestoringLayout: false });
    }
  },
  buildDefaultLayout: (api) => {
    const { userDefaultLayout } = get();
    // Clear pinned overrides so fresh default gets clean preset widths
    const freshPinned = new Map<string, number>();
    set({ isRestoringLayout: true, pinnedWidths: freshPinned });
    // Check if plan mode was queued before navigation (e.g. task created in plan mode).
    // If so, use the plan preset directly and consume the deferred actions.
    const pending = get().deferredPanelActions;
    const hasPlanAction = pending.some((a) => a.id === "plan");
    if (hasPlanAction) {
      set({ deferredPanelActions: [] });
      const planState = getPresetLayout("plan");
      const ids = applyLayout(api, planState, freshPinned);
      set({ ...ids, sidebarVisible: true, rightPanelsVisible: false });
      // Apply any remaining non-plan deferred actions
      const remaining = pending.filter((a) => a.id !== "plan");
      if (remaining.length > 0) applyDeferredPanelActions(api, remaining);
    } else {
      const state = userDefaultLayout ?? getPresetLayout("default");
      const ids = applyLayout(api, state, freshPinned);
      const hasSidebar = state.columns.some((c) => c.id === "sidebar");
      const hasRight = state.columns.length > (hasSidebar ? 2 : 1);
      set({ ...ids, sidebarVisible: hasSidebar, rightPanelsVisible: hasRight });
      if (pending.length > 0) {
        set({ deferredPanelActions: [] });
        applyDeferredPanelActions(api, pending);
      }
    }
    requestAnimationFrame(() => {
      syncPinnedWidthsFromApi(api, set);
      set({ isRestoringLayout: false });
    });
  },
  resetLayout: () => {
    const { api } = get();
    if (api) get().buildDefaultLayout(api);
  },
  ...buildPanelActions(set, get),
  ...buildExtraPanelActions(set, get),
}));

/** Perform a layout switch between sessions. */
export function performLayoutSwitch(oldSessionId: string | null, newSessionId: string): void {
  useDockviewStore.getState().switchSessionLayout(oldSessionId, newSessionId);
}
