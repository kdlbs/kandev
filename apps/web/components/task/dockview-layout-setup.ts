import type { DockviewReadyEvent } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getRootSplitview } from "@/lib/state/dockview-layout-builders";
import { computeRightMaxPx, getPinnedTarget, setPinnedTarget } from "@/lib/state/layout-manager";
import {
  getManualRightWidth,
  setEnvLayout,
  setEnvLayoutProfile,
  setManualRightWidth,
} from "@/lib/local-storage";
import {
  resolveEnvRightWidth,
  resolveResponsiveRightWidth,
} from "@/lib/state/layout-manager/right-width";
import {
  getReusableRightWidth,
  clearReusableRootGeometry,
} from "@/lib/state/layout-manager/pinned-targets";
import {
  restoreReusableRootGeometry,
  validateRetainedRootGeometry,
} from "@/lib/state/layout-manager/reusable-root-geometry";
import {
  getPinnedRightColumn,
  type PinnedRightColumn,
} from "@/lib/state/layout-manager/pinned-right-column";
import { refreshPinnedRightConstraints } from "@/lib/state/layout-manager/pinned-right-constraints";
import { getRootSidebarWidth } from "@/lib/state/layout-manager/applier";
import { setSashDragging as setPinnedEnforcementSashDragging } from "@/lib/state/dockview-pinned-enforce";
import { withHiddenRightPaneMetadata } from "@/lib/state/dockview-right-pane";
import { getDockviewElement, measureDockviewGridWidth } from "@/lib/state/dockview-measure";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { stopVscode } from "@/lib/api/domains/vscode-api";
import { parkUserShell, stopUserShell } from "@/lib/api/domains/user-shell-api";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import {
  snapshotColumnWidths,
  formatWidthsSnapshot,
  formatJsonRootSizes,
} from "@/lib/state/dockview-widths-debug";

const debugWidths = createDebugLogger("dockview:widths");
// v3: bumped alongside DOCKVIEW_ENV_LAYOUT_PREFIX so the no-env fallback
// also discards layouts captured with the now-removed dockview sidebar column.
const LAYOUT_STORAGE_KEY = "dockview-layout-v3";
const terminalTerminateClosePanelIds = new Set<string>();

/** Record a terminal panel whose close should skip the park-on-close handling
 *  (the caller terminates the terminal explicitly). */
export function markTerminalPanelTerminateClose(panelId: string): void {
  terminalTerminateClosePanelIds.add(panelId);
}

/**
 * Pinned-column target enforcement.
 *
 * Dockview's splitview rebalances proportionally on any `api.layout` call,
 * which would otherwise grow pinned columns past their initial defaults on
 * container expansion and shrink them on container contraction. Manual widths
 * remain pixel targets; automatic widths are resolved against the current
 * container and reusable profile ratio. After each layout change we enforce
 * those targets via `sv.resizeView`.
 */

/** Enforcement-in-progress guard to prevent infinite loops when our own
 *  `sv.resizeView` triggers `onDidLayoutChange`. */
let enforcing = false;

/** True while the user is actively dragging a `.dv-sash`. We pause target
 *  enforcement during the drag so the in-progress resize doesn't get
 *  reverted to the previous target on every intermediate layout change. */
let sashDragging = false;
let rightSashDragging = false;

/** Recompute the automatic right-column pinned target from the measured grid
 *  width and sidebar visibility; no-op when the right column is hidden or the
 *  user has set a manual right width. */
function refreshAutomaticRightTarget(api: DockviewReadyEvent["api"], width?: number): void {
  const store = useDockviewStore.getState();
  if (!getPinnedRightColumn(api) || getManualRightWidth(store.currentLayoutEnvId) !== null) return;
  const sidebarWidth = getRootSidebarWidth(api);
  setPinnedTarget(
    "right",
    resolveEnvRightWidth(
      width ?? measureDockviewGridWidth(api),
      sidebarWidth,
      null,
      store.currentLayoutEnvId,
    ),
  );
}

