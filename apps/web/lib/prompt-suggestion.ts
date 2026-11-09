import type { Message, TaskSession, Turn } from "@/lib/types/http";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";

// i18n-exempt: localStorage key, not user-facing copy.
const DISMISSAL_KEY = "kandev.promptSuggestionDismissals.v1";
const DISMISSAL_LIMIT = 50;

export type NativePromptSuggestion = { turnKey: string; text: string };

/** Reads the native suggestion the backend stores in session metadata. */
export function readNativePromptSuggestion(
  metadata: Record<string, unknown> | null | undefined,
): NativePromptSuggestion | null {
  const raw = metadata?.prompt_suggestion;
  if (!raw || typeof raw !== "object") return null;
  const { turn_id: turnKey, text } = raw as { turn_id?: unknown; text?: unknown };
  if (typeof turnKey !== "string" || !turnKey || typeof text !== "string" || !text.trim()) {
    return null;
  }
  return { turnKey, text: text.trim() };
}

function isConversationEntry(message: Message): boolean {
  return message.type === "message" || message.type === "content";
}

/** Identifies the suggestion turn. `isTurnId` is false when only a message ID was available. */
export type SuggestionTurnKey = { key: string; isTurnId: boolean };

/** The suggestion turn: the latest completed turn, or the turn of the newest agent
 *  message when no turn records are loaded. Null while a newer turn is pending. */
export function resolveSuggestionTurnKey(
  turns: readonly Turn[] | undefined,
  messages: readonly Message[] | undefined,
): SuggestionTurnKey | null {
  if (turns && turns.length > 0) {
    const latest = turns[turns.length - 1];
    return latest.completed_at ? { key: latest.id, isTurnId: true } : null;
  }
  const entries = (messages ?? []).filter(isConversationEntry);
  const newest = entries[entries.length - 1];
  if (!newest || newest.author_type === "user") return null;
  return newest.turn_id
    ? { key: newest.turn_id, isTurnId: true }
    : { key: newest.id, isTurnId: false };
}

/** Whether the session negotiated native suggestions with its agent. The
 *  backend records this from the agent's ACP capability advertisement. */
export function isNativeSuggestionSession(
  session: Pick<TaskSession, "metadata"> | null | undefined,
): boolean {
  return session?.metadata?.prompt_suggestion_source === "native";
}

export type SuggestionSelectionInput = {
  enabled: boolean;
  session: Pick<TaskSession, "state" | "metadata" | "error_message"> | null;
  /** The current suggestion turn, from `resolveSuggestionTurnKey`. */
  turnKey: SuggestionTurnKey | null;
  activeTurn: boolean;
  /** True while the composer is busy, recovering, moving, or waiting on a clarification. */
  blocked: boolean;
  queued: boolean;
  dismissedTurnKey: string | null;
};

function isIdle(input: SuggestionSelectionInput): boolean {
  const session = input.session;
  return (
    session?.state === "WAITING_FOR_INPUT" &&
    !session.error_message &&
    !input.activeTurn &&
    !input.blocked &&
    !input.queued
  );
}

/** The native suggestion the composer may show now, or null. Sessions without
 *  native support never show one, and a suggestion from an older turn is stale. */
export function selectVisibleSuggestion(
  input: SuggestionSelectionInput,
): NativePromptSuggestion | null {
  if (!input.enabled || !isNativeSuggestionSession(input.session) || !isIdle(input)) return null;
  const native = readNativePromptSuggestion(input.session?.metadata);
  if (!native) return null;
  const turnKey = input.turnKey;
  // Without a turn ID, the backend's state-guarded turn binding is authoritative.
  if (!turnKey || (turnKey.isTurnId && native.turnKey !== turnKey.key)) return null;
  return native.turnKey === input.dismissedTurnKey ? null : native;
}

export type SuggestionDismissal = { sessionId: string; turnKey: string };

function isDismissal(value: unknown): value is SuggestionDismissal {
  if (!value || typeof value !== "object") return false;
  const entry = value as Record<string, unknown>;
  return typeof entry.sessionId === "string" && typeof entry.turnKey === "string";
}

function readDismissals(): SuggestionDismissal[] {
  const raw: unknown = getLocalStorage(DISMISSAL_KEY, []);
  return Array.isArray(raw) ? raw.filter(isDismissal) : [];
}

/** Reads the browser-local dismissal for a session. */
export function readSuggestionDismissal(sessionId: string): SuggestionDismissal | null {
  return readDismissals().find((entry) => entry.sessionId === sessionId) ?? null;
}

/** Stores one dismissal per session, newest first, bounded to 50 sessions. */
export function writeSuggestionDismissal(entry: SuggestionDismissal): void {
  const rest = readDismissals().filter((item) => item.sessionId !== entry.sessionId);
  setLocalStorage(DISMISSAL_KEY, [entry, ...rest].slice(0, DISMISSAL_LIMIT));
}
