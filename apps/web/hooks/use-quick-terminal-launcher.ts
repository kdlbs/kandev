"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { captureQuickChatLauncherFocus } from "@/components/quick-chat/quick-chat-focus";

/** Opens or re-selects a workspace's terminal in the shared Quick Chat surface. */
export function useQuickTerminalLauncher(workspaceId?: string | null) {
  const reuseOrCreateQuickTerminal = useAppStore((state) => state.reuseOrCreateQuickTerminal);

  return useCallback(() => {
    if (!workspaceId) return;
    captureQuickChatLauncherFocus();
    reuseOrCreateQuickTerminal(workspaceId);
  }, [reuseOrCreateQuickTerminal, workspaceId]);
}
