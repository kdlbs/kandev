"use client";

import { useAppStore } from "@/components/state-provider";
import { useCallback, useSyncExternalStore } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getActiveTabForSession } from "@/lib/local-storage";
import {
  hiddenSessionIdsFor,
  onDidHideSessionPanel,
} from "@/components/task/dockview-hidden-session-panels";

const serverHiddenSnapshot = () => false;

function useSessionHiddenInDesktopLayout(sessionId: string | null): boolean {
  const api = useDockviewStore((state) => state.api);
  // The same API can host a different environment-scoped hide set.
  useDockviewStore((state) => state.currentLayoutEnvId);
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  const subscribe = useCallback(
    (notify: () => void) => {
      if (!api || !usesDesktopWorkbench) return () => {};
      const hide = onDidHideSessionPanel(api, notify);
      let visibility: { dispose: () => void } | undefined;
      const rebind = () => {
        visibility?.dispose();
        visibility = sessionId
          ? api.getPanel(`session:${sessionId}`)?.api.onDidVisibilityChange(notify)
          : undefined;
        notify();
      };
      rebind();
      const layout = api.onDidLayoutChange(rebind);
      return () => {
        hide.dispose();
        layout.dispose();
        visibility?.dispose();
      };
    },
    [api, usesDesktopWorkbench, sessionId],
  );
  const snapshot = useCallback(
    () =>
      !!api &&
      usesDesktopWorkbench &&
      !!sessionId &&
      (hiddenSessionIdsFor(api).has(sessionId) ||
        !api.getPanel(`session:${sessionId}`)?.api.isVisible),
    [api, usesDesktopWorkbench, sessionId],
  );
  return useSyncExternalStore(subscribe, snapshot, serverHiddenSnapshot);
}

/** Unread-dot preferences plus the displayed session the open task excludes. */
export function useUnreadTaskDotDisplay() {
  const unreadMessagesEnabled = useAppStore((state) => state.userSettings.unreadTaskDot);
  const delaySeconds = useAppStore((state) => state.userSettings.unreadTaskDotHideDelaySeconds);
  const displayedSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const chatVisible = useAppStore(
    (state) =>
      !displayedSessionId ||
      (isMobile
        ? (state.mobileSession.activePanelBySessionId[displayedSessionId] ?? "chat") === "chat"
        : !isTablet ||
          (state.tabletSession.activeTabBySessionId[displayedSessionId] ??
            (typeof window !== "undefined"
              ? getActiveTabForSession(displayedSessionId, "chat")
              : "chat")) === "chat"),
  );
  const hidden = useSessionHiddenInDesktopLayout(displayedSessionId);
  return {
    unreadMessagesEnabled,
    unreadDotHideDelayMs: delaySeconds * 1000,
    displayedSessionId: hidden || !chatVisible ? null : displayedSessionId,
  };
}
