import type { DockviewReadyEvent } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getRootSplitview } from "@/lib/state/dockview-layout-builders";
import { setEnvLayout } from "@/lib/local-storage";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { stopVscode } from "@/lib/api/domains/vscode-api";
import { parkUserShell, stopUserShell } from "@/lib/api/domains/user-shell-api";

const LAYOUT_STORAGE_KEY = "dockview-layout-v1";

function trackPinnedWidths(api: DockviewReadyEvent["api"]): void {
  if (useDockviewStore.getState().isRestoringLayout) return;
  if (api.hasMaximizedGroup() || useDockviewStore.getState().preMaximizeLayout !== null) return;
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  try {
    const sidebarW = sv.getViewSize(0);
    if (sidebarW > 50) {
      const current = useDockviewStore.getState().pinnedWidths.get("sidebar");
      if (current !== sidebarW) {
        useDockviewStore.getState().setPinnedWidth("sidebar", sidebarW);
      }
    }
    if (sv.length >= 3) {
      const rightIdx = sv.length - 1;
      const rightW = sv.getViewSize(rightIdx);
      if (rightW > 50) {
        const current = useDockviewStore.getState().pinnedWidths.get("right");
        if (current !== rightW) {
          useDockviewStore.getState().setPinnedWidth("right", rightW);
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
): void {
  api.onDidLayoutChange(() => {
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
      const live = useDockviewStore.getState();
      if (live.preMaximizeLayout !== null || live.isRestoringLayout) {
        saveTimerRef.current = null;
        return;
      }
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
    }, 300);
  });
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
  params: Record<string, unknown>,
): void {
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
    if (entry?.component === "terminal") handleTerminalPanelClosed(appStore, entry.params);
    panelPortalManager.release(panel.id);
  });
}