/** Only the root sashes immediately before or after the pinned right child own its width. */
function isPinnedRightSash(api: DockviewReadyEvent["api"], target: EventTarget | null): boolean {
  const right = getPinnedRightColumn(api);
  if (!right) return false;
  const sash = (target as HTMLElement | null)?.closest(".dv-sash");
  if (!sash) return false;
  const sv = getRootSplitview(api);
  const rootSashes = sv?.sashes as Array<{ container?: HTMLElement }> | undefined;
  return (
    (right.index > 0 && rootSashes?.[right.index - 1]?.container === sash) ||
    (right.index < sv.length - 1 && rootSashes?.[right.index]?.container === sash)
  );
}

type RootDrag = { envId: string | null; sizes: number[] };

function captureRootDrag(
  api: DockviewReadyEvent["api"],
  target: EventTarget | null,
): RootDrag | null {
  const sv = getRootSplitview(api);
  const sash = (target as HTMLElement | null)?.closest(".dv-sash");
  if (!sash || !sv?.sashes?.some((item: { container: HTMLElement }) => item.container === sash))
    return null;
  return {
    envId: useDockviewStore.getState().currentLayoutEnvId,
    sizes: Array.from({ length: sv.length }, (_, index) => sv.getViewSize(index)),
  };
}

function releaseEditedRootGeometry(api: DockviewReadyEvent["api"], drag: RootDrag | null): void {
  if (!drag) return;
  const sv = getRootSplitview(api);
  if (
    !sv ||
    drag.sizes.length !== sv.length ||
    drag.sizes.some((size, index) => Math.abs(size - sv.getViewSize(index)) >= 1)
  ) {
    clearReusableRootGeometry(drag.envId);
  }
}

/** Resize splitview view `idx` back to `target` (clamped to `maximumWidth`)
 *  when it differs by more than 1px, ignoring dockview rejections of
 *  unreachable sizes. */
function restoreColumnToTarget(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  sv: any,
  idx: number,
  target: number | undefined,
  maximumWidth: number,
): void {
  if (target === undefined) return;
  const reachableTarget = Math.min(target, maximumWidth);
  const cur = sv.getViewSize(idx);
  if (Math.abs(cur - reachableTarget) <= 1) return;
  try {
    sv.resizeView(idx, reachableTarget);
  } catch {
    /* dockview rejects unreachable sizes — ignore */
  }
}

/** Keep right-column constraints tied to Dockview's measured container, not
 * `window.innerWidth`. The app sidebar sits outside Dockview, so the browser
 * viewport can materially overstate the space available to chat + files. */
function applyRightConstraints(
  api: DockviewReadyEvent["api"],
  measuredWidth: number | undefined,
  right: PinnedRightColumn,
): number {
  const sidebarWidth = getRootSidebarWidth(api);
  const maximumWidth = computeRightMaxPx(measuredWidth, sidebarWidth);
  refreshPinnedRightConstraints(api, { groups: right.groups, maximumWidth });
  return maximumWidth;
}

/** Force the pinned right column back to its target width (manual width,
 *  stored target, or responsive default) after layout changes, counteracting
 *  dockview's proportional rebalancing; skipped while a sash drag, maximize,
 *  or layout restore is in progress unless `allowDuringRestore` is set. */
// eslint-disable-next-line complexity
function enforcePinnedTargets(api: DockviewReadyEvent["api"], allowDuringRestore = false): void {
  if (enforcing || sashDragging) return;
  const store = useDockviewStore.getState();
  if (store.isRestoringLayout && !allowDuringRestore) return;
  if (api.hasMaximizedGroup() || store.preMaximizeLayout !== null) return;
  validateRetainedRootGeometry(api, store.currentLayoutEnvId, store.hiddenRightPane);
  const right = getPinnedRightColumn(api);
  if (!right) refreshPinnedRightConstraints(api, null);
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  enforcing = true;
  try {
    if (right) {
      const measuredWidth = measureDockviewGridWidth(api);
      const sidebarWidth = getRootSidebarWidth(api);
      const maximumWidth = applyRightConstraints(api, measuredWidth, right);
      const manualRightWidth = getManualRightWidth(store.currentLayoutEnvId);
      const target =
        manualRightWidth ??
        getPinnedTarget("right") ??
        resolveResponsiveRightWidth(measuredWidth, sidebarWidth, null);
      if (
        !restoreReusableRootGeometry(
          api,
          store.currentLayoutEnvId,
          measuredWidth,
          Math.min(target, maximumWidth),
          {
            manualWidth: manualRightWidth,
            hiddenRightPane: store.hiddenRightPane,
          },
        )
      ) {
        restoreColumnToTarget(sv, right.index, target, maximumWidth);
      }
    }
  } finally {
    enforcing = false;
  }
}

