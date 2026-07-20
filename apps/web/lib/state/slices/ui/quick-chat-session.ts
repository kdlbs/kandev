import type { QuickChatSessionKind } from "./types";

const SETUP_SESSION_PREFIX = "quick-chat-setup:";
const SETUP_SESSION_ID_PATTERN = /^quick-chat-setup:.+:(?:chat|config)$/;

export function getQuickChatSetupSessionId(
  workspaceId: string,
  kind: QuickChatSessionKind,
): string {
  return `${SETUP_SESSION_PREFIX}${workspaceId}:${kind}`;
}

export function isQuickChatSetupSessionId(sessionId: string): boolean {
  return SETUP_SESSION_ID_PATTERN.test(sessionId);
}
