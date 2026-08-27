import type { Message, Turn } from "@/lib/types/http";

export type PromptHistoryEntry = {
  messageId: string;
  sessionId: string;
  content: string;
  sentAt: string;
  durationSeconds: number | null;
  isLastPrompt: boolean;
  isAgentPrompt: boolean;
  /** Absolute 1-based ordinal among ALL user messages of the session (from the
   * server's `prompt_index`), or null when the payload carried no ordinal. */
  promptNumber: number | null;
};

export type PromptDurationUnits = {
  s: string;
  m: string;
  h: string;
};

type PromptWithTimestamp = Message & {
  /** Full epoch-nanosecond instant, used only for duration subtraction/bounds. */
  timestampNs: bigint;
  /** Backend-order key: epochNanoseconds / 1000n (pinned UTC microsecond
   * storage precision), ties broken by message id. Never compared as Number. */
  timestampMicros: bigint;
};

/** Returns whether a user prompt was sent by another task's agent. */
function isAgentPrompt(message: Message): boolean {
  const senderTaskId = message.metadata?.sender_task_id;
  return typeof senderTaskId === "string" && senderTaskId.length > 0;
}

/**
 * Parse an RFC3339/RFC3339Nano timestamp into a full BigInt epoch-nanosecond
 * key, returning `null` for missing or unparseable values. The variable-width
 * fraction is removed before whole-second parsing (Date.parse only has
 * millisecond precision and rounds the sub-ms remainder), then re-attached as
 * the digit run right-padded to nine digits — matching the backend's
 * `YYYY-MM-DD HH:MM:SS.ffffff` UTC microsecond key after floor division by
 * 1000n. Offsets (e.g. `+02:00`) are normalized by Date.parse to UTC; they
 * are whole seconds, so the fraction is preserved unchanged.
 */
function epochNanoseconds(value: string | undefined): bigint | null {
  if (!value) return null;
  const dot = value.indexOf(".");
  let wholePart = value;
  let fractionDigits = "";
  if (dot !== -1) {
    const rest = value.slice(dot + 1);
    const digitMatch = /^\d+/.exec(rest);
    fractionDigits = digitMatch ? digitMatch[0] : "";
    // Remove only the fraction digits; the zone suffix (Z or ±HH:MM) stays in
    // the whole part so Date.parse still normalizes the offset to UTC.
    wholePart = value.slice(0, dot) + rest.slice(fractionDigits.length);
  }
  const parsed = Date.parse(wholePart);
  if (Number.isNaN(parsed)) return null;
  // The whole part has no fraction, so the ms value is a whole second.
  const seconds = BigInt(Math.floor(parsed / 1000));
  const fractionNs = BigInt(fractionDigits.padEnd(9, "0").slice(0, 9) || "0");
  return seconds * BigInt(1_000_000_000) + fractionNs;
}

/** Floors a positive-or-negative BigInt division (BigInt `/` truncates toward
 * zero, which would mis-order pre-epoch values: -999ns must floor to -1µs,
 * not 0). Matches the backend's truncation semantics on both sides of epoch. */
function floorDivideBy1000(ns: bigint): bigint {
  return ns >= BigInt(0) ? ns / BigInt(1000) : -((-ns + BigInt(999)) / BigInt(1000));
}

/** Sort prompts by microsecond-truncated timestamp ascending, breaking ties by
 * id in code-unit order for deterministic results (the backend's
 * `(created_at_microsecond, id)` order). */
function comparePrompts(a: PromptWithTimestamp, b: PromptWithTimestamp): number {
  // Deterministic code-unit ascending id order — localeCompare collation can
  // reorder mixed-case/punctuation ids across runtimes and locales.
  const microsDiff = a.timestampMicros - b.timestampMicros;
  if (microsDiff !== BigInt(0)) return microsDiff < BigInt(0) ? -1 : 1;
  if (a.id < b.id) return -1;
  if (a.id > b.id) return 1;
  return 0;
}

/** Map each prompt's `session_id:id` key to its turn's completion timestamp in
 * full epoch nanoseconds, or `null` when the prompt has no turn or the turn
 * lacks a completion time. */
function turnCompletionByPrompt(messages: PromptWithTimestamp[], turns: Turn[]) {
  const turnsBySessionAndId = new Map<string, Turn>();
  for (const turn of turns) {
    turnsBySessionAndId.set(`${turn.session_id}:${turn.id}`, turn);
  }

  return new Map(
    messages.map((message) => [
      `${message.session_id}:${message.id}`,
      // Absent turn_id must never match: interpolating it would collide with
      // a turn literally id'd "undefined"/"null" in the same session.
      message.turn_id
        ? epochNanoseconds(
            turnsBySessionAndId.get(`${message.session_id}:${message.turn_id}`)?.completed_at,
          )
        : null,
    ]),
  );
}

