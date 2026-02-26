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
import {
  injectIntentPanels,
  applyActivePanelOverrides,
  resolveNamedIntent,
} from "./layout-manager";
import { buildFileStateActions } from "./dockview-file-state";
import { buildPanelActions, buildExtraPanelActions } from "./dockview-panel-actions";
import { preserveChatScrollDuringLayout } from "./dockview-scroll-preserve";

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
  buildDefaultLayout: (api: DockviewApi, intentName?: string) => void;
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
  pendingChatScrollTop: number | null;
  setPendingChatScrollTop: (value: number | null) => void;
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
    if (state.columns.length !== sv.length) return;
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

type SessionSwitchParams = {
  api: DockviewApi;
  oldSessionId: string | null;
  newSessionId: string;
  safeWidth: number;
  safeHeight: number;
  buildDefault: (api: DockviewApi) => void;
};

function performSessionSwitch(params: SessionSwitchParams): LayoutGroupIds {
  const { api, oldSessionId, newSessionId, safeWidth, safeHeight, buildDefault } = params;
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
      api.layout(safeWidth, safeHeight);
      return applyLayoutFixups(api);
    } catch {
      /* fall through */
    }
  }
  buildDefault(api);
  api.layout(safeWidth, safeHeight);
  return applyLayoutFixups(api);
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
      preserveChatScrollDuringLayout();
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
      preserveChatScrollDuringLayout();
      if (rightPanelsVisible) {
        const current = fromDockviewApi(api);
        const withoutRight: LayoutState = {
          columns: current.columns.filter(
            (c) =>
              !c.groups.some((g) => g.panels.some((p) => p.id === "files" || p.id === "changes")),
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
      preserveChatScrollDuringLayout();
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
      preserveChatScrollDuringLayout();
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

function performBuildDefault(
  api: DockviewApi,
  set: StoreSet,
  get: StoreGet,
  intentName?: string,
): void {
  const { userDefaultLayout } = get();
  const intent = intentName ? resolveNamedIntent(intentName) : null;
  const freshPinned = new Map<string, number>();
  set({ isRestoringLayout: true, pinnedWidths: freshPinned });

  const basePreset = intent?.preset as BuiltInPreset | undefined;
  let state = basePreset
    ? getPresetLayout(basePreset)
    : (userDefaultLayout ?? getPresetLayout("default"));

  if (intent?.panels?.length) {
    state = injectIntentPanels(state, intent.panels);
  }
  if (intent?.activePanels) {
    state = applyActivePanelOverrides(state, intent.activePanels);
  }

  const ids = applyLayout(api, state, freshPinned);
  const hasSidebar = state.columns.some((c) => c.id === "sidebar");
  const hasRight = state.columns.length > (hasSidebar ? 2 : 1);
  set({ ...ids, sidebarVisible: hasSidebar, rightPanelsVisible: hasRight });

  const pending = get().deferredPanelActions;
  if (pending.length > 0) {
    set({ deferredPanelActions: [] });
    applyDeferredPanelActions(api, pending);
  }

  requestAnimationFrame(() => {
    syncPinnedWidthsFromApi(api, set);
    set({ isRestoringLayout: false });
  });
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
    // When the first session becomes active (oldSessionId is null and no prior
    // layout session), onReady already built the correct layout (possibly with
    // an intent). Just adopt the current layout for the new session without
    // rebuilding, which would lose the intent-based layout.
    if (!oldSessionId && !currentLayoutSessionId) {
      set({ currentLayoutSessionId: newSessionId });
      try {
        setSessionLayout(newSessionId, api.toJSON());
      } catch {
        /* ignore */
      }
      return;
    }
    const safeWidth = api.width;
    const safeHeight = api.height;
    set({ isRestoringLayout: true, currentLayoutSessionId: newSessionId });
    try {
      const ids = performSessionSwitch({
        api,
        oldSessionId,
        newSessionId,
        safeWidth,
        safeHeight,
        buildDefault: (a) => get().buildDefaultLayout(a),
      });
      set(ids);
    } finally {
      set({ isRestoringLayout: false });
    }
  },
  buildDefaultLayout: (api, intentName) => performBuildDefault(api, set, get, intentName),
  resetLayout: () => {
    const { api } = get();
    if (api) get().buildDefaultLayout(api);
  },
  pendingChatScrollTop: null,
  setPendingChatScrollTop: (value) => set({ pendingChatScrollTop: value }),
  ...buildPanelActions(set, get),
  ...buildExtraPanelActions(get),
}));

/** Perform a layout switch between sessions. */
export function performLayoutSwitch(oldSessionId: string | null, newSessionId: string): void {
  useDockviewStore.getState().switchSessionLayout(oldSessionId, newSessionId);
}
