import type {
  ClarificationRequestMetadata,
  Message,
  TaskPendingAction,
  Turn,
} from "@/lib/types/http";
import { isInputCapableSessionState } from "./task-pending-input";

export type PendingClarificationScope = {
  /**
   * Undefined means turn history is not loaded, so pendingAction gates the fallback.
   * Null means history is loaded but has no durable turns, so all messages are hidden.
   * A string scopes detection to that exact turn. An empty object disables detection.
   */
  currentTurnId?: string | null;
  pendingAction?: TaskPendingAction | null;
};

export function isPendingClarificationMessage(message: Message): boolean {
  if (message.type !== "clarification_request") return false;
  const metadata = message.metadata as ClarificationRequestMetadata | undefined;
  return !metadata?.status || metadata.status === "pending";
}

// TurnDTO timestamps are normalized by the backend to UTC with a Z suffix.
// Non-Z input falls through to raw comparison and is outside this contract.
function durableTurnTimestampKey(value: string): string {
  const match = /^(.*T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z$/.exec(value);
  if (!match) return value;
  return `${match[1]}.${(match[2] ?? "").padEnd(9, "0")}Z`;
}

// A turn counts as lifecycle-only for exactly these four encodings, mirroring
// the backend's turnMetadataFlagPredicate (D12): boolean true, number 1,
// string "true", string "1". Everything else - an absent key, null,
// undefined, false, 0, "", "false", "0", "yes", an object, an array - is
// conversational. Deliberately a value-set membership check, not a
// truthiness test (misreads "false"/"0"/"yes"/{}/[] as lifecycle) or a
// String() coercion (misreads ["1"]).
const LIFECYCLE_ONLY_VALUES: ReadonlySet<unknown> = new Set([true, 1, "true", "1"]);

function isLifecycleOnlyTurn(turn: Turn): boolean {
  return LIFECYCLE_ONLY_VALUES.has(turn.metadata?.lifecycle_only);
}

// completed_at is optional on the wire, so an open turn arrives as an absent
// key or explicit null; == null treats both alike.
function isOpenTurn(turn: Turn): boolean {
  return turn.completed_at == null;
}

// isNewerTurn mirrors D1's total order: an open turn always outranks a
// completed one regardless of timestamps, then started_at, created_at, id -
// all descending.
function isNewerTurn(candidate: Turn, current: Turn): boolean {
  const candidateOpen = isOpenTurn(candidate);
  if (candidateOpen !== isOpenTurn(current)) return candidateOpen;
  const candidateKey = [
    durableTurnTimestampKey(candidate.started_at),
    durableTurnTimestampKey(candidate.created_at),
    // Mirrors the backend's final deterministic tie-break. Turn IDs do not
    // encode creation time; exact timestamp ties have no finer ordering.
    candidate.id,
  ];
  const currentKey = [
    durableTurnTimestampKey(current.started_at),
    durableTurnTimestampKey(current.created_at),
    current.id,
  ];
  for (let part = 0; part < candidateKey.length; part++) {
    if (candidateKey[part] === currentKey[part]) continue;
    return candidateKey[part] > currentKey[part];
  }
  return false;
}

export function newestDurableTurnId(turns?: readonly Turn[]): string | null | undefined {
  if (turns === undefined) return undefined;
  const eligible = turns.filter((candidate) => !isLifecycleOnlyTurn(candidate));
  if (eligible.length === 0) return null;
  let newest = eligible[0];
  for (let index = 1; index < eligible.length; index++) {
    const candidate = eligible[index];
    if (isNewerTurn(candidate, newest)) newest = candidate;
  }
  return newest.id;
}

export function clarificationTurnIdForSession(
  sessionState: string | null | undefined,
  turns?: readonly Turn[],
): string | null | undefined {
  if (sessionState && !isInputCapableSessionState(sessionState)) return null;
  return newestDurableTurnId(turns);
}

function clarificationMessagesInScope(
  messages: readonly Message[],
  scope?: PendingClarificationScope,
): readonly Message[] {
  if (!scope) return messages;
  if (scope.pendingAction !== undefined && scope.pendingAction !== "clarification") return [];
  if (scope.currentTurnId === null) return [];
  if (scope.currentTurnId === undefined) {
    return scope.pendingAction === "clarification" ? messages : [];
  }
  return messages.filter((message) => message.turn_id === scope.currentTurnId);
}

function newestMessageTurnId(messages: readonly Message[]): string | undefined {
  let newest: Message | undefined;
  for (const message of messages) {
    if (!message.turn_id) continue;
    if (!newest) {
      newest = message;
      continue;
    }
    const createdAt = durableTurnTimestampKey(message.created_at);
    const newestCreatedAt = durableTurnTimestampKey(newest.created_at);
    if (createdAt > newestCreatedAt || (createdAt === newestCreatedAt && message.id > newest.id)) {
      newest = message;
    }
  }
  return newest?.turn_id;
}

export function findPendingClarification(
  messages?: readonly Message[] | null,
  scope?: PendingClarificationScope,
): Message | null {
  if (!messages?.length) return null;
  const scoped = clarificationMessagesInScope(messages, scope);
  // Sidebar callers do not have durable turn history. Use persisted message
  // order instead of WebSocket arrival order so a delayed predecessor event
  // cannot hide the current request or re-arm an older one.
  const latestTurnId = newestMessageTurnId(scoped);
  for (let i = scoped.length - 1; i >= 0; i--) {
    if (latestTurnId && scoped[i].turn_id !== latestTurnId) continue;
    if (scoped[i].type !== "clarification_request") continue;
    if (isPendingClarificationMessage(scoped[i])) return scoped[i];
  }
  return null;
}

// findPendingClarificationGroup returns pending clarification_request messages
// that share the latest pending message's pending_id, ordered by chat position.
// Multi-question bundles emit one message per question; terminal siblings count
// toward arrival completeness but are not rendered for a replacement answer.
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
  const bundle = scoped.filter((m) => {
    if (m.type !== "clarification_request") return false;
    const mMeta = m.metadata as ClarificationRequestMetadata | undefined;
    return mMeta?.pending_id === pendingID;
  });
  const expectedTotal = meta?.question_total;
  if (typeof expectedTotal === "number" && expectedTotal > 0 && bundle.length < expectedTotal) {
    return [];
  }
  return bundle.filter(isPendingClarificationMessage);
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
  scope?: PendingClarificationScope,
): boolean {
  if (!sessionId) return false;
  return hasPendingClarification(messagesBySession[sessionId], scope);
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
