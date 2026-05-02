import type { ClarificationRequestMetadata, Message } from "@/lib/types/http";

export function isPendingClarificationMessage(message: Message): boolean {
  if (message.type !== "clarification_request") return false;
  const metadata = message.metadata as ClarificationRequestMetadata | undefined;
  return !metadata?.status || metadata.status === "pending";
}

export function findPendingClarification(messages?: readonly Message[] | null): Message | null {
  if (!messages) return null;
  for (let i = messages.length - 1; i >= 0; i--) {
    if (isPendingClarificationMessage(messages[i])) return messages[i];
  }
  return null;
}

export function hasPendingClarification(messages?: readonly Message[] | null): boolean {
  return findPendingClarification(messages) !== null;
}

export function hasPendingClarificationForSession(
  messagesBySession: Record<string, readonly Message[] | undefined>,
  sessionId?: string | null,
): boolean {
  if (!sessionId) return false;
  return hasPendingClarification(messagesBySession[sessionId]);
}
