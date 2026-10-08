import type { Message, TaskSession, Turn } from "@/lib/types/http";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";

/** Built-in utility agent that predicts the next prompt for agents without native suggestions. */
export const SUGGEST_NEXT_PROMPT_AGENT_ID = "builtin-suggest-next-prompt";

/** Abandon a utility fallback request that has not answered within this budget. */
export const PROMPT_SUGGESTION_TIMEOUT_MS = 15_000;

const MAX_SUGGESTION_WORDS = 12;
const MAX_SUGGESTION_CHARS = 100;
const TRANSCRIPT_LIMIT = 6000;
const USER_PROMPT_LIMIT = 1500;
const CACHE_KEY = "kandev.promptSuggestion.v1";
const CACHE_LIMIT = 50;

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

/** The suggestion turn: the latest completed turn, or the newest agent message without turn records. */
export function resolveSuggestionTurnKey(
  turns: readonly Turn[] | undefined,
  messages: readonly Message[] | undefined,
): string | null {
  if (turns && turns.length > 0) {
    const latest = turns[turns.length - 1];
    return latest.completed_at ? latest.id : null;
  }
  const entries = (messages ?? []).filter(isConversationEntry);
  const newest = entries[entries.length - 1];
  return newest && newest.author_type !== "user" ? newest.id : null;
}

/** Whether the newest conversation entry came from the agent. */
export function lastEntryIsAgent(messages: readonly Message[] | undefined): boolean {
  const entries = (messages ?? []).filter(isConversationEntry);
  const newest = entries[entries.length - 1];
  return Boolean(newest) && newest.author_type !== "user";
}

/** Formats the latest exchange (last user prompt and the agent reply to it) for the fallback agent. */
export function buildSuggestionTranscript(messages: readonly Message[]): string {
  const entries = messages.filter(isConversationEntry);
  let start = entries.length - 1;
  while (start >= 0 && entries[start].author_type !== "user") start -= 1;
  const user = start >= 0 ? entries[start].content.trim().slice(0, USER_PROMPT_LIMIT) : "";
  const agent = entries
    .slice(start + 1)
    .map((entry) => entry.content.trim())
    .filter(Boolean)
    .join("\n\n");
  // i18n-exempt: transcript role labels are model input, never shown to a user.
  const userPart = user ? `User: ${user}\n\n` : "";
  // i18n-exempt: transcript role labels are model input, never shown to a user.
  const agentLabel = "Agent: ";
  const room = Math.max(0, TRANSCRIPT_LIMIT - userPart.length - agentLabel.length);
  return `${userPart}${agentLabel}${agent.slice(Math.max(0, agent.length - room))}`;
}

const WRAPPING_TAG = /^<(suggestion|response|output|answer|result)>([\s\S]*)<\/\1>$/i;
const LEADING_LABEL =
  /^\s*(suggested\s+(response|reply|prompt)|suggestion|response|reply|answer)\s*[:：]\s*/i;
const META_TEXT =
  /^(none|done|silence|nothing to suggest.*|no suggestion.*)\W*$|\bstay(s|ing)? silent\b|^\W*[([].*[)\]]\W*$/i;
const SEVERAL_SENTENCES = /[.!?]\s+\p{Lu}/u;
const EVALUATIVE =
  /\b(thanks|thank you|looks good|sounds good|that works|perfect|great|nice|awesome|makes sense)\b/i;
const AGENT_VOICE =
  /^(let me|i'll|i've|i'm|i can|i will|here's|here is|here are|sure,|of course|certainly)\b/i;

function stripWrappers(raw: string): string {
  let text = raw.trim();
  const tag = WRAPPING_TAG.exec(text);
  if (tag) text = tag[2].trim();
  text = text.replace(LEADING_LABEL, "").trim();
  if (/^(["'`])[\s\S]*\1$/.test(text) || /^“[\s\S]*”$/.test(text)) text = text.slice(1, -1).trim();
  return text;
}

/** Applies Claude Code's suggestion filters; returns null when nothing usable remains. */
export function normalizeFallbackSuggestion(raw: string | null | undefined): string | null {
  const text = stripWrappers(raw ?? "");
  if (!text) return null;
  const words = text.split(/\s+/).filter(Boolean).length;
  const rejected =
    META_TEXT.test(text) ||
    words > MAX_SUGGESTION_WORDS ||
    text.length >= MAX_SUGGESTION_CHARS ||
    SEVERAL_SENTENCES.test(text) ||
    /[\n*]/.test(text) ||
    EVALUATIVE.test(text) ||
    AGENT_VOICE.test(text);
  return rejected ? null : text;
}

export type FallbackEligibility = {
  enabled: boolean;
  fallbackEnabled: boolean;
  native: boolean;
  sessionState: string | null | undefined;
  needsRecovery: boolean;
  activeTurn: boolean;
  pendingClarification: boolean;
  queued: boolean;
  lastEntryIsAgent: boolean;
  turnKey: string | null;
  cached: boolean;
  inFlight: boolean;
};

/** Whether the composer should ask the utility fallback for a suggestion now. */
export function shouldRequestFallback(input: FallbackEligibility): boolean {
  return (
    input.enabled &&
    input.fallbackEnabled &&
    !input.native &&
    input.sessionState === "WAITING_FOR_INPUT" &&
    !input.needsRecovery &&
    !input.activeTurn &&
    !input.pendingClarification &&
    !input.queued &&
    input.lastEntryIsAgent &&
    Boolean(input.turnKey) &&
    !input.cached &&
    !input.inFlight
  );
}

export type SuggestionCacheEntry = {
  sessionId: string;
  turnKey: string;
  text: string | null;
  dismissed: boolean;
};

function isCacheEntry(value: unknown): value is SuggestionCacheEntry {
  if (!value || typeof value !== "object") return false;
  const entry = value as Record<string, unknown>;
  return (
    typeof entry.sessionId === "string" &&
    typeof entry.turnKey === "string" &&
    (typeof entry.text === "string" || entry.text === null) &&
    typeof entry.dismissed === "boolean"
  );
}

function readCache(): SuggestionCacheEntry[] {
  const raw: unknown = getLocalStorage(CACHE_KEY, []);
  return Array.isArray(raw) ? raw.filter(isCacheEntry) : [];
}

/** Reads the browser-local fallback result and dismissal state for a session. */
export function readSuggestionCache(sessionId: string): SuggestionCacheEntry | null {
  return readCache().find((entry) => entry.sessionId === sessionId) ?? null;
}

/** Stores one entry per session, newest first, bounded to 50 sessions. */
export function writeSuggestionCache(entry: SuggestionCacheEntry): void {
  const rest = readCache().filter((item) => item.sessionId !== entry.sessionId);
  setLocalStorage(CACHE_KEY, [entry, ...rest].slice(0, CACHE_LIMIT));
}

/** Whether the session asked its Claude Code agent for native suggestions, so
 *  the utility fallback must not run. Recorded by the backend from the agent's
 *  negotiated capability. */
export function isNativeSuggestionSession(
  session: Pick<TaskSession, "metadata"> | null | undefined,
): boolean {
  return session?.metadata?.prompt_suggestion_source === "native";
}
