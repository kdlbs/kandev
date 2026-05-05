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

// findPendingClarificationGroup returns every clarification_request message
// that shares the latest pending message's pending_id, ordered by chat position.
// Multi-question bundles emit one message per question; the chat panel uses this
// list to render every pending question card together.
//
// Gates on `question_total` from metadata: returns an empty array until the
// number of messages received equals the expected bundle size. This prevents
// a user from acting on a partially-arrived bundle (clicking an option before
// the rest of the N messages have been streamed in via the WS), which would
// otherwise trigger a 400 from the backend's all-required gate.
export function findPendingClarificationGroup(messages?: readonly Message[] | null): Message[] {
  if (!messages) return [];
  const last = findPendingClarification(messages);
  if (!last) return [];
  const meta = last.metadata as ClarificationRequestMetadata | undefined;
  const pendingID = meta?.pending_id;
  if (!pendingID) return [last];
  const group = messages.filter((m) => {
    if (m.type !== "clarification_request") return false;
    const mMeta = m.metadata as ClarificationRequestMetadata | undefined;
    return mMeta?.pending_id === pendingID;
  });
  const expectedTotal = meta?.question_total;
  if (typeof expectedTotal === "number" && expectedTotal > 0 && group.length < expectedTotal) {
    return [];
  }
  return group;
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
