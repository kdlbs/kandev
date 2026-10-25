import type { DockviewReadyEvent, SerializedDockview } from "dockview-react";
import type { StoreApi } from "zustand";
import {
  filterPreMaximizeLayout,
  hasRightColumn,
  useDockviewStore,
} from "@/lib/state/dockview-store";
import { applyLayoutFixups } from "@/lib/state/dockview-layout-builders";
import { measureDockviewContainer } from "@/lib/state/dockview-measure";
import type { LayoutGroup, LayoutNode, LayoutState } from "@/lib/state/layout-manager";
import {
  applyLayout,
  resolveGroupIds,
  setPinnedTarget,
  toSerializedDockview,
} from "@/lib/state/layout-manager";
import {
  filterLayoutStateByComponents,
  maximizedGroupIdOf,
  sanitizeSerializedLayout,
  serializedGridGroupIds,
} from "@/lib/state/layout-manager/sanitize-serialized-layout";
import type { AppState } from "@/lib/state/store";
import {
  getEnvLayout,
  getEnvMaximizeState,
  getManualRightWidth,
  removeEnvMaximizeState,
  setEnvLayout,
} from "@/lib/local-storage";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import {
  getRightPaneToggleState,
  readHiddenRightPane,
  stripHiddenRightPaneMetadata,
  withHiddenRightPaneMetadata,
} from "@/lib/state/dockview-right-pane";

const debug = createDebugLogger("dockview:restore");

type SavedMax = ReturnType<typeof getEnvMaximizeState>;

export type SessionRestoreContext = {
  activeSessionId: string | null;
  // null means the task session list has not completed hydration yet.
  currentSessionIds: string[] | null;
};

type SavedMaximizeOptions = {
  phantomSessionIds?: Set<string>;
  manualRightWidth?: number | null;
  sessionContext?: SessionRestoreContext;
};

function filterLayoutForRestore(
  state: LayoutState,
  validComponents: ReadonlySet<string>,
  phantomSessionIds?: Set<string>,
  sessionContext?: SessionRestoreContext,
): LayoutState {
  const preparedState =
    sessionContext?.activeSessionId === null || !sessionContext
      ? state
      : filterPreMaximizeLayout(
          state,
          sessionContext.activeSessionId,
          sessionContext.currentSessionIds,
        );
  if (!phantomSessionIds?.size)
    return filterLayoutStateByComponents(preparedState, validComponents);

  // Mark phantoms non-renderable so the shared filter prunes groups and repairs active panels.
  const filterGroup = (group: LayoutGroup): LayoutGroup => ({
    ...group,
    panels: group.panels.map((panel) =>
      panel.id.startsWith("session:") && phantomSessionIds.has(panel.id.slice("session:".length))
        ? { ...panel, component: "" }
        : panel,
    ),
  });
  const filterNode = (node: LayoutNode): LayoutNode =>
    node.type === "leaf"
      ? { ...node, group: filterGroup(node.group) }
      : { ...node, children: node.children.map(filterNode) };

  const markedState: LayoutState = {
    ...preparedState,
    columns: preparedState.columns.map((column) => ({
      ...column,
      groups: column.groups.map(filterGroup),
      ...(column.tree ? { tree: filterNode(column.tree) } : {}),
    })),
  };
  return filterLayoutStateByComponents(markedState, validComponents);
}

/**
 * Apply a saved maximize blob onto the live dockview api and mirror the full
 * maximize state into the store. Single source of truth for both restore
 * call sites — keeping `preMaximizeLayout` and `maximizedGroupId` in lockstep.
 *
 * Both stored structures are filtered first, because neither is sanitized on
 * the way in: an entry whose component is no longer registered throws inside
 * `api.fromJSON`. Returns false when the maximized group itself does not
 * survive, so the caller can fall back to the filtered pre-maximize layout
 * instead of restoring an overlay around a group the user no longer has.
 */
function applySavedMaximize(
  api: DockviewReadyEvent["api"],
  savedMax: NonNullable<SavedMax>,
  validComponents: ReadonlySet<string>,
  { phantomSessionIds, manualRightWidth, sessionContext }: SavedMaximizeOptions,
): boolean {
  const rawMaximized = savedMax.maximizedDockviewJson;
  const maximizedGroupId = maximizedGroupIdOf(rawMaximized);
  const sanitizedMaximized = sanitizeSerializedLayout(rawMaximized, validComponents, {
    excludeSessionIds: phantomSessionIds,
  });
  if (!sanitizedMaximized) {
    debug("applySavedMaximize: maximized payload unusable after sanitize");
    return false;
  }
  if (
    maximizedGroupId &&
    !serializedGridGroupIds(sanitizedMaximized.grid?.root).has(maximizedGroupId)
  ) {
    debug("applySavedMaximize: maximized group did not survive sanitize", { maximizedGroupId });
    return false;
  }
  const preMaximizeLayout = filterLayoutForRestore(
    savedMax.preMaximizeLayout as unknown as LayoutState,
    validComponents,
    phantomSessionIds,
    sessionContext,
  );
  api.fromJSON(sanitizedMaximized as SerializedDockview);
  const { width, height } = measureDockviewContainer(api);
  api.layout(width, height);
  const ids = applyLayoutFixups(api, undefined, manualRightWidth);
  // The maximize JSON is 2-column — captureRightTarget skips it (sv.length < 3).
  // Seed the right target directly so enforcePinnedTargets can snap the column
  // back to the saved width when the user exits maximize mode.
  if (manualRightWidth !== undefined && manualRightWidth !== null && manualRightWidth > 0) {
    setPinnedTarget("right", manualRightWidth);
  }
  useDockviewStore.setState({
    ...ids,
    preMaximizeLayout,
    maximizedGroupId: ids.centerGroupId,
    rightPanelsVisible: hasRightColumn(preMaximizeLayout),
  });
  return true;
}

