import type { DockviewReadyEvent } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getRootSplitview } from "@/lib/state/dockview-layout-builders";
import { setEnvLayout } from "@/lib/local-storage";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { stopVscode } from "@/lib/api/domains/vscode-api";
import { stopUserShell } from "@/lib/api/domains/user-shell-api";

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

    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
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
