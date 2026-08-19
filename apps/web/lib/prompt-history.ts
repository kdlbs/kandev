import type { Message, Turn } from "@/lib/types/http";

export type PromptHistoryEntry = {
  messageId: string;
  sessionId: string;
  content: string;
  sentAt: string;
  durationSeconds: number | null;
  isLastPrompt: boolean;
  isAgentPrompt: boolean;
};

export type PromptDurationUnits = {
  s: string;
  m: string;
  h: string;
};

type PromptWithTimestamp = Message & { timestamp: number };

/** Returns whether a user prompt was sent by another task's agent. */
function isAgentPrompt(message: Message): boolean {
  const senderTaskId = message.metadata?.sender_task_id;
  return typeof senderTaskId === "string" && senderTaskId.length > 0;
}

/** Parse a date string into a millisecond epoch timestamp, returning `null` for missing or unparseable values. */
function timestamp(value: string | undefined): number | null {
  if (!value) return null;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

/** Sort prompts by timestamp ascending, breaking ties by id in code-unit order for deterministic results. */
function comparePrompts(a: PromptWithTimestamp, b: PromptWithTimestamp): number {
  // Deterministic code-unit ascending id order — localeCompare collation can
  // reorder mixed-case/punctuation ids across runtimes and locales.
  const timeDiff = a.timestamp - b.timestamp;
  if (timeDiff !== 0) return timeDiff;
  if (a.id < b.id) return -1;
  if (a.id > b.id) return 1;
  return 0;
}

/** Map each prompt's `session_id:id` key to its turn's completion timestamp, or `null` when the prompt has no turn or the turn lacks a completion time. */
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
        ? timestamp(
            turnsBySessionAndId.get(`${message.session_id}:${message.turn_id}`)?.completed_at,
          )
        : null,
    ]),
  );
}

/** Build prompt-history entries from user messages and turns, newest-first, with each prompt's duration bounded by its turn completion or the next prompt's send time. */
export function buildPromptHistoryEntries(
  messages: Message[],
  turns: Turn[],
): PromptHistoryEntry[] {
  const prompts = messages
    .flatMap((message) => {
      if (message.author_type !== "user") return [];
      const sentAt = timestamp(message.created_at);
      return sentAt === null ? [] : [{ ...message, timestamp: sentAt }];
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
    const nextPromptAt = nextPrompt?.timestamp ?? null;
    const end = Math.min(
      ...[completedAt, nextPromptAt].filter((value): value is number => value !== null),
    );
    const durationSeconds = Number.isFinite(end)
      ? Math.floor(Math.max(0, end - prompt.timestamp) / 1000)
      : null;

    return {
      messageId: prompt.id,
      sessionId: prompt.session_id,
      content: prompt.content,
      sentAt: prompt.created_at,
      durationSeconds,
      isLastPrompt: index === sessionPrompts.length - 1,
      isAgentPrompt: isAgentPrompt(prompt),
    };
  });

  return entries.reverse();
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
