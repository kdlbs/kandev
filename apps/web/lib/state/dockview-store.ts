/* eslint-disable max-lines -- zustand god-store; splitting is a separate refactor. */
import { create } from "zustand";
import type { DockviewApi, AddPanelOptions, SerializedDockview } from "dockview-react";
import {
  setEnvLayout,
  getEnvMaximizeState,
  setEnvMaximizeState,
  removeEnvMaximizeState,
  clearGlobalSidebarWidth,
  setGlobalSidebarWidth,
} from "@/lib/local-storage";
import { setPinnedTarget, clearPinnedTarget } from "./layout-manager";
import { applyLayoutFixups, focusOrAddPanel } from "./dockview-layout-builders";
import {
  SIDEBAR_GROUP,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  TERMINAL_DEFAULT_ID,
  getPresetLayout,
  getPresetSidebarColumn,
  applyLayout,
  getPinnedWidth,
  getRootSplitview,
  fromDockviewApi,
  filterEphemeral,
  defaultLayout,
  mergeCurrentPanelsIntoPreset,
  toSerializedDockview,
} from "./layout-manager";
import type { BuiltInPreset, LayoutState, LayoutGroupIds } from "./layout-manager";
import { performEnvSwitch } from "./dockview-env-switch";
import { enforcePinnedTargets } from "./dockview-pinned-enforce";
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
import { measureDockviewContainer } from "./dockview-measure";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { snapshotColumnWidths, formatWidthsSnapshot } from "./dockview-widths-debug";

const debugSwitch = createDebugLogger("dockview:store-switch");
const debugSave = createDebugLogger("dockview:save");
const debugWidths = createDebugLogger("dockview:widths");

const RIGHT_PANEL_IDS = new Set(["changes", "files", TERMINAL_DEFAULT_ID]);

// Re-export types and constants used by other modules
export type { BuiltInPreset } from "./layout-manager";
export {
  LAYOUT_SIDEBAR_RATIO,
  LAYOUT_RIGHT_RATIO,
  computeSidebarMaxPx,
  computeRightMaxPx,
  LAYOUT_PINNED_MIN_PX,
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
    opts?: OpenPanelOpts & {
      content?: string;
      groupId?: string;
      source?: string;
      repositoryName?: string;
    },
  ) => void;
  addCommitDetailPanel: (
    sha: string,
    opts?: OpenPanelOpts & { groupId?: string; repo?: string },
  ) => void;
  addFileEditorPanel: (path: string, name: string, opts?: OpenPanelOpts) => void;
  promotePreviewToPinned: (type: PreviewType) => void;
  addBrowserPanel: (url?: string, groupId?: string) => void;
  addVscodePanel: () => void;
  openInternalVscode: (goto_: { file: string; line: number; col: number } | null) => void;
  addPlanPanel: (opts?: { groupId?: string; quiet?: boolean; inCenter?: boolean }) => void;
  /** Open a PR detail panel. prKey (owner/repo/pr_number) gives multi-repo tasks one tab per PR.
   *  activeSessionId anchors the new panel to the session's current group so it lands as a tab
   *  next to the session, not as a split. Falls back to centerGroupId when omitted. */
  addPRPanel: (prKey?: string, activeSessionId?: string | null) => void;
  addTerminalPanel: (
    terminalId?: string,
    groupId?: string,
    environmentId?: string,
    taskID?: string,
    title?: string,
  ) => void;
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
  applyBuiltInPreset: (preset: BuiltInPreset, resetWidths?: boolean) => void;
  defaultPreset: BuiltInPreset;
  setDefaultPreset: (preset: BuiltInPreset) => void;
  applyCustomLayout: (layout: SavedLayoutConfig) => void;
  captureCurrentLayout: () => Record<string, unknown>;
  isRestoringLayout: boolean;
  /** ID of the task environment whose layout is currently rendered. Layouts are
   *  keyed by env so sessions sharing an env reuse one layout. */
  currentLayoutEnvId: string | null;
  /** Switch the rendered layout to a new task environment. Same-env switches
   *  are a no-op (the layout already belongs to that env). `activeSessionId`
   *  is the session whose chat panel should be present in the new env. */
  switchEnvLayout: (
    oldEnvId: string | null,
    newEnvId: string,
    activeSessionId: string | null,
  ) => void;
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

