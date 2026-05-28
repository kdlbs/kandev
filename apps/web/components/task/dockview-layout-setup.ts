import type { DockviewReadyEvent } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getRootSplitview } from "@/lib/state/dockview-layout-builders";
import {
  computeSidebarMaxPx,
  computeRightMaxPx,
  LAYOUT_PINNED_MIN_PX,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  setPinnedTarget,
} from "@/lib/state/layout-manager";
import { setEnvLayout } from "@/lib/local-storage";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { stopVscode } from "@/lib/api/domains/vscode-api";
import { parkUserShell, stopUserShell } from "@/lib/api/domains/user-shell-api";
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";
import { snapshotColumnWidths, formatWidthsSnapshot } from "@/lib/state/dockview-widths-debug";
import { enforcePinnedTargets, setSashDragging } from "@/lib/state/dockview-pinned-enforce";

const debugWidths = createDebugLogger("dockview:widths");

// v2: bumped alongside DOCKVIEW_ENV_LAYOUT_PREFIX so the no-env fallback
// also invalidates layouts saved under the previous caps.
const LAYOUT_STORAGE_KEY = "dockview-layout-v2";
const terminalTerminateClosePanelIds = new Set<string>();

export function markTerminalPanelTerminateClose(panelId: string): void {
  terminalTerminateClosePanelIds.add(panelId);
}

// Pinned-column target enforcement and the `sashDragging` flag live in
// `lib/state/dockview-pinned-enforce.ts` so the store can call enforcement
// without importing this component-layer module. The sash mousedown/mouseup
// handlers below toggle the flag via `setSashDragging`.

/** Set the loose runtime cap so the user can drag the column past its target.
 *  Uses `api.width` (dockview's measured grid width) for the cap computation
 *  instead of `window.innerWidth`, which can briefly read stale during route
 *  transitions and devtools toggles - a vw=601 read yields cap=301 (=
 *  vw - VIEWPORT_RESERVE_PX) and squeezes the pinned column down to 301. */
function setLooseConstraints(api: DockviewReadyEvent["api"]): void {
  const store = useDockviewStore.getState();
  if (store.isRestoringLayout) return;
  if (api.hasMaximizedGroup() || store.preMaximizeLayout !== null) return;

  const vw = api.width > 0 ? api.width : undefined;
  const sb = api.getPanel("sidebar");
  if (sb && store.sidebarVisible) {
    sb.group.api.setConstraints({
      maximumWidth: computeSidebarMaxPx(vw),
      minimumWidth: LAYOUT_PINNED_MIN_PX,
    });
  }

  if (store.rightPanelsVisible) {
    for (const gid of [RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]) {
      const group = api.groups.find((g) => g.id === gid);
      if (group) {
        group.api.setConstraints({
          maximumWidth: computeRightMaxPx(vw),
          minimumWidth: LAYOUT_PINNED_MIN_PX,
        });
      }
    }
  }
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

  const layoutSub = api.onDidLayoutChange(() => {
    // Skip the reactive enforcement during a programmatic restore - the
    // restore path itself calls `enforcePinnedTargets` synchronously inside
    // its rAF so the user never sees a transient post-rebalance flicker.
    // Without this gate, the in-flight restore (which fires its own
    // layout-change events) would be re-entered before its rAF settled.
    const s = useDockviewStore.getState();
    if (s.isRestoringLayout) return;
    enforcePinnedTargets(api, {
      sidebarVisible: s.sidebarVisible,
      rightPanelsVisible: s.rightPanelsVisible,
      maximized: s.preMaximizeLayout !== null,
    });
  });

  if (typeof document === "undefined") {
    return () => layoutSub.dispose();
  }

  // Local mirror of the enforcement-module flag so the mouseup handler can
  // short-circuit when no drag was in progress without round-tripping through
  // a getter. Both this local and the module flag (via setSashDragging) must
  // stay in lockstep.
  let dragging = false;
  const onMouseDown = (e: MouseEvent): void => {
    // Only track primary-button drags. A right/middle mousedown that didn't
    // start a drag must not leave the flag permanently set (cubic P2).
    if (e.button !== 0) return;
    const t = e.target as HTMLElement | null;
    if (t?.closest(".dv-sash")) {
      dragging = true;
      setSashDragging(true);
    }
  };
  const onMouseUp = (e: MouseEvent): void => {
    if (e.button !== 0 || !dragging) return;
    dragging = false;
    setSashDragging(false);
    // Capture the post-drag width as the new target.
    requestAnimationFrame(() => {
      const sv = getRootSplitview(api);
      if (!sv) return;
      const store = useDockviewStore.getState();
      if (store.sidebarVisible) setPinnedTarget("sidebar", sv.getViewSize(0));
      if (store.rightPanelsVisible) setPinnedTarget("right", sv.getViewSize(sv.length - 1));
      if (IS_DEBUG) {
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
    // Reset both flags so an unmount mid-drag (e.g. user navigates away while
    // holding a sash) doesn't leave enforcement permanently paused for the
    // next mount.
    dragging = false;
    setSashDragging(false);
  };
}

function trackPinnedWidths(api: DockviewReadyEvent["api"]): void {
  const store = useDockviewStore.getState();
  if (store.isRestoringLayout) return;
  if (api.hasMaximizedGroup() || store.preMaximizeLayout !== null) return;
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  try {
    // Sidebar is grid index 0 *only when sidebar is visible*. Without the
    // visibility guard, hiding the sidebar makes index 0 the center column,
    // and we'd persist the center width as the sidebar's preferred width.
    if (store.sidebarVisible) {
      const sidebarW = sv.getViewSize(0);
      if (sidebarW > 50) {
        const current = store.pinnedWidths.get("sidebar");
        if (current !== sidebarW) {
          store.setPinnedWidth("sidebar", sidebarW);
        }
      }
    }
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
    if (IS_DEBUG) {
      debugWidths(
        `container-resize prev=${api.width}x${api.height} next=${w}x${h} ` +
          `pre=${formatWidthsSnapshot(snapshotColumnWidths(api))}`,
      );
    }
    api.layout(w, h);
    // `enforcePinnedTargets` (wired in `setupSashDragCapToggle`) restores
    // sidebar/right to their target widths via `onDidLayoutChange`, so we
    // don't need to redo that here.
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
    const nonSidebarRemaining = api.panels.filter(
      (p) => p.id !== panel.id && p.api.component !== "sidebar",
    ).length;
    handleMaximizeExitOnLastClose(api, panel.id, nonSidebarRemaining);
    const entry = panelPortalManager.get(panel.id);
    const sessionForApi = resolveSessionForEntry(appStore, entry?.envId);
    if (entry?.component === "vscode" && sessionForApi) stopVscode(sessionForApi);
    if (entry?.component === "terminal")
      handleTerminalPanelClosed(appStore, panel.id, entry.params);
    panelPortalManager.release(panel.id);
  });
}