/** Apply layout fixups, preferring a surviving maximize overlay over a
 *  filtered pre-maximize fallback. */
function applyFixupsWithMaximize(
  api: DockviewReadyEvent["api"],
  envId: string | null,
  validComponents: ReadonlySet<string>,
  phantomSessionIds?: Set<string>,
  sessionContext?: SessionRestoreContext,
): void {
  const manualRightWidth = getManualRightWidth(envId);
  const savedMax = envId ? getEnvMaximizeState(envId) : null;
  if (envId && savedMax) {
    if (
      applySavedMaximize(api, savedMax, validComponents, {
        phantomSessionIds,
        manualRightWidth,
        sessionContext,
      })
    ) {
      return;
    }
    applyPreMaximizeLayout(
      api,
      envId,
      filterLayoutForRestore(
        savedMax.preMaximizeLayout as unknown as LayoutState,
        validComponents,
        phantomSessionIds,
        sessionContext,
      ),
    );
    return;
  }
  const { width, height } = measureDockviewContainer(api);
  api.layout(width, height);
  // Anchor the right column to its per-env manual width when one exists;
  // serialized geometry otherwise remains responsive to the current viewport.
  const ids = applyLayoutFixups(api, undefined, manualRightWidth);
  useDockviewStore.setState(ids);
}

/** Apply and persist the filtered pre-maximize layout without maximize state. */
function applyPreMaximizeLayout(
  api: DockviewReadyEvent["api"],
  envId: string,
  preMaximizeLayout: LayoutState,
): void {
  const { width, height } = measureDockviewContainer(api);
  const manualRightWidth = getManualRightWidth(envId);
  const pinnedWidths =
    manualRightWidth === null ? new Map() : new Map([["right", manualRightWidth]]);
  const serialized = toSerializedDockview(preMaximizeLayout, width, height, pinnedWidths);
  applyLayout(api, preMaximizeLayout, pinnedWidths, width, height);
  const hiddenRightPane = readHiddenRightPane(getEnvLayout(envId));
  const paneState = getRightPaneToggleState(preMaximizeLayout, hiddenRightPane);
  useDockviewStore.setState({
    ...resolveGroupIds(api),
    preMaximizeLayout: null,
    maximizedGroupId: null,
    rightPanelsVisible: hasRightColumn(preMaximizeLayout),
    rightPaneVisible: paneState.visible,
    rightPaneAvailable: paneState.available,
    hiddenRightPane,
  });
  try {
    if (setEnvLayout(envId, withHiddenRightPaneMetadata(serialized, hiddenRightPane))) {
      removeEnvMaximizeState(envId);
    }
  } catch {
    // Keep the maximize snapshot until its replacement layout is durable.
  }
}

function tryRestoreMaximizeOnly(
  api: DockviewReadyEvent["api"],
  envId: string,
  validComponents: ReadonlySet<string>,
  phantomSessionIds?: Set<string>,
  sessionContext?: SessionRestoreContext,
): boolean {
  const savedMax = getEnvMaximizeState(envId);
  if (!savedMax) return false;
  try {
    if (
      applySavedMaximize(api, savedMax, validComponents, {
        phantomSessionIds,
        sessionContext,
      })
    ) {
      return true;
    }
    // This reader is reached only when there is no usable per-environment
    // layout, so falling through would end in the built-in default and discard
    // the panels the blob still holds. Apply what survived instead.
    applyPreMaximizeLayout(
      api,
      envId,
      filterLayoutForRestore(
        savedMax.preMaximizeLayout as unknown as LayoutState,
        validComponents,
        phantomSessionIds,
        sessionContext,
      ),
    );
    return true;
  } catch {
    // Drop the bad blob so subsequent page loads for this env don't keep
    // re-attempting the same failing fromJSON. Mirrors the self-heal in
    // dockview-store's restoreMaximizeFromStorage.
    removeEnvMaximizeState(envId);
    return false;
  }
}

/**
 * Restore the per-env saved layout, after sanitizing phantom session panels.
 * Returns true on a successful fromJSON, false when no usable saved layout
 * exists (caller falls through to maximize-only / global / default build).
 */
