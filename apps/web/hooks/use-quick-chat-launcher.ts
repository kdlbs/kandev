import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";

/**
 * Hook to handle opening quick chat.
 * Just opens the modal - the user will select an agent from the picker.
 */
export function useQuickChatLauncher(
  workspaceId?: string | null,
  kind: QuickChatSessionKind = "chat",
) {
  const openQuickChat = useAppStore((state) => state.openQuickChat);
  const quickChatSessions = useAppStore((state) => state.quickChat.sessions);

  const handleOpenQuickChat = useCallback(() => {
    if (!workspaceId) return;

    // If there's an existing session, open it. Otherwise just open the modal with agent picker
    const existingSession = quickChatSessions.find(
      (session) => session.workspaceId === workspaceId && (session.kind ?? "chat") === kind,
    );
    if (existingSession) {
      openQuickChat(existingSession.sessionId, workspaceId, undefined, kind);
    } else {
      // Open modal without a session - will show agent picker
      openQuickChat("", workspaceId, undefined, kind);
    }
  }, [workspaceId, kind, quickChatSessions, openQuickChat]);

  return handleOpenQuickChat;
}
