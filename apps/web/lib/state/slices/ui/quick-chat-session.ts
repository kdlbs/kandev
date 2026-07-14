import type { QuickChatSessionKind } from "./types";

const SETUP_SESSION_PREFIX = "quick-chat-setup:";

export function getQuickChatSetupSessionId(
  workspaceId: string,
  kind: QuickChatSessionKind,
): string {
  return `${SETUP_SESSION_PREFIX}${workspaceId}:${kind}`;
}

export function isQuickChatSetupSessionId(sessionId: string): boolean {
  return sessionId.startsWith(SETUP_SESSION_PREFIX);
}
