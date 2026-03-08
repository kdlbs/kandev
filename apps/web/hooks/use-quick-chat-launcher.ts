import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";

/**
 * Hook to handle opening quick chat.
 * Just opens the modal - the user will select an agent from the picker.
 */
export function useQuickChatLauncher(workspaceId?: string | null) {
  const openQuickChat = useAppStore((state) => state.openQuickChat);
  const quickChatSessions = useAppStore((state) => state.quickChat.sessions);

  const handleOpenQuickChat = useCallback(() => {
    if (!workspaceId) return;

    // If there's an existing session, open it. Otherwise just open the modal with agent picker
    const existingSession = quickChatSessions.find((s) => s.workspaceId === workspaceId);
    if (existingSession) {
      openQuickChat(existingSession.sessionId, workspaceId);
    } else {
      // Open modal without a session - will show agent picker
      openQuickChat("", workspaceId);
    }
  }, [workspaceId, quickChatSessions, openQuickChat]);

  return handleOpenQuickChat;
}