/** Set the loose runtime cap so the user can drag the column past its target. */
function setLooseConstraints(api: DockviewReadyEvent["api"]): void {
  const store = useDockviewStore.getState();
  if (store.isRestoringLayout) return;
  if (api.hasMaximizedGroup() || store.preMaximizeLayout !== null) return;

  const right = getPinnedRightColumn(api);
  if (right) {
    applyRightConstraints(api, measureDockviewGridWidth(api), right);
  } else refreshPinnedRightConstraints(api, null);
}

/**
 * Wire sash-drag handlers + per-layout-change enforcement.
 *
 * On `mousedown` on a `.dv-sash` we let dockview drive the drag freely.
 * On `mouseup`, we record the new column width as the target so future
 * rebalances restore to it. The `onDidLayoutChange` subscription enforces
 * the target after any non-user rebalance.
 */
export function setupSashDragCapToggle(api: DockviewReadyEvent["api"]): () => void {
  // Apply loose constraints once so the user can resize freely; targets are
  // enforced post-hoc via `enforcePinnedTargets`.
  setLooseConstraints(api);

  const layoutSub = api.onDidLayoutChange(() => enforcePinnedTargets(api));

  if (typeof document === "undefined") {
    return () => layoutSub.dispose();
  }

  let rootDrag: RootDrag | null = null;

  /** Begin tracking a sash drag: flag the drag state on a primary-button
   *  mousedown over a `.dv-sash` and record whether it is the right column's
   *  sash. */
  const onMouseDown = (e: MouseEvent): void => {
    // Only track primary-button drags. A right/middle mousedown that didn't
    // start a drag must not leave `sashDragging` permanently set (cubic P2).
    if (e.button !== 0) return;
    const t = e.target as HTMLElement | null;
    if (t?.closest(".dv-sash")) {
      sashDragging = true;
      rightSashDragging = isPinnedRightSash(api, e.target);
      rootDrag = captureRootDrag(api, e.target);
      setPinnedEnforcementSashDragging(true);
      if (isDebug()) {
        debugWidths(`sash-drag-start ${formatWidthsSnapshot(snapshotColumnWidths(api))}`);
      }
    }
  };
  /** End a sash drag: clear the drag flags and, when the dragged sash was the
   *  right column's, capture the post-drag width as the new pinned target and
   *  persist it as the manual right width. */
  const onMouseUp = (e: MouseEvent): void => {
    if (e.button !== 0 || !sashDragging) return;
    const persistRight = rightSashDragging;
    releaseEditedRootGeometry(api, rootDrag);
    rootDrag = null;
    sashDragging = false;
    rightSashDragging = false;
    setPinnedEnforcementSashDragging(false);
    // Capture the post-drag width as the new target.
    requestAnimationFrame(() => {
      const sv = getRootSplitview(api);
      if (!sv) return;
      const store = useDockviewStore.getState();
      const right = getPinnedRightColumn(api);
      if (persistRight && right) {
        const newRight = sv.getViewSize(right.index);
        setPinnedTarget("right", newRight);
        setManualRightWidth(store.currentLayoutEnvId, newRight);
        if (isDebug()) {
          debugWidths(
            `sash-drag-end captured=right:${Math.round(newRight)} ` +
              `${formatWidthsSnapshot(snapshotColumnWidths(api))}`,
          );
        }
      } else if (isDebug()) {
        debugWidths(`sash-drag-end ${formatWidthsSnapshot(snapshotColumnWidths(api))}`);
      }
    });
  };
  document.addEventListener("mousedown", onMouseDown, true);
  document.addEventListener("mouseup", onMouseUp, true);

  return () => {
    layoutSub.dispose();
    document.removeEventListener("mousedown", onMouseDown, true);
    document.removeEventListener("mouseup", onMouseUp, true);
    // Reset the module-scope flag so an unmount mid-drag (e.g. user navigates
    // away while holding a sash) doesn't leave enforcement permanently paused
    // for the next mount (claude).
    sashDragging = false;
    rightSashDragging = false;
    setPinnedEnforcementSashDragging(false);
  };
}

