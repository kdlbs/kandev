import type { DockviewReadyEvent } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getRootSplitview } from "@/lib/state/dockview-layout-builders";
import {
  computeRightMaxPx,
  LAYOUT_PINNED_MIN_PX,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  setPinnedTarget,
  getPinnedTarget,
} from "@/lib/state/layout-manager";
import { setEnvLayout } from "@/lib/local-storage";
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

export function markTerminalPanelTerminateClose(panelId: string): void {
  terminalTerminateClosePanelIds.add(panelId);
}

/**
 * Pinned-column target enforcement.
 *
 * Dockview's splitview rebalances proportionally on any `api.layout` call,
 * which would otherwise grow pinned columns past their initial defaults on
 * container expansion and shrink them on container contraction. We treat
 * sidebar/right as having a *target width* (stored in `pinned-targets.ts`)
 * that is updated only by explicit user actions (drag, initial layout,
 * restore from saved); after every layout-change event we force the live
 * columns back to their targets via `sv.resizeView`.
 */

/** Enforcement-in-progress guard to prevent infinite loops when our own
 *  `sv.resizeView` triggers `onDidLayoutChange`. */
let enforcing = false;

/** True while the user is actively dragging a `.dv-sash`. We pause target
 *  enforcement during the drag so the in-progress resize doesn't get
 *  reverted to the previous target on every intermediate layout change. */
let sashDragging = false;

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
function applyRightConstraints(api: DockviewReadyEvent["api"]): number {
  const measuredWidth = api.width > 0 ? api.width : undefined;
  const maximumWidth = computeRightMaxPx(measuredWidth);
  for (const gid of [RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]) {
    const group = api.groups.find((candidate) => candidate.id === gid);
    if (!group) continue;
    group.api.setConstraints({
      maximumWidth,
      minimumWidth: LAYOUT_PINNED_MIN_PX,
    });
  }
  return maximumWidth;
}

function enforcePinnedTargets(api: DockviewReadyEvent["api"]): void {
  if (enforcing || sashDragging) return;
  const store = useDockviewStore.getState();
  if (store.isRestoringLayout) return;
  if (api.hasMaximizedGroup() || store.preMaximizeLayout !== null) return;
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  enforcing = true;
  try {
    if (store.rightPanelsVisible) {
      const maximumWidth = applyRightConstraints(api);
      restoreColumnToTarget(sv, sv.length - 1, getPinnedTarget("right"), maximumWidth);
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

  if (store.rightPanelsVisible) applyRightConstraints(api);
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

  const onMouseDown = (e: MouseEvent): void => {
    // Only track primary-button drags. A right/middle mousedown that didn't
    // start a drag must not leave `sashDragging` permanently set (cubic P2).
    if (e.button !== 0) return;
    const t = e.target as HTMLElement | null;
    if (t?.closest(".dv-sash")) {
      sashDragging = true;
      if (isDebug()) {
        debugWidths(`sash-drag-start ${formatWidthsSnapshot(snapshotColumnWidths(api))}`);
      }
    }
  };
  const onMouseUp = (e: MouseEvent): void => {
    if (e.button !== 0 || !sashDragging) return;
    sashDragging = false;
    // Capture the post-drag width as the new target.
    requestAnimationFrame(() => {
      const sv = getRootSplitview(api);
      if (!sv) return;
      const store = useDockviewStore.getState();
      if (store.rightPanelsVisible) {
        const newRight = sv.getViewSize(sv.length - 1);
        setPinnedTarget("right", newRight);
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
  };
}

function trackPinnedWidths(api: DockviewReadyEvent["api"]): void {
  const store = useDockviewStore.getState();
  if (store.isRestoringLayout) return;
  if (api.hasMaximizedGroup() || store.preMaximizeLayout !== null) return;
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  try {
    // Right column is the last grid index when present. Skip when there is
    // no right column (compact preset, rightPanelsVisible=false).
    if (store.rightPanelsVisible) {
      const rightIdx = sv.length - 1;
      const rightW = sv.getViewSize(rightIdx);
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
  if (typeof window === "undefined" || typeof ResizeObserver === "undefined") {
    return () => {};
  }
  const dv = document.querySelector(".dv-dockview") as HTMLElement | null;
  const parent = dv?.parentElement;
  if (!parent) return () => {};
  const ro = new ResizeObserver(() => {
    const w = parent.clientWidth;
    const h = parent.clientHeight;
    if (w <= 0 || h <= 0) return;
    if (w === api.width && h === api.height) return;
    api.layout(w, h);
    // Dockview's direct `api.layout` path does not consistently emit
    // `onDidLayoutChange`, so enforce immediately after the proportional
    // rebalance instead of relying on the subscription above.
    enforcePinnedTargets(api);
  });
  ro.observe(parent);
  return () => ro.disconnect();
}

export function setupGroupTracking(api: DockviewReadyEvent["api"]): () => void {
  const d1 = api.onDidActiveGroupChange((group) => {
    useDockviewStore.setState({ activeGroupId: group?.id ?? null });
  });
  useDockviewStore.setState({ activeGroupId: api.activeGroup?.id ?? null });
  const d2 = api.onDidLayoutChange(() => trackPinnedWidths(api));
  trackPinnedWidths(api);
  return () => {
    d1.dispose();
    d2.dispose();
  };
}

export function setupLayoutPersistence(
  api: DockviewReadyEvent["api"],
  saveTimerRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>,
  envIdRef: React.MutableRefObject<string | null>,
): () => void {
  const persistNow = (): void => {
    const live = useDockviewStore.getState();
    if (live.preMaximizeLayout !== null || live.isRestoringLayout) return;
    try {
      const json = api.toJSON();
      const envId = envIdRef.current;
      localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(json));
      if (envId) {
        setEnvLayout(envId, json);
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

export function setupPortalCleanup(
  api: DockviewReadyEvent["api"],
  appStore: StoreApi<AppState>,
): void {
  api.onDidRemovePanel((panel) => {
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