/** Build prompt-history entries from user messages and turns, newest-first,
 * with each prompt's duration bounded by its turn completion or the next
 * prompt's send time. Ordering uses the backend's microsecond-truncated key
 * with the message id as tie-break; duration bounds use full-nanosecond
 * subtraction (floored to seconds, clamped at zero). */
export function buildPromptHistoryEntries(
  messages: Message[],
  turns: Turn[],
): PromptHistoryEntry[] {
  const prompts = messages
    .flatMap((message) => {
      if (message.author_type !== "user") return [];
      const timestampNs = epochNanoseconds(message.created_at);
      return timestampNs === null
        ? []
        : [{ ...message, timestampNs, timestampMicros: floorDivideBy1000(timestampNs) }];
    })
    .sort(comparePrompts);
  const completions = turnCompletionByPrompt(prompts, turns);
  const promptsBySession = new Map<string, PromptWithTimestamp[]>();
  const indexByPrompt = new Map<PromptWithTimestamp, number>();

  for (const prompt of prompts) {
    const sessionPrompts = promptsBySession.get(prompt.session_id) ?? [];
    indexByPrompt.set(prompt, sessionPrompts.length);
    sessionPrompts.push(prompt);
    promptsBySession.set(prompt.session_id, sessionPrompts);
  }

  const entries = prompts.map((prompt) => {
    const sessionPrompts = promptsBySession.get(prompt.session_id)!;
    const index = indexByPrompt.get(prompt)!;
    const nextPrompt = sessionPrompts[index + 1];
    const completedAt = completions.get(`${prompt.session_id}:${prompt.id}`) ?? null;
    const nextPromptAt = nextPrompt?.timestampNs ?? null;
    // The duration end is the EARLIER of the turn-completion and next-prompt
    // bounds (full-nanosecond precision); absent bounds leave it undefined.
    const bounds = [completedAt, nextPromptAt].filter((value): value is bigint => value !== null);
    let end: bigint | undefined;
    for (const bound of bounds) {
      if (end === undefined || bound < end) end = bound;
    }
    const durationNs = end !== undefined ? end - prompt.timestampNs : null;
    const durationSeconds =
      durationNs === null
        ? null
        : Number(durationNs < BigInt(0) ? BigInt(0) : durationNs / BigInt(1_000_000_000));

    return {
      messageId: prompt.id,
      sessionId: prompt.session_id,
      content: prompt.content,
      sentAt: prompt.created_at,
      durationSeconds,
      isLastPrompt: index === sessionPrompts.length - 1,
      isAgentPrompt: isAgentPrompt(prompt),
      promptNumber:
        typeof prompt.prompt_index === "number" && prompt.prompt_index > 0
          ? prompt.prompt_index
          : null,
    };
  });

  return entries.reverse();
}

/**
 * Return a completed user prompt turn's wall-clock duration in whole seconds.
 * This shares only the panel's timestamp parsing, flooring, and clamp
 * arithmetic; unlike prompt history, it does not bound the duration by the
 * next prompt.
 */
export function messageTurnDurationSeconds(message: Message, turn: Turn | null): number | null {
  if (
    message.author_type !== "user" ||
    !message.turn_id ||
    turn === null ||
    turn.id !== message.turn_id ||
    turn.session_id !== message.session_id
  ) {
    return null;
  }

  const createdAt = epochNanoseconds(message.created_at);
  const completedAt = epochNanoseconds(turn.completed_at);
  if (createdAt === null || completedAt === null) return null;

  const durationNs = completedAt - createdAt;
  return Number(durationNs < BigInt(0) ? BigInt(0) : durationNs / BigInt(1_000_000_000));
}

/** Format a duration in seconds as a compact `h m s` string using the given unit labels, omitting empty hour/minute parts. */
export function formatPromptDuration(seconds: number, units: PromptDurationUnits): string {
  const totalSeconds = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const remainingSeconds = totalSeconds % 60;

  if (hours > 0) return `${hours}${units.h} ${minutes}${units.m} ${remainingSeconds}${units.s}`;
  if (minutes > 0) return `${minutes}${units.m} ${remainingSeconds}${units.s}`;
  return `${remainingSeconds}${units.s}`;
}