/** Persist the current right-column width to the store's `pinnedWidths` when
 *  it changes so it can be restored later; no-op during layout restore,
 *  maximize, or when no manual right width is set. */
function trackPinnedWidths(api: DockviewReadyEvent["api"]): void {
  const store = useDockviewStore.getState();
  if (store.isRestoringLayout) return;
  if (api.hasMaximizedGroup() || store.preMaximizeLayout !== null) return;
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  try {
    const right = getPinnedRightColumn(api);
    if (right) {
      if (getManualRightWidth(store.currentLayoutEnvId) === null) return;
      const rightW = sv.getViewSize(right.index);
      if (rightW > 50) {
        const current = store.pinnedWidths.get("right");
        if (current !== rightW) {
          store.setPinnedWidth("right", rightW);
        }
      }
    }
  } catch {
    /* noop */
  }
}

/**
 * Keep dockview's internal grid width in sync with the live DOM container.
 *
 * Dockview's own ResizeObserver occasionally drifts: a sequence of
 * fromJSON calls (each carrying a recorded `grid.width`) plus a viewport
 * change (devtools open/close, window resize) can leave `api.width` pinned
 * at a value smaller than the actual container, after which every
 * subsequent layout op pins it there. Observing the parent element and
 * forcing `api.layout` on every resize is a cheap belt-and-suspenders fix.
 */
export function setupContainerResizeSync(api: DockviewReadyEvent["api"]): () => void {
  if (typeof window === "undefined") {
    return () => {};
  }
  const dv = getDockviewElement(api);
  const parent = dv?.parentElement;
  const ro =
    parent && typeof ResizeObserver !== "undefined"
      ? new ResizeObserver(() => syncDockviewContainerSize(api, parent))
      : null;
  if (ro && parent) ro.observe(parent);
  /** On window resize, refresh the automatic right target and re-sync
   *  dockview's grid to the live container on the next animation frame. */
  const onWindowResize = (): void => {
    requestAnimationFrame(() => {
      refreshAutomaticRightTarget(api);
      const currentParent = getDockviewElement(api)?.parentElement;
      if (currentParent) syncDockviewContainerSize(api, currentParent);
    });
  };
  window.addEventListener("resize", onWindowResize);
  return () => {
    ro?.disconnect();
    window.removeEventListener("resize", onWindowResize);
  };
}

/**
 * Synchronize Dockview to its live container immediately.
 *
 * ResizeObserver normally owns this. Root-layout changes also call it from a
 * React layout effect so stale group coordinates cannot reach a painted frame.
 */
export function syncDockviewContainerSize(
  api: DockviewReadyEvent["api"],
  container: HTMLElement,
): void {
  const width = container.clientWidth;
  const height = container.clientHeight;
  if (width <= 0 || height <= 0) return;
  // Only the active reusable owner scales with a container-only change.
  // Non-owned saved targets retain their existing pixel policy.
  if (getReusableRightWidth(useDockviewStore.getState().currentLayoutEnvId, width) !== null) {
    refreshAutomaticRightTarget(api, width);
  }
  if (width === api.width && height === api.height) {
    // Environment switches can replace pinned targets without changing the
    // container dimensions, so enforce even when Dockview's layout is a no-op.
    enforcePinnedTargets(api, true);
    return;
  }
  api.layout(width, height);
  // Dockview's direct `api.layout` path does not consistently emit
  // `onDidLayoutChange`, so enforce immediately after the proportional
  // rebalance instead of relying on the subscription above.
  enforcePinnedTargets(api);
}