function tryRestoreEnvLayout(
  api: DockviewReadyEvent["api"],
  envId: string,
  validComponents: Set<string>,
  phantomSessionIds: Set<string> | undefined,
  sessionContext?: SessionRestoreContext,
): boolean {
  const envLayout = getEnvLayout(envId);
  if (!envLayout) {
    debug("tryRestoreEnvLayout: no saved layout for env", { envId });
    return false;
  }
  if (isDebug()) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const rawPanelIds = Object.keys((envLayout as any).panels ?? {});
    debug("tryRestoreEnvLayout: loaded saved layout", {
      envId,
      rawPanelCount: rawPanelIds.length,
      rawPanelIds,
      phantomSessionIds: phantomSessionIds ? Array.from(phantomSessionIds) : [],
    });
  }
  const sanitized = sanitizeSerializedLayout(
    stripHiddenRightPaneMetadata(envLayout),
    validComponents,
    {
      excludeSessionIds: phantomSessionIds,
    },
  );
  if (!sanitized) {
    debug("tryRestoreEnvLayout: sanitize returned null", { envId });
    return false;
  }
  if (isDebug()) {
    debug("tryRestoreEnvLayout: calling api.fromJSON", {
      envId,
      sanitizedPanelIds: Object.keys(sanitized.panels),
    });
  }
  api.fromJSON(sanitized as SerializedDockview);
  applyFixupsWithMaximize(api, envId, validComponents, phantomSessionIds, sessionContext);
  return true;
}

export function tryRestoreLayout(
  api: DockviewReadyEvent["api"],
  currentEnvId: string | null,
  validComponents: Set<string>,
  phantomSessionIds?: Set<string>,
  sessionContext?: SessionRestoreContext,
): boolean {
  // No env yet — the task is still preparing or its session→env mapping hasn't
  // hydrated. Return false so `onReady` builds the DEFAULT layout instead of
  // restoring a cross-env "last layout": the global layout key is shared across
  // tasks, so restoring it here would flash the *previous* task's proportions
  // while a fresh task prepares. The env's own saved layout is applied later by
  // `switchEnvLayout` once the env hydrates.
  if (!currentEnvId) return false;
  try {
    if (
      tryRestoreEnvLayout(api, currentEnvId, validComponents, phantomSessionIds, sessionContext)
    ) {
      return true;
    }
  } catch {
    // fall through to maximize-only
  }
  return tryRestoreMaximizeOnly(
    api,
    currentEnvId,
    validComponents,
    phantomSessionIds,
    sessionContext,
  );
}

/**
 * Collect session ids that DEFINITIVELY belong to a different env than `envId`.
 * These are phantoms (typically from a previously-deleted task) that must be
 * stripped on env-layout restore.
 *
 * Sessions absent from `environmentIdBySessionId` are NOT classified as
 * phantoms — they may be a still-loading WS arrival that legitimately belongs
 * to this env. `useAutoSessionTab`'s reconcile cleans up anything that turns
 * out to be stale once the store catches up.
 */
export function collectPhantomSessionIdsForEnv(
  state: { environmentIdBySessionId: Record<string, string> },
  envId: string,
): Set<string> {
  const result = new Set<string>();
  for (const [sessionId, mappedEnv] of Object.entries(state.environmentIdBySessionId)) {
    if (mappedEnv && mappedEnv !== envId) result.add(sessionId);
  }
  return result;
}

/**
 * Restore the env's saved layout, stripping session panels that we KNOW
 * belong to a different env — guards against phantom panels from
 * previously-deleted tasks resurfacing on restore.
 */
export function restoreEnvLayout(
  api: DockviewReadyEvent["api"],
  envId: string | null,
  appStore: StoreApi<AppState>,
  validComponents: Set<string>,
): boolean {
  const state = appStore.getState();
  const phantoms = envId ? collectPhantomSessionIdsForEnv(state, envId) : undefined;
  const { tasks, taskSessionsByTask } = state;
  const activeTaskId = tasks.activeTaskId;
  const sessionListLoaded = activeTaskId
    ? (taskSessionsByTask.loadedByTaskId?.[activeTaskId] ?? false)
    : false;
  const sessionContext: SessionRestoreContext = {
    activeSessionId: tasks.activeSessionId,
    currentSessionIds:
      activeTaskId && sessionListLoaded
        ? (taskSessionsByTask.itemsByTaskId[activeTaskId] ?? []).map((session) => session.id)
        : null,
  };
  if (isDebug()) {
    debug("restoreEnvLayout: entry", {
      envId,
      phantomCount: phantoms?.size ?? 0,
      phantomSessionIds: phantoms ? Array.from(phantoms) : [],
      livePanelIdsBefore: api.panels.map((p) => p.id),
    });
  }
  const result = tryRestoreLayout(api, envId, validComponents, phantoms, sessionContext);
  if (isDebug()) {
    debug("restoreEnvLayout: result", {
      envId,
      restored: result,
      livePanelIdsAfter: api.panels.map((p) => p.id),
    });
  }
  return result;
}
