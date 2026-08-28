import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  captureQuickChatLauncherFocus,
  requestQuickChatClose,
} from "@/components/quick-chat/quick-chat-focus";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";

type QuickChatLauncherOptions = {
  silentFocusReturn?: boolean;
  toggleWhenOpen?: boolean;
};

/**
 * Hook to handle opening quick chat.
 * Just opens the modal - the user will select an agent from the picker.
 */
export function useQuickChatLauncher(
  workspaceId?: string | null,
  kind: QuickChatSessionKind = "chat",
  options: QuickChatLauncherOptions = {},
) {
  const silentFocusReturn = options.silentFocusReturn ?? true;
  const toggleWhenOpen = options.toggleWhenOpen ?? false;
  const openQuickChat = useAppStore((state) => state.openQuickChat);
  const closeQuickChat = useAppStore((state) => state.closeQuickChat);
  const isQuickChatOpen = useAppStore((state) => state.quickChat.isOpen);
  const quickChatSessions = useAppStore((state) => state.quickChat.sessions);
  const activeSessionId = useAppStore((state) => state.quickChat.activeSessionId);

  const handleOpenQuickChat = useCallback(() => {
    if (!workspaceId) return;
    if (toggleWhenOpen && isQuickChatOpen) {
      if (!requestQuickChatClose()) closeQuickChat();
      return;
    }
    captureQuickChatLauncherFocus({ silent: silentFocusReturn });

    // If there's an existing session, open it. Otherwise just open the modal with agent picker
    const matchingSessions = quickChatSessions.filter(
      (session) => session.workspaceId === workspaceId && (session.kind ?? "chat") === kind,
    );
    const existingSession =
      matchingSessions.find((session) => session.sessionId === activeSessionId) ??
      matchingSessions[0];
    if (existingSession) {
      openQuickChat(
        existingSession.sessionId,
        workspaceId,
        undefined,
        existingSession.kind ?? "chat",
      );
    } else {
      // Open modal without a session - will show agent picker
      openQuickChat("", workspaceId, undefined, kind);
    }
  }, [
    workspaceId,
    toggleWhenOpen,
    isQuickChatOpen,
    closeQuickChat,
    silentFocusReturn,
    quickChatSessions,
    kind,
    activeSessionId,
    openQuickChat,
  ]);

  return handleOpenQuickChat;
}