/** Keep the store's active group id in sync with dockview and persist pinned
 *  column widths on every layout change. */
export function setupGroupTracking(api: DockviewReadyEvent["api"]): () => void {
  const d1 = api.onDidActiveGroupChange((group) => {
    useDockviewStore.setState({ activeGroupId: group?.id ?? null });
  });
  useDockviewStore.setState({ activeGroupId: api.activeGroup?.id ?? null });
  const d2 = api.onDidLayoutChange(() => {
    trackPinnedWidths(api);
    const store = useDockviewStore.getState();
    if (!store.isRestoringLayout) store.refreshRightPaneState();
  });
  trackPinnedWidths(api);
  return () => {
    d1.dispose();
    d2.dispose();
  };
}

/** Debounce-save the dockview layout to localStorage and the env layout store
 *  on layout changes (skipping restores and maximize overlays), flush pending
 *  saves on page unload, and expose `__persistDockviewLayout__` for e2e tests;
 *  returns a cleanup that removes listeners and cancels the debounce. */
export function setupLayoutPersistence(
  api: DockviewReadyEvent["api"],
  saveTimerRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>,
  envIdRef: React.MutableRefObject<string | null>,
): () => void {
  /** Serialize the current dockview layout to localStorage and, when an env
   *  id is set, to the env layout store; skips while a restore is in flight
   *  or a pre-maximize layout is pending. */
  const persistNow = (): void => {
    const live = useDockviewStore.getState();
    if (live.preMaximizeLayout !== null || live.isRestoringLayout) return;
    try {
      const json = api.toJSON();
      const envId = envIdRef.current;
      localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(json));
      if (envId) {
        setEnvLayout(envId, withHiddenRightPaneMetadata(json, live.hiddenRightPane));
        setEnvLayoutProfile(envId, live.activeLayoutProfile);
      }
      if (isDebug()) {
        debugWidths(
          `persist env=${envId ?? "-"} ${formatWidthsSnapshot(snapshotColumnWidths(api))} ` +
            `jsonSizes=${formatJsonRootSizes(json)}`,
        );
      }
    } catch {
      // Ignore serialization errors
    }
  };
  // Expose `persistNow` to e2e tests so the helper can flush the saved layout
  // after a programmatic `sv.resizeView` (which doesn't emit
  // `onDidLayoutChange` and therefore can't ride the debounced auto-save).
  if (typeof window !== "undefined") {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (window as any).__persistDockviewLayout__ = persistNow;
  }

  const sub = api.onDidLayoutChange(() => {
    if (useDockviewStore.getState().isRestoringLayout) return;
    // While maximized, the live layout is the 2-column overlay. Persisting it
    // as the env's regular layout would mean: if we ever fall back to that
    // layout (e.g. maximize state lost), the user gets a truncated layout
    // instead of their real one. The dedicated maximize-state slot (managed
    // by maximizeGroup / saveOutgoingEnv) already captures the overlay.
    if (useDockviewStore.getState().preMaximizeLayout !== null) return;

    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      // Re-check at fire time: a maximize (or another restore) may have
      // started after this timer was scheduled. Persisting api.toJSON() now
      // would write the maximize overlay as the env's regular layout — the
      // bug this guard is meant to prevent.
      saveTimerRef.current = null;
      persistNow();
    }, 300);
  });

  // Flush a pending debounced save on tab close / reload — otherwise a
  // resize completed less than 300ms before unload is lost.
  /** Flush any pending debounced layout save before the page unloads so a
   *  resize made less than the debounce window ago isn't lost. */
  const onBeforeUnload = (): void => {
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
      persistNow();
    }
  };
  if (typeof window !== "undefined") {
    window.addEventListener("beforeunload", onBeforeUnload);
  }

  return () => {
    sub.dispose();
    if (typeof window !== "undefined") {
      window.removeEventListener("beforeunload", onBeforeUnload);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (window as any).__persistDockviewLayout__;
    }
    // Cancel any in-flight debounce so a pending fire can't race with
    // teardown and write a stale layout to storage.
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
    }
  };
}

