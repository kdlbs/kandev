import type {
  ClarificationRequestMetadata,
  Message,
  TaskPendingAction,
  Turn,
} from "@/lib/types/http";

export type PendingClarificationScope = {
  /** Undefined means turn history is unavailable; null means loaded legacy history has no turns. */
  currentTurnId?: string | null;
  pendingAction?: TaskPendingAction | null;
};

export function isPendingClarificationMessage(message: Message): boolean {
  if (message.type !== "clarification_request") return false;
  const metadata = message.metadata as ClarificationRequestMetadata | undefined;
  return !metadata?.status || metadata.status === "pending";
}

export function newestDurableTurnId(turns?: readonly Turn[]): string | null | undefined {
  if (turns === undefined) return undefined;
  if (turns.length === 0) return null;
  let newest = turns[0];
  for (let index = 1; index < turns.length; index++) {
    const candidate = turns[index];
    const newestKey = [newest.started_at, newest.created_at, newest.id];
    const candidateKey = [candidate.started_at, candidate.created_at, candidate.id];
    for (let part = 0; part < candidateKey.length; part++) {
      if (candidateKey[part] === newestKey[part]) continue;
      if (candidateKey[part] > newestKey[part]) newest = candidate;
      break;
    }
  }
  return newest.id;
}

function clarificationMessagesInScope(
  messages: readonly Message[],
  scope?: PendingClarificationScope,
): readonly Message[] {
  if (!scope || scope.currentTurnId === null) return messages;
  if (scope.currentTurnId === undefined) {
    return scope.pendingAction === "clarification" ? messages : [];
  }
  return messages.filter((message) => message.turn_id === scope.currentTurnId);
}

export function findPendingClarification(
  messages?: readonly Message[] | null,
  scope?: PendingClarificationScope,
): Message | null {
  if (!messages) return null;
  const scoped = clarificationMessagesInScope(messages, scope);
  for (let i = scoped.length - 1; i >= 0; i--) {
    if (scoped[i].type !== "clarification_request") continue;
    if (isPendingClarificationMessage(scoped[i])) return scoped[i];
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
export function findPendingClarificationGroup(
  messages?: readonly Message[] | null,
  scope?: PendingClarificationScope,
): Message[] {
  if (!messages) return [];
  const scoped = clarificationMessagesInScope(messages, scope);
  const last = findPendingClarification(scoped);
  if (!last) return [];
  const meta = last.metadata as ClarificationRequestMetadata | undefined;
  const pendingID = meta?.pending_id;
  if (!pendingID) return [last];
  const group = scoped.filter((m) => {
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

export function hasPendingClarification(
  messages?: readonly Message[] | null,
  scope?: PendingClarificationScope,
): boolean {
  return findPendingClarification(messages, scope) !== null;
}

export function hasPendingClarificationForSession(
  messagesBySession: Record<string, readonly Message[] | undefined>,
  sessionId?: string | null,
): boolean {
  if (!sessionId) return false;
  return hasPendingClarification(messagesBySession[sessionId]);
}

// --- Permission request helpers ---

export function isPendingPermissionMessage(message: Message): boolean {
  if (message.type !== "permission_request") return false;
  const metadata = message.metadata as { status?: string } | undefined;
  return !metadata?.status || metadata.status === "pending";
}

// hasPendingPermissionRequest reports whether the *current turn* is blocked
// on a permission_request. Scope:
//   - Scans backwards from the end and stops at the first permission_request
//     it sees — only the latest one drives the UI. A stale pending row left
//     behind by an earlier crash followed by a newer approved one must not
//     light up the amber icon (the agent is no longer blocked on the old row).
//   - Honours turn boundaries: when the latest message has a turn_id, walking
//     back to any row that doesn't share it ends the scan (including legacy
//     rows with null/undefined turn_id). Old turns' permissions can never
//     leak into the indicator and the scan stays bounded by the current turn
//     size (typically 5–50 messages) regardless of total session length.
//   - When the latest message itself has no turn_id (entirely pre-turn-scope
//     data), boundary enforcement is disabled and we fall back to plain
//     latest-only semantics across the whole array.
export function hasPendingPermissionRequest(messages?: readonly Message[] | null): boolean {
  if (!messages?.length) return false;
  const latestTurnId = messages[messages.length - 1].turn_id;
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (latestTurnId && m.turn_id !== latestTurnId) break;
    if (m.type !== "permission_request") continue;
    return isPendingPermissionMessage(m);
  }
  return false;
}

export function hasPendingPermissionForSession(
  messagesBySession: Record<string, readonly Message[] | undefined>,
  sessionId?: string | null,
): boolean {
  if (!sessionId) return false;
  return hasPendingPermissionRequest(messagesBySession[sessionId]);
}
