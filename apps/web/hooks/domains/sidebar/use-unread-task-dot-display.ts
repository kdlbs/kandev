"use client";

import { useAppStore } from "@/components/state-provider";
import { useCallback, useSyncExternalStore } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  hiddenSessionIdsFor,
  onDidHideSessionPanel,
} from "@/components/task/dockview-hidden-session-panels";

const serverHiddenSnapshot = () => false;

function useSessionHiddenInDesktopLayout(sessionId: string | null): boolean {
  const api = useDockviewStore((state) => state.api);
  // The same API can host a different environment-scoped hide set.
  useDockviewStore((state) => state.currentLayoutEnvId);
  const { isMobile } = useResponsiveBreakpoint();
  const subscribe = useCallback(
    (notify: () => void) => {
      if (!api || isMobile) return () => {};
      const hide = onDidHideSessionPanel(api, notify);
      const layout = api.onDidLayoutChange(notify);
      return () => {
        hide.dispose();
        layout.dispose();
      };
    },
    [api, isMobile],
  );
  const snapshot = useCallback(
    () => !!api && !isMobile && !!sessionId && hiddenSessionIdsFor(api).has(sessionId),
    [api, isMobile, sessionId],
  );
  return useSyncExternalStore(subscribe, snapshot, serverHiddenSnapshot);
}

/** Unread-dot preferences plus the displayed session the open task excludes. */
export function useUnreadTaskDotDisplay() {
  const unreadMessagesEnabled = useAppStore((state) => state.userSettings.unreadTaskDot);
  const delaySeconds = useAppStore((state) => state.userSettings.unreadTaskDotHideDelaySeconds);
  const displayedSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const hidden = useSessionHiddenInDesktopLayout(displayedSessionId);
  return {
    unreadMessagesEnabled,
    unreadDotHideDelayMs: delaySeconds * 1000,
    displayedSessionId: hidden ? null : displayedSessionId,
  };
}