/** When the last non-sidebar panel is closed while maximized, exit maximize
 *  and drop the closed panel from the restored pre-maximize layout. */
function handleMaximizeExitOnLastClose(
  api: DockviewReadyEvent["api"],
  removedId: string,
  nonSidebarRemaining: number,
): void {
  if (!(useDockviewStore.getState().preMaximizeLayout !== null) || nonSidebarRemaining > 0) return;
  requestAnimationFrame(() => {
    useDockviewStore.getState().exitMaximizedLayout();
    requestAnimationFrame(() => {
      const restoredPanel = api.getPanel(removedId);
      if (restoredPanel) restoredPanel.api.close();
    });
  });
}

/** Resolve a session id whose env matches the closed panel's env, used for
 *  session-scoped stops like stopVscode. */
function resolveSessionForEntry(
  appStore: StoreApi<AppState>,
  entryEnvId: string | undefined,
): string | null {
  const state = appStore.getState();
  const active = state.tasks.activeSessionId;
  if (!entryEnvId) return active;
  if (active && state.environmentIdBySessionId[active] === entryEnvId) return active;
  const match = Object.entries(state.environmentIdBySessionId).find(
    ([, eid]) => eid === entryEnvId,
  );
  return match?.[0] ?? active;
}

/** Tab close → ordinary terminals park (PTY + DB row survive, reappear in
 *  the "+" menu); scripts/bottom-panel/legacy passthrough still destroy. */
function handleTerminalPanelClosed(
  appStore: StoreApi<AppState>,
  panelId: string,
  params: Record<string, unknown>,
): void {
  if (terminalTerminateClosePanelIds.delete(panelId)) return;
  const terminalId = params.terminalId as string | undefined;
  if (!terminalId) return;
  const stampedEnv = params.environmentId as string | undefined;
  const stampedTaskID = params.taskID as string | undefined;
  const state = appStore.getState();
  const active = state.tasks.activeSessionId;
  const fallbackEnv = active ? (state.environmentIdBySessionId[active] ?? null) : null;
  const envForTerminal = stampedEnv || fallbackEnv;
  if (!envForTerminal) return;
  const shell = state.userShells.byEnvironmentId[envForTerminal]?.find(
    (s) => s.terminalId === terminalId,
  );
  if (shell?.kind === "ordinary") {
    parkUserShell(terminalId, stampedTaskID).then(
      () => state.updateUserShell(envForTerminal, terminalId, { state: "parked" }),
      (err: unknown) => console.error("park terminal on tab close:", err),
    );
  } else {
    stopUserShell(envForTerminal, terminalId, stampedTaskID).catch((err: unknown) =>
      console.warn("stop terminal on tab close:", err),
    );
  }
}

/** Wire dockview panel-removal cleanup: clear scroll targets, exit maximize
 *  when the last non-sidebar panel closes, stop the panel's vscode session /
 *  park or stop its terminal, and release the panel's portal. */
export function setupPortalCleanup(
  api: DockviewReadyEvent["api"],
  appStore: StoreApi<AppState>,
): void {
  api.onDidRemovePanel((panel) => {
    const dockviewStore = useDockviewStore.getState();
    if (panel.id.startsWith("session:")) {
      dockviewStore.clearScrollTargetForOwner(panel.id.slice("session:".length), panel.id);
    } else if (panel.id === "chat") {
      const activeSessionId = appStore.getState().tasks.activeSessionId;
      if (activeSessionId) dockviewStore.clearScrollTargetForOwner(activeSessionId, panel.id);
    }
    if (useDockviewStore.getState().isRestoringLayout) return;
    const remainingPanelCount = api.panels.filter((p) => p.id !== panel.id).length;
    handleMaximizeExitOnLastClose(api, panel.id, remainingPanelCount);
    const entry = panelPortalManager.get(panel.id);
    const sessionForApi = resolveSessionForEntry(appStore, entry?.envId);
    if (entry?.component === "vscode" && sessionForApi) stopVscode(sessionForApi);
    if (entry?.component === "terminal")
      handleTerminalPanelClosed(appStore, panel.id, entry.params);
    panelPortalManager.release(panel.id);
  });
}
