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
} from "@/lib/state/layout-manager";
import { setEnvLayout } from "@/lib/local-storage";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { stopVscode } from "@/lib/api/domains/vscode-api";
import { stopUserShell } from "@/lib/api/domains/user-shell-api";

// v2: bumped alongside DOCKVIEW_ENV_LAYOUT_PREFIX so the no-env fallback
// also invalidates layouts saved under the previous caps.
const LAYOUT_STORAGE_KEY = "dockview-layout-v2";

/**
 * Lock or release pinned-column max widths.
 *
 * Dockview's splitview rebalances proportionally on any `api.layout` call.
 * Without a tight upper bound, sidebar/right grow past their defaults every
 * time the container resizes. We solve this by pinning `maximumWidth` to the
 * column's current width whenever the user is NOT actively dragging — so
 * rebalance has no room to grow the column.
 *
 * During a sash drag (`allowGrowth=true`), we widen the cap to the
 * viewport-proportional runtime cap so the user can drag past the initial
 * default. On mouseup we snap the cap back down to the new current width.
 */
function applyPinnedCap(api: DockviewReadyEvent["api"], allowGrowth: boolean): void {
  const store = useDockviewStore.getState();
  if (store.isRestoringLayout) return;
  if (api.hasMaximizedGroup() || store.preMaximizeLayout !== null) return;
  const sv = getRootSplitview(api);
  if (!sv) return;

  const sb = api.getPanel("sidebar");
  if (sb && store.sidebarVisible) {
    const currentW = sv.getViewSize(0);
    const cap = allowGrowth ? computeSidebarMaxPx() : Math.max(currentW, LAYOUT_PINNED_MIN_PX);
    sb.group.api.setConstraints({ maximumWidth: cap, minimumWidth: LAYOUT_PINNED_MIN_PX });
  }

  if (store.rightPanelsVisible) {
    const rightIdx = sv.length - 1;
    const rightW = sv.getViewSize(rightIdx);
    const cap = allowGrowth ? computeRightMaxPx() : Math.max(rightW, LAYOUT_PINNED_MIN_PX);
    for (const gid of [RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]) {
      const group = api.groups.find((g) => g.id === gid);
      if (group) {
        group.api.setConstraints({ maximumWidth: cap, minimumWidth: LAYOUT_PINNED_MIN_PX });
      }
    }
  }
}

/** Listen for sash drag start/end and toggle the pinned cap accordingly. */
export function setupSashDragCapToggle(api: DockviewReadyEvent["api"]): () => void {
  if (typeof document === "undefined") return () => {};
  const onMouseDown = (e: MouseEvent): void => {
    const target = e.target as HTMLElement | null;
    if (target?.closest(".dv-sash")) {
      applyPinnedCap(api, true);
    }
  };
  const onMouseUp = (e: MouseEvent): void => {
    if (e.button !== 0) return;
    // Defer to the next frame so dockview's drag-end handler runs first;
    // otherwise we'd read the pre-release width.
    requestAnimationFrame(() => applyPinnedCap(api, false));
  };
  document.addEventListener("mousedown", onMouseDown, true);
  document.addEventListener("mouseup", onMouseUp, true);
  return () => {
    document.removeEventListener("mousedown", onMouseDown, true);
    document.removeEventListener("mouseup", onMouseUp, true);
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
    // Pin pinned-column caps to current widths so the impending api.layout
    // doesn't grow them proportionally past their defaults.
    applyPinnedCap(api, false);
    if (w === api.width && h === api.height) return;
    api.layout(w, h);
    // After api.layout, currentWidth may have shifted (constraint kicked in).
    // Re-pin so a future call has a fresh cap matching the new state.
    applyPinnedCap(api, false);
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
    }
    // Cancel any in-flight debounce so a pending fire can't race with
    // teardown and write a stale layout to storage.
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
    }
  };
}

export function setupPortalCleanup(
  api: DockviewReadyEvent["api"],
  appStore: StoreApi<AppState>,
): void {
  api.onDidRemovePanel((panel) => {
    if (useDockviewStore.getState().isRestoringLayout) return;
    const isMax = useDockviewStore.getState().preMaximizeLayout !== null;
    const remaining = api.panels.filter((p) => p.id !== panel.id);
    const nonSidebar = remaining.filter((p) => p.api.component !== "sidebar");
    // If we're in maximize mode and the last non-sidebar panel was just closed,
    // exit maximize to restore the pre-maximize layout (avoids empty view).
    // Then remove the closed panel from the restored layout so it doesn't reappear.
    if (isMax && nonSidebar.length === 0) {
      const removedId = panel.id;
      requestAnimationFrame(() => {
        useDockviewStore.getState().exitMaximizedLayout();
        // exitMaximizedLayout schedules a rAF to finalize — wait for that, then
        // remove the panel that was closed (it was re-created from preMaximizeLayout).
        requestAnimationFrame(() => {
          const restoredPanel = api.getPanel(removedId);
          if (restoredPanel) {
            restoredPanel.api.close();
          }
        });
      });
    }
    const entry = panelPortalManager.get(panel.id);
    // vscode is session-scoped; resolve to a session in the entry's env (or
    // the active session) for the stop call.
    const sessionForApi = (() => {
      const state = appStore.getState();
      const active = state.tasks.activeSessionId;
      if (!entry?.envId) return active;
      if (active && state.environmentIdBySessionId[active] === entry.envId) return active;
      const match = Object.entries(state.environmentIdBySessionId).find(
        ([, eid]) => eid === entry.envId,
      );
      return match?.[0] ?? active;
    })();
    if (entry?.component === "vscode" && sessionForApi) stopVscode(sessionForApi);
    if (entry?.component === "terminal") {
      const terminalId = entry.params.terminalId as string | undefined;
      // Prefer the env id stamped into params at creation time — this
      // survives task switches that happen between open and close. Fall
      // back to the active session's env for legacy panels created before
      // the param was added (e.g. layouts persisted from older releases).
      const stampedEnv = entry.params.environmentId as string | undefined;
      const state = appStore.getState();
      const active = state.tasks.activeSessionId;
      const fallbackEnv = active ? (state.environmentIdBySessionId[active] ?? null) : null;
      const envForTerminal = stampedEnv || fallbackEnv;
      if (terminalId && envForTerminal) stopUserShell(envForTerminal, terminalId);
    }
    panelPortalManager.release(panel.id);
  });
}
