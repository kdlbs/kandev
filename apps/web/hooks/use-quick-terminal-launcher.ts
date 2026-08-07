"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { captureQuickChatLauncherFocus } from "@/components/quick-chat/quick-chat-focus";
import { listQuickTerminalTabs, toQuickTerminalTab } from "@/lib/api/domains/quick-terminal-api";

/** Opens or re-selects a workspace's terminal in the shared Quick Chat surface. */
export function useQuickTerminalLauncher(workspaceId?: string | null) {
  const reuseOrCreateQuickTerminal = useAppStore((state) => state.reuseOrCreateQuickTerminal);
  const hydrate = useAppStore((state) => state.hydrate);
  const terminalTabs = useAppStore((state) => state.quickChat.terminalTabs);

  return useCallback(async () => {
    if (!workspaceId) return;
    captureQuickChatLauncherFocus();

    if (!terminalTabs.some((tab) => tab.workspaceId === workspaceId)) {
      try {
        const response = await listQuickTerminalTabs(workspaceId);
        if (response.tabs.length > 0) {
          hydrate({
            quickChat: { terminalTabs: response.tabs.map(toQuickTerminalTab) },
          });
        }
      } catch {
        // Best-effort. If the shared-state refresh fails, fall back to the
        // existing local reuse-or-create behavior.
      }
    }

    reuseOrCreateQuickTerminal(workspaceId);
  }, [hydrate, reuseOrCreateQuickTerminal, terminalTabs, workspaceId]);
}
