/* eslint-disable max-lines -- zustand god-store; splitting is a separate refactor. */
import { create } from "zustand";
import type { DockviewApi, AddPanelOptions, SerializedDockview } from "dockview-react";
import {
  setSessionLayout,
  getSessionMaximizeState,
  setSessionMaximizeState,
  removeSessionMaximizeState,
} from "@/lib/local-storage";
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
import { performSessionSwitch } from "./dockview-session-switch";
import {
  injectIntentPanels,
  applyActivePanelOverrides,
  resolveNamedIntent,
} from "./layout-manager";
import { buildFileStateActions } from "./dockview-file-state";
import {
  buildPanelActions,
  buildExtraPanelActions,
  type OpenPanelOpts,
  type PreviewType,
} from "./dockview-panel-actions";
import { preserveChatScrollDuringLayout } from "./dockview-scroll-preserve";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";

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
  resolvedPath?: string;
  hasRemoteUpdate?: boolean;
  remoteContent?: string;
  remoteOriginalHash?: string;
  markdownPreview?: boolean;
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
  addFileDiffPanel: (
    path: string,
    opts?: OpenPanelOpts & { content?: string; groupId?: string },
  ) => void;
  addCommitDetailPanel: (sha: string, opts?: OpenPanelOpts & { groupId?: string }) => void;
  addFileEditorPanel: (path: string, name: string, opts?: OpenPanelOpts) => void;
  promotePreviewToPinned: (type: PreviewType) => void;
  addBrowserPanel: (url?: string, groupId?: string) => void;
  addVscodePanel: () => void;
  openInternalVscode: (goto_: { file: string; line: number; col: number } | null) => void;
  addPlanPanel: (opts?: { groupId?: string; quiet?: boolean; inCenter?: boolean }) => void;
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
  /** Saved layout from before a manual maximize. Null when not maximized. */
  preMaximizeLayout: LayoutState | null;
  /** The group ID that was maximized (used for session restore). */
  maximizedGroupId: string | null;
  maximizeGroup: (groupId: string) => void;
  exitMaximizedLayout: () => void;
  /** One-shot flag: skip the next layout switch for this specific session ID.
   *  Used when adding a panel within the same task to prevent a full rebuild. */
  _skipLayoutSwitchForSession: string | null;
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
  if (api.hasMaximizedGroup()) return;
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
  if (api.hasMaximizedGroup()) {
    api.exitMaximizedGroup();
  }
  syncPinnedWidthsFromApi(api, set);
  return useDockviewStore.getState().pinnedWidths;
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
      const safeWidth = api.width;
      const safeHeight = api.height;
      if (sidebarVisible) {
        const current = fromDockviewApi(api);
        const withoutSidebar: LayoutState = {
          columns: current.columns.filter((c) => c.id !== "sidebar"),
        };
        set({ isRestoringLayout: true, sidebarVisible: false });
        applyLayoutAndSet(api, withoutSidebar, liveWidths, set);
        requestAnimationFrame(() => {
          api.layout(safeWidth, safeHeight);
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
          api.layout(safeWidth, safeHeight);
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
      const safeWidth = api.width;
      const safeHeight = api.height;
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
          api.layout(safeWidth, safeHeight);
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
          api.layout(safeWidth, safeHeight);
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

/** Restore a saved maximize state from sessionStorage onto the dockview API. */
function restoreMaximizeFromStorage(api: DockviewApi, sessionId: string, set: StoreSet): boolean {
  const saved = getSessionMaximizeState(sessionId);
  if (!saved) return false;
  try {
    api.fromJSON(saved.maximizedDockviewJson as SerializedDockview);
    api.layout(api.width, api.height);
    const ids = applyLayoutFixups(api);
    const preMax = saved.preMaximizeLayout as unknown as LayoutState;
    set({ ...ids, preMaximizeLayout: preMax });
  } catch {
    return false;
  }
  requestAnimationFrame(() => {
    set({ isRestoringLayout: false });
  });
  return true;
}

/** Consume the one-shot skip flag. Returns true if the switch should be skipped. */
function consumeSkipFlag(set: StoreSet, get: StoreGet, newSessionId: string): boolean {
  const flag = get()._skipLayoutSwitchForSession;
  if (!flag) return false;
  set({ _skipLayoutSwitchForSession: null });
  if (flag === newSessionId) {
    set({ currentLayoutSessionId: newSessionId });
    return true;
  }
  return false;
}

/** Save the outgoing session's layout & maximize state, then release its portals. */
function saveOutgoingSession(
  api: DockviewApi,
  oldSessionId: string | null,
  preMaximizeLayout: LayoutState | null,
): void {
  if (!oldSessionId) return;
  if (preMaximizeLayout) {
    setSessionMaximizeState(oldSessionId, {
      preMaximizeLayout: preMaximizeLayout as unknown as object,
      maximizedDockviewJson: api.toJSON(),
    });
  } else {
    removeSessionMaximizeState(oldSessionId);
  }
  try {
    setSessionLayout(oldSessionId, api.toJSON());
  } catch {
    /* ignore */
  }
  panelPortalManager.releaseBySession(oldSessionId);
}

function buildSessionSwitchAction(set: StoreSet, get: StoreGet) {
  return (oldSessionId: string | null, newSessionId: string) => {
    const { api, currentLayoutSessionId, preMaximizeLayout } = get();
    if (!api) return;
    if (consumeSkipFlag(set, get, newSessionId)) return;
    if (currentLayoutSessionId === newSessionId) return;
    // First session adoption — onReady already built the layout; just adopt it.
    if (!oldSessionId && !currentLayoutSessionId) {
      set({ isRestoringLayout: true, currentLayoutSessionId: newSessionId });
      if (restoreMaximizeFromStorage(api, newSessionId, set)) return;
      set({ isRestoringLayout: false, currentLayoutSessionId: newSessionId });
      try {
        setSessionLayout(newSessionId, api.toJSON());
      } catch {
        /* ignore */
      }
      return;
    }
    // When oldSessionId is null but there is a live layout session (e.g. the
    // useSessionSwitchCleanup hook fires after passing through a null state),
    // fall back to currentLayoutSessionId so we correctly save and release the
    // outgoing session rather than silently skipping it.
    const effectiveOld = oldSessionId ?? currentLayoutSessionId;
    saveOutgoingSession(api, effectiveOld, preMaximizeLayout);
    set({ preMaximizeLayout: null, maximizedGroupId: null });
    set({ isRestoringLayout: true, currentLayoutSessionId: newSessionId });
    try {
      if (restoreMaximizeFromStorage(api, newSessionId, set)) return;
      const ids = performSessionSwitch({
        api,
        oldSessionId: effectiveOld,
        newSessionId,
        safeWidth: api.width,
        safeHeight: api.height,
        buildDefault: (a) => get().buildDefaultLayout(a),
        getDefaultLayout: () => get().userDefaultLayout ?? getPresetLayout("default"),
      });
      set(ids);
      set({ isRestoringLayout: false });
      panelPortalManager.reconcile(new Set(api.panels.map((p) => p.id)));
    } catch {
      set({ isRestoringLayout: false });
    }
  };
}

function buildMaximizeActions(set: StoreSet, get: StoreGet) {
  return {
    maximizeGroup: (groupId: string) => {
      const { api, preMaximizeLayout, currentLayoutSessionId } = get();
      if (!api) return;
      if (preMaximizeLayout) {
        get().exitMaximizedLayout();
        return;
      }
      const liveWidths = captureLiveWidths(api, set);
      preserveChatScrollDuringLayout();
      const current = fromDockviewApi(api);
      let targetGroup: {
        panels: LayoutState["columns"][0]["groups"][0]["panels"];
        activePanel?: string;
      } | null = null;
      for (const col of current.columns) {
        for (const g of col.groups) {
          if (g.id === groupId) {
            targetGroup = { panels: g.panels, activePanel: g.activePanel };
            break;
          }
        }
        if (targetGroup) break;
      }
      if (!targetGroup || targetGroup.panels.length === 0) return;
      const sidebarCol = current.columns.find((c) => c.id === "sidebar");
      const columns: LayoutState["columns"] = [];
      if (sidebarCol) columns.push(sidebarCol);
      columns.push({
        id: "maximized",
        groups: [{ panels: targetGroup.panels, activePanel: targetGroup.activePanel }],
      });
      const maximizedLayout: LayoutState = { columns };
      set({ isRestoringLayout: true, preMaximizeLayout: current, maximizedGroupId: groupId });
      const safeWidth = api.width;
      const safeHeight = api.height;
      applyLayoutAndSet(api, maximizedLayout, liveWidths, set);
      requestAnimationFrame(() => {
        api.layout(safeWidth, safeHeight);
        // Persist to sessionStorage so maximize survives page refresh
        if (currentLayoutSessionId) {
          setSessionMaximizeState(currentLayoutSessionId, {
            preMaximizeLayout: current as unknown as object,
            maximizedDockviewJson: api.toJSON(),
          });
        }
        set({ isRestoringLayout: false });
      });
    },
    exitMaximizedLayout: () => {
      const { api, preMaximizeLayout, currentLayoutSessionId } = get();
      if (!api || !preMaximizeLayout) return;
      preserveChatScrollDuringLayout();
      const safeWidth = api.width;
      const safeHeight = api.height;
      const liveWidths = get().pinnedWidths;
      set({ isRestoringLayout: true, preMaximizeLayout: null, maximizedGroupId: null });
      if (currentLayoutSessionId) {
        removeSessionMaximizeState(currentLayoutSessionId);
      }
      applyLayoutAndSet(api, preMaximizeLayout, liveWidths, set);
      requestAnimationFrame(() => {
        api.layout(safeWidth, safeHeight);
        syncPinnedWidthsFromApi(api, set);
        set({ isRestoringLayout: false });
      });
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
  // Capture dimensions before layout change — api.width can become stale
  // after fromJSON inside applyLayout
  const safeWidth = api.width;
  const safeHeight = api.height;
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
    api.layout(safeWidth, safeHeight);
    syncPinnedWidthsFromApi(api, set);
    set({ isRestoringLayout: false });
  });
}

export const useDockviewStore = create<DockviewStore>((set, get) => ({
  api: null,
  activeFilePath: null,
  setApi: (api) => {
    set({ api, activeFilePath: null });
    if (typeof window !== "undefined") {
      // Exposed for E2E tests to assert on panel/group placement. Harmless in
      // prod; the DockviewApi is already reachable via the store in devtools.
      (window as unknown as { __dockviewApi__: DockviewApi | null }).__dockviewApi__ = api;
    }
    if (api) {
      api.onDidActivePanelChange((event) => {
        const id = event?.id;
        if (id?.startsWith("file:")) {
          set({ activeFilePath: id.slice(5) });
        } else if (id === "preview:file-editor") {
          const path = (api.getPanel(id)?.params as Record<string, unknown> | undefined)?.path as
            | string
            | undefined;
          set({ activeFilePath: path ?? null });
        } else {
          set({ activeFilePath: null });
        }
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
  switchSessionLayout: buildSessionSwitchAction(set, get),
  buildDefaultLayout: (api, intentName) => performBuildDefault(api, set, get, intentName),
  resetLayout: () => {
    const { api } = get();
    if (api) get().buildDefaultLayout(api);
  },
  pendingChatScrollTop: null,
  setPendingChatScrollTop: (value) => set({ pendingChatScrollTop: value }),
  preMaximizeLayout: null,
  maximizedGroupId: null,
  _skipLayoutSwitchForSession: null,
  ...buildMaximizeActions(set, get),
  ...buildPanelActions(set, get),
  ...buildExtraPanelActions(get),
}));

/** Perform a layout switch between sessions. */
export function performLayoutSwitch(oldSessionId: string | null, newSessionId: string): void {
  useDockviewStore.getState().switchSessionLayout(oldSessionId, newSessionId);
}