/**
 * Build the pinnedWidths updates for a width sync, tracking only the VISIBLE
 * default right column.
 *
 * In plan/preview/vscode layouts the side column inherits merged files/changes
 * panels and `fromDockviewApi` mislabels it "right"; storing its width as the
 * right override would then leak into the default layout when toggling back
 * (e.g. plan-mode off snapping the right column to the plan column's width).
 *
 * Sidebar is intentionally excluded: its persisted width is a global pref
 * written only by explicit sash drag, and syncing live layout-change widths
 * would overwrite the raw pref with viewport-clamped transient widths.
 * Widths <= 50px are treated as transient/collapsed and skipped.
 */
export function collectPinnedWidthUpdates(
  columns: { id: string }[],
  getSize: (index: number) => number,
  visibility: { rightPanelsVisible: boolean },
): Map<string, number> {
  const updates = new Map<string, number>();
  columns.forEach((col, i) => {
    if (col.id !== "right" || !visibility.rightPanelsVisible) return;
    const w = getSize(i);
    if (w > 50) updates.set(col.id, w);
  });
  return updates;
}

/** Read live column widths from dockview's splitview and persist the visible
 *  right width as a pinned override (see `collectPinnedWidthUpdates`). */
function syncPinnedWidthsFromApi(api: DockviewApi, set: StoreSet): void {
  if (api.hasMaximizedGroup()) return;
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  try {
    const state = fromDockviewApi(api);
    if (state.columns.length !== sv.length) return;
    const { rightPanelsVisible } = useDockviewStore.getState();
    const updates = collectPinnedWidthUpdates(state.columns, (i) => sv.getViewSize(i), {
      rightPanelsVisible,
    });
    if (updates.size > 0) {
      if (isDebug()) {
        const pairs = Array.from(updates.entries())
          .map(([k, v]) => `${k}=${Math.round(v)}`)
          .join(",");
        debugWidths(`store-sync ${pairs} ${formatWidthsSnapshot(snapshotColumnWidths(api))}`);
      }
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

/**
 * Decide which pinned-width overrides to apply when switching to a preset.
 *
 * - `resetWidths` (explicit layout pick from the selector): return each pinned
 *   column's computed DEFAULT width (ratio clamped to its initial cap). These
 *   are passed as explicit overrides — NOT an empty map — so `applyLayout`'s
 *   resize-to-target path is used. An empty map makes it read the
 *   post-`fromJSON` live size instead, which is fragile: dockview can lay
 *   `fromJSON` out at a transient narrower width, and that shrunken size would
 *   be captured as the pinned target and then enforced, leaving the columns
 *   too narrow.
 * - otherwise (programmatic switch, e.g. plan-mode toggle): keep the live right
 *   width, minus overrides for columns absent in the target layout. Sidebar is
 *   always resolved from the global pref/default instead of an in-memory
 *   override.
 */
export function resolvePresetPinnedWidths(
  liveWidths: Map<string, number>,
  columns: LayoutState["columns"],
  totalWidth: number,
  resetWidths: boolean,
): Map<string, number> {
  if (resetWidths) {
    // "Default layout" is the reset gesture: drop any custom global sidebar
    // width (and its runtime target) so getPinnedWidth returns the fresh
    // ratio default for the current screen instead of re-reading the pref.
    clearGlobalSidebarWidth();
    clearPinnedTarget("sidebar");
    const defaults = new Map<string, number>();
    for (const col of columns) {
      if (col.pinned) defaults.set(col.id, getPinnedWidth(col, totalWidth, undefined));
    }
    return defaults;
  }
  const targetColumnIds = new Set(columns.map((c) => c.id));
  const cleaned = new Map(liveWidths);
  for (const key of cleaned.keys()) {
    if (key === "sidebar" || !targetColumnIds.has(key)) cleaned.delete(key);
  }
  return cleaned;
}

/** Capture the live right pixel width into pinnedWidths before a layout rebuild. */
function captureLiveWidths(api: DockviewApi, set: StoreSet): Map<string, number> {
  if (api.hasMaximizedGroup()) {
    api.exitMaximizedGroup();
  }
  syncPinnedWidthsFromApi(api, set);
  return useDockviewStore.getState().pinnedWidths;
}

/**
 * Snap pinned columns to their targets using the current store state.
 *
 * Called inside every programmatic layout path's post-`api.layout` rAF
 * before flipping `isRestoringLayout` false - dockview's proportional
 * rebalance can grow pinned columns up to their loose `setConstraints` max,
 * and the reactive enforcement (wired via `onDidLayoutChange`) is gated by
 * `isRestoringLayout`, so without this synchronous call the correction
 * would only land on the next user-triggered layout-change event,
 * producing a visible jerk once env prep settles.
 */
function enforceFromStore(api: DockviewApi, get: StoreGet): void {
  const s = get();
  enforcePinnedTargets(api, {
    sidebarVisible: s.sidebarVisible,
    rightPanelsVisible: s.rightPanelsVisible,
    maximized: s.preMaximizeLayout !== null,
  });
}

function applyLayoutAndSet(
  api: DockviewApi,
  state: LayoutState,
  pinnedWidths: Map<string, number>,
  set: StoreSet,
  preMeasured?: { width: number; height: number },
): LayoutGroupIds {
  // Pass measured container dims so fromJSON's grid.width matches the live
  // container — avoids the proportional rescale that would otherwise grow
  // pinned columns past their legacy initial caps on the next api.layout.
  //
  // Callers can pre-measure and pass `preMeasured` to avoid re-measuring inside
  // the middle of a layout transition. The visibility toggles below take that
  // path because `set({ sidebarVisible: ... })` runs before this call and can
  // trigger React to repaint the host shell, momentarily shrinking the
  // dockview parent's `clientWidth` — re-measuring then would read the
  // transient narrow width and clamp pinned columns to it (the
  // `pane-resize-sidebar.spec.ts:41` flake mode: cap=301 from a 601px stale
  // measurement clamps the 430px sidebar override down to 301).
  const measured = preMeasured ?? measureDockviewContainer(api);
  const ids = applyLayout(api, state, pinnedWidths, measured.width, measured.height);
  set(ids);
  return ids;
}

function removeRightPanelTabs(state: LayoutState): LayoutState {
  const columns = state.columns
    .map((col) => {
      const groups = col.groups
        .map((group) => {
          const panels = group.panels.filter((panel) => !RIGHT_PANEL_IDS.has(panel.id));
          if (panels.length === group.panels.length) return group;
          const activePanel = panels.some((panel) => panel.id === group.activePanel)
            ? group.activePanel
            : panels[0]?.id;
          return { ...group, panels, activePanel };
        })
        .filter((group) => group.panels.length > 0);
      return { ...col, groups };
    })
    .filter((col) => col.groups.length > 0);
  return { columns };
}

function buildVisibilityActions(set: StoreSet, get: StoreGet) {
  return {
    toggleSidebar: () => {
      const { api, sidebarVisible } = get();
      if (!api) return;
      const liveWidths = captureLiveWidths(api, set);
      preserveChatScrollDuringLayout();
      const { width: safeWidth, height: safeHeight } = measureDockviewContainer(api);
      const preMeasured = { width: safeWidth, height: safeHeight };
      if (sidebarVisible) {
        const current = fromDockviewApi(api);
        const withoutSidebar: LayoutState = {
          columns: current.columns.filter((c) => c.id !== "sidebar"),
        };
        set({ isRestoringLayout: true, sidebarVisible: false });
        applyLayoutAndSet(api, withoutSidebar, liveWidths, set, preMeasured);
        requestAnimationFrame(() => {
          api.layout(safeWidth, safeHeight);
          enforceFromStore(api, get);
          syncPinnedWidthsFromApi(api, set);
          set({ isRestoringLayout: false });
        });
      } else {
        const current = fromDockviewApi(api);
        const sidebarCol = getPresetSidebarColumn(get().defaultPreset);
        const withSidebar: LayoutState = {
          columns: [sidebarCol, ...current.columns],
        };
        set({ isRestoringLayout: true, sidebarVisible: true });
        applyLayoutAndSet(api, withSidebar, liveWidths, set, preMeasured);
        requestAnimationFrame(() => {
          api.layout(safeWidth, safeHeight);
          enforceFromStore(api, get);
          syncPinnedWidthsFromApi(api, set);
          set({ isRestoringLayout: false });
        });
      }
    },
    toggleRightPanels: () => {
      const { api, rightPanelsVisible, defaultPreset } = get();
      if (!api) return;
      if (!rightPanelsVisible && defaultPreset === "compact") return;
      const liveWidths = captureLiveWidths(api, set);
      preserveChatScrollDuringLayout();
      const { width: safeWidth, height: safeHeight } = measureDockviewContainer(api);
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
          enforceFromStore(api, get);
          syncPinnedWidthsFromApi(api, set);
          set({ isRestoringLayout: false });
        });
      } else {
        const defLayout = defaultLayout();
        const rightCol = defLayout.columns.find((c) => c.id === "right");
        if (!rightCol) return;
        const current = removeRightPanelTabs(fromDockviewApi(api));
        const withRight: LayoutState = {
          columns: [...current.columns, rightCol],
        };
        set({ isRestoringLayout: true, rightPanelsVisible: true });
        applyLayoutAndSet(api, withRight, liveWidths, set);
        requestAnimationFrame(() => {
          api.layout(safeWidth, safeHeight);
          enforceFromStore(api, get);
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
    applyBuiltInPreset: (preset: BuiltInPreset, resetWidths = false) => {
      const { api } = get();
      if (!api) return;
      const liveWidths = captureLiveWidths(api, set);
      preserveChatScrollDuringLayout();
      // Capture dimensions before layout change — api.width can become stale
      // inside the rAF callback after dockview serialization
      const { width: safeWidth, height: safeHeight } = measureDockviewContainer(api);
      set({ isRestoringLayout: true });
      const presetState = getPresetLayout(preset);
      const state = mergeCurrentPanelsIntoPreset(api, presetState);
      // resetWidths (explicit pick from the layout selector) → preset defaults;
      // otherwise carry live widths minus columns absent in the target layout.
      const cleanedWidths = resolvePresetPinnedWidths(
        liveWidths,
        state.columns,
        safeWidth,
        resetWidths,
      );
      const ids = applyLayout(api, state, cleanedWidths, safeWidth, safeHeight);
      if (isDebug()) {
        const applied =
          [...cleanedWidths].map(([k, v]) => `${k}:${Math.round(v)}`).join(",") || "-";
        debugWidths(
          `preset-apply preset=${preset} reset=${resetWidths} safeW=${safeWidth} ` +
            `applied=${applied} postApply=${formatWidthsSnapshot(snapshotColumnWidths(api))}`,
        );
      }
      set({
        ...ids,
        sidebarVisible: true,
        rightPanelsVisible: preset === "default",
        pinnedWidths: cleanedWidths,
      });
      requestAnimationFrame(() => {
        api.layout(safeWidth, safeHeight);
        if (isDebug()) {
          debugWidths(
            `preset-post-layout preset=${preset} ${formatWidthsSnapshot(snapshotColumnWidths(api))}`,
          );
        }
        enforceFromStore(api, get);
        syncPinnedWidthsFromApi(api, set);
        set({ isRestoringLayout: false });
      });
    },
    applyCustomLayout: (layout: SavedLayoutConfig) => {
      const { api } = get();
      if (!api) return;
      const liveWidths = captureLiveWidths(api, set);
      preserveChatScrollDuringLayout();
      const { width: safeWidth, height: safeHeight } = measureDockviewContainer(api);
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
        const ids = applyLayout(api, state, liveWidths, safeWidth, safeHeight);
        set(ids);
      }
      const hasSidebar = !!api.getPanel("sidebar");
      const colCount = state?.columns?.length ?? api.groups.length;
      const sidebarCols = hasSidebar ? 1 : 0;
      const hasRight = colCount > sidebarCols + 1;
      set({ sidebarVisible: hasSidebar, rightPanelsVisible: hasRight });
      requestAnimationFrame(() => {
        api.layout(safeWidth, safeHeight);
        enforceFromStore(api, get);
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
function restoreMaximizeFromStorage(api: DockviewApi, envId: string, set: StoreSet): boolean {
  const saved = getEnvMaximizeState(envId);
  if (!saved) return false;
  try {
    api.fromJSON(saved.maximizedDockviewJson as SerializedDockview);
    // After fromJSON, `api.width/height` reflect the JSON's recorded grid
    // dims, which may not match the live container. Always lay out against
    // the measured DOM size so a stale value can't pin the dockview at the
    // wrong width on subsequent restores.
    const { width, height } = measureDockviewContainer(api);
    api.layout(width, height);
    const ids = applyLayoutFixups(api);
    const preMax = saved.preMaximizeLayout as unknown as LayoutState;
    // The maximized layout is `[sidebar?, maximized]` — the non-sidebar group
    // is the one being maximized, which `resolveGroupIds` returns as
    // `centerGroupId`. Tracking it keeps the store consistent with what
    // `maximizeGroup` would have set (so toggle/exit logic doesn't operate on
    // a half-restored maximize).
    set({ ...ids, preMaximizeLayout: preMax, maximizedGroupId: ids.centerGroupId });
  } catch {
    // Drop the bad blob so the next switch/reload doesn't keep reattempting
    // the same failing fromJSON before falling back. Self-healing.
    removeEnvMaximizeState(envId);
    return false;
  }
  requestAnimationFrame(() => {
    set({ isRestoringLayout: false });
  });
  return true;
}

/** Save the outgoing env's layout & maximize state, then release its portals. */
function saveOutgoingEnv(
  api: DockviewApi,
  oldEnvId: string | null,
  preMaximizeLayout: LayoutState | null,
  pinnedWidths: Map<string, number>,
): void {
  if (!oldEnvId) {
    debugSave("saveOutgoingEnv: skip (no oldEnvId)");
    return;
  }
  if (isDebug()) {
    debugSave("saveOutgoingEnv: entry", {
      oldEnvId,
      livePanelIds: api.panels.map((p) => p.id),
      maximized: !!preMaximizeLayout,
    });
  }
  if (preMaximizeLayout) {
    // While maximized, `api.toJSON()` is the 2-column maximize overlay, NOT
    // the user's intended layout. Persist the pre-max layout under both keys:
    //  - max state: maximizedDockviewJson is what the user sees (the overlay);
    //  - env layout: pre-max serialized so a reload that misses the max state
    //    (e.g. cleared maximize) falls back to the user's real layout, not a
    //    truncated 2-column slice.
    // Wrapped in try/catch so a serialization throw can't skip releaseByEnv at
    // the bottom (which would re-leak env-scoped portals).
    try {
      setEnvMaximizeState(oldEnvId, {
        preMaximizeLayout: preMaximizeLayout as unknown as object,
        maximizedDockviewJson: api.toJSON(),
      });
    } catch (err) {
      removeEnvMaximizeState(oldEnvId);
      console.warn("saveOutgoingEnv: failed to persist maximize state", err);
    }
    try {
      // Use measured container size — `api.width/height` can be drifted from
      // the live container, and serializing with stale dims would persist a
      // shrunken layout that resurfaces on the next reload.
      const { width, height } = measureDockviewContainer(api);
      const preMaxSerialized = toSerializedDockview(preMaximizeLayout, width, height, pinnedWidths);
      setEnvLayout(oldEnvId, preMaxSerialized as unknown as object);
    } catch (err) {
      console.warn("saveOutgoingEnv: serialize failed", err);
      /* fall back: skip writing rather than overwrite with maximized JSON */
    }
  } else {
    removeEnvMaximizeState(oldEnvId);
    try {
      setEnvLayout(oldEnvId, api.toJSON());
    } catch {
      /* ignore */
    }
  }
  panelPortalManager.releaseByEnv(oldEnvId);
}

function buildEnvSwitchAction(set: StoreSet, get: StoreGet) {
  return (oldEnvId: string | null, newEnvId: string, activeSessionId: string | null) => {
    const { api, currentLayoutEnvId, preMaximizeLayout } = get();
    if (!api) {
      debugSwitch("envSwitch: skip (no api)", { oldEnvId, newEnvId, activeSessionId });
      return;
    }
    if (isDebug()) {
      debugSwitch("envSwitch: entry", {
        oldEnvId,
        newEnvId,
        activeSessionId,
        currentLayoutEnvId,
        maximized: !!preMaximizeLayout,
        livePanelIds: api.panels.map((p) => p.id),
      });
    }
    // Same-env switch (e.g. between sessions of the same task) is a no-op.
    // The layout, terminals, and env-scoped portals already belong to this env.
    if (currentLayoutEnvId === newEnvId) {
      debugSwitch("envSwitch: skip (same env)", { newEnvId });
      return;
    }
    // First adoption (oldEnvId and currentLayoutEnvId both null) falls through
    // to the general path below. We deliberately do NOT "just adopt" whatever
    // onReady rendered: this branch only fires when onReady ran with a null
    // env (otherwise currentLayoutEnvId would equal newEnvId and we'd have
    // skipped above as same-env), so onReady built the cross-env GLOBAL
    // FALLBACK layout — not this env's saved/default layout. Adopting it gave a
    // fresh task the previous env's stale proportions instead of the defaults,
    // and `setEnvLayout(newEnvId, api.toJSON())` even overwrote the env's real
    // saved layout with that stale one. `performEnvSwitch` instead restores the
    // env's saved layout (or builds defaults for a brand-new task), and
    // `saveOutgoingEnv(null)` below is a no-op so there is nothing to lose.
    //
    // When oldEnvId is null but there IS a live layout env (the
    // useEnvSwitchCleanup hook firing after passing through a null state),
    // fall back to currentLayoutEnvId so we correctly save and release the
    // outgoing env rather than silently skipping it.
    const effectiveOld = oldEnvId ?? currentLayoutEnvId;
    saveOutgoingEnv(api, effectiveOld, preMaximizeLayout, get().pinnedWidths);
    set({ preMaximizeLayout: null, maximizedGroupId: null });
    set({ isRestoringLayout: true, currentLayoutEnvId: newEnvId });
    try {
      if (restoreMaximizeFromStorage(api, newEnvId, set)) return;
      const measured = measureDockviewContainer(api);
      const ids = performEnvSwitch({
        api,
        oldEnvId: effectiveOld,
        newEnvId,
        activeSessionId,
        safeWidth: measured.width,
        safeHeight: measured.height,
        buildDefault: (a) => get().buildDefaultLayout(a),
        getDefaultLayout: () => get().userDefaultLayout ?? getPresetLayout(get().defaultPreset),
      });
      set(ids);
      enforceFromStore(api, get);
      set({ isRestoringLayout: false });
      if (isDebug()) {
        debugWidths(
          `env-switch-done old=${effectiveOld ?? "-"} new=${newEnvId} ` +
            `${formatWidthsSnapshot(snapshotColumnWidths(api))}`,
        );
      }
      panelPortalManager.reconcile(new Set(api.panels.map((p) => p.id)));
    } catch {
      set({ isRestoringLayout: false });
    }
  };
}

function buildMaximizeActions(set: StoreSet, get: StoreGet) {
  return {
    maximizeGroup: (groupId: string) => {
      const { api, preMaximizeLayout, currentLayoutEnvId } = get();
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
      const { width: safeWidth, height: safeHeight } = measureDockviewContainer(api);
      applyLayoutAndSet(api, maximizedLayout, liveWidths, set);
      requestAnimationFrame(() => {
        api.layout(safeWidth, safeHeight);
        if (currentLayoutEnvId) {
          setEnvMaximizeState(currentLayoutEnvId, {
            preMaximizeLayout: current as unknown as object,
            maximizedDockviewJson: api.toJSON(),
          });
        }
        set({ isRestoringLayout: false });
      });
    },
    exitMaximizedLayout: () => {
      const { api, preMaximizeLayout, currentLayoutEnvId } = get();
      if (!api || !preMaximizeLayout) return;
      preserveChatScrollDuringLayout();
      const measured = measureDockviewContainer(api);
      const safeWidth = measured.width;
      const safeHeight = measured.height;
      const liveWidths = get().pinnedWidths;
      set({ isRestoringLayout: true, preMaximizeLayout: null, maximizedGroupId: null });
      if (currentLayoutEnvId) {
        removeEnvMaximizeState(currentLayoutEnvId);
      }
      applyLayoutAndSet(api, preMaximizeLayout, liveWidths, set);
      requestAnimationFrame(() => {
        api.layout(safeWidth, safeHeight);
        enforceFromStore(api, get);
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
  const { width: safeWidth, height: safeHeight } = measureDockviewContainer(api);
  if (isDebug()) {
    debugWidths(
      `build-default-entry intent=${intentName ?? "-"} ` +
        `measured=${safeWidth}x${safeHeight} ` +
        `pre=${formatWidthsSnapshot(snapshotColumnWidths(api))}`,
    );
  }
  set({ isRestoringLayout: true, pinnedWidths: freshPinned });

  const basePreset = intent?.preset as BuiltInPreset | undefined;
  let state = basePreset
    ? getPresetLayout(basePreset)
    : (userDefaultLayout ?? getPresetLayout(get().defaultPreset));

  if (intent?.panels?.length) {
    state = injectIntentPanels(state, intent.panels);
  }
  if (intent?.activePanels) {
    state = applyActivePanelOverrides(state, intent.activePanels);
  }

  const ids = applyLayout(api, state, freshPinned, safeWidth, safeHeight);
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
    enforceFromStore(api, get);
    syncPinnedWidthsFromApi(api, set);
    if (isDebug()) {
      debugWidths(`build-default-done ${formatWidthsSnapshot(snapshotColumnWidths(api))}`);
    }
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
      type TestWindow = {
        __dockviewApi__: DockviewApi | null;
        __setPinnedTarget__?: typeof setPinnedTarget;
        __setGlobalSidebarWidth__?: typeof setGlobalSidebarWidth;
      };
      const w = window as unknown as TestWindow;
      w.__dockviewApi__ = api;
      // E2E test helpers: let `resizeColumnViaSplitview` update the target
      // width after a programmatic resize (mirroring the sash-drag mouseup),
      // including persisting the global sidebar-width pref like a real drag.
      w.__setPinnedTarget__ = setPinnedTarget;
      w.__setGlobalSidebarWidth__ = setGlobalSidebarWidth;
    }
    if (api) {
      const resolveFilePath = (panelId: string | undefined): string | null => {
        if (!panelId) return null;
        if (panelId.startsWith("file:")) return panelId.slice(5);
        if (panelId.startsWith("diff:file:")) return panelId.slice("diff:file:".length);
        if (panelId === "preview:file-editor" || panelId === "preview:file-diff") {
          const path = (api.getPanel(panelId)?.params as Record<string, unknown> | undefined)
            ?.path as string | undefined;
          return path ?? null;
        }
        return null;
      };
      api.onDidActivePanelChange((event) => {
        set({ activeFilePath: resolveFilePath(event?.id) });
      });
      // Track per-panel param-change subscriptions so they can be disposed when
      // the panel is removed (e.g. across env switches that re-create the
      // preview panel) instead of relying on dockview's internal cleanup.
      const paramSubs = new Map<string, { dispose: () => void }>();
      api.onDidAddPanel((panel) => {
        // The preview file-editor panel reuses a single dockview panel and swaps
        // its `params.path` via `updateParameters` when the user previews a
        // different file. Dockview does not refire `onDidActivePanelChange` for
        // params-only updates on an already-active panel, so subscribe to the
        // panel's own parameter-change event and refresh `activeFilePath`.
        if (panel.id !== "preview:file-editor" && panel.id !== "preview:file-diff") return;
        paramSubs.get(panel.id)?.dispose();
        const sub = panel.api.onDidParametersChange(() => {
          if (!panel.api.isActive) return;
          set({ activeFilePath: resolveFilePath(panel.id) });
        });
        paramSubs.set(panel.id, sub);
      });
      api.onDidRemovePanel((panel) => {
        const sub = paramSubs.get(panel.id);
        if (sub) {
          sub.dispose();
          paramSubs.delete(panel.id);
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
  defaultPreset: "default",
  setDefaultPreset: (preset) => set({ defaultPreset: preset }),
  isRestoringLayout: false,
  currentLayoutEnvId: null,
  deferredPanelActions: [],
  queuePanelAction: (action) =>
    set((prev) => ({
      deferredPanelActions: [...prev.deferredPanelActions, action],
    })),
  switchEnvLayout: buildEnvSwitchAction(set, get),
  buildDefaultLayout: (api, intentName) => performBuildDefault(api, set, get, intentName),
  resetLayout: () => {
    const { api } = get();
    if (api) get().buildDefaultLayout(api);
  },
  pendingChatScrollTop: null,
  setPendingChatScrollTop: (value) => set({ pendingChatScrollTop: value }),
  preMaximizeLayout: null,
  maximizedGroupId: null,
  ...buildMaximizeActions(set, get),
  ...buildPanelActions(set, get),
  ...buildExtraPanelActions(get),
}));

/**
 * Perform a layout switch between task environments. Same-env (e.g. between
 * sessions of the same task) is a no-op — terminals + layout stay put.
 *
 * `activeSessionId` is the session whose chat panel should be present in the
 * resulting layout. It can differ across sessions of the same env, but layout
 * reuse means we just ensure the right session: chat panel is visible.
 */
export function performLayoutSwitch(
  oldEnvId: string | null,
  newEnvId: string,
  activeSessionId: string | null,
): void {
  useDockviewStore.getState().switchEnvLayout(oldEnvId, newEnvId, activeSessionId);
}

/**
 * Release the dockview to a clean default layout — used when selecting a task
 * that has no session (and prepare failed to launch one). Without this the
 * dockview keeps the outgoing env's panels live but disconnected from any
 * active session, and the corrupted state can be persisted on the next save.
 */
export function releaseLayoutToDefault(oldEnvId: string | null): void {
  const { api, currentLayoutEnvId, preMaximizeLayout, buildDefaultLayout, pinnedWidths } =
    useDockviewStore.getState();
  if (!api) return;
  const effectiveOld = oldEnvId ?? currentLayoutEnvId;
  saveOutgoingEnv(api, effectiveOld, preMaximizeLayout, pinnedWidths);
  useDockviewStore.setState({
    preMaximizeLayout: null,
    maximizedGroupId: null,
    currentLayoutEnvId: null,
  });
  buildDefaultLayout(api);
}
