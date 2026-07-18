import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";

/**
 * Hook to handle opening quick chat.
 * Just opens the modal - the user will select an agent from the picker.
 */
export function useQuickChatLauncher(workspaceId?: string | null, kind?: QuickChatSessionKind) {
  const openQuickChat = useAppStore((state) => state.openQuickChat);
  const quickChatSessions = useAppStore((state) => state.quickChat.sessions);
  const activeSessionId = useAppStore((state) => state.quickChat.activeSessionId);

  const handleOpenQuickChat = useCallback(() => {
    if (!workspaceId) return;

    // If there's an existing session, open it. Otherwise just open the modal with agent picker
    const matchingSessions = quickChatSessions.filter(
      (session) =>
        session.workspaceId === workspaceId && (!kind || (session.kind ?? "chat") === kind),
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
      openQuickChat("", workspaceId, undefined, kind ?? "chat");
    }
  }, [workspaceId, kind, quickChatSessions, activeSessionId, openQuickChat]);

  return handleOpenQuickChat;
}
