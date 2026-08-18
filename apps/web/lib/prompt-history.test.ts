import { describe, expect, it } from "vitest";
import type { Message, Turn } from "@/lib/types/http";
import { buildPromptHistoryEntries, formatPromptDuration } from "./prompt-history";

const MESSAGE_ID = "message-1";
const TURN_ID = "turn-1";
const CREATED_AT = "2026-01-01T00:00:00.000Z";

/** Builds a user Message with fixed defaults, merged with the given overrides. */
function message(overrides: Partial<Message>): Message {
  return {
    id: MESSAGE_ID,
    session_id: "session-1" as Message["session_id"],
    task_id: "task-1" as Message["task_id"],
    author_type: "user",
    content: "prompt",
    type: "message",
    created_at: CREATED_AT,
    ...overrides,
  };
}

/** Builds a Turn with fixed defaults, merged with the given overrides. */
function turn(overrides: Partial<Turn>): Turn {
  return {
    id: TURN_ID,
    session_id: "session-1" as Turn["session_id"],
    task_id: "task-1" as Turn["task_id"],
    started_at: CREATED_AT,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
    ...overrides,
  };
}

const ENTRY_CASES = [
  {
    name: "uses a completed turn when it is the only end bound",
    messages: [message({ turn_id: TURN_ID })],
    turns: [turn({ completed_at: "2026-01-01T00:00:05.900Z" })],
    expected: [{ messageId: MESSAGE_ID, durationSeconds: 5, isLastPrompt: true }],
  },
  {
    name: "uses the next prompt before turn completion",
    messages: [
      message({ id: "first", turn_id: TURN_ID }),
      message({ id: "second", created_at: "2026-01-01T00:00:03.200Z" }),
    ],
    turns: [turn({ completed_at: "2026-01-01T00:00:10.000Z" })],
    expected: [
      { messageId: "second", durationSeconds: null, isLastPrompt: true },
      { messageId: "first", durationSeconds: 3, isLastPrompt: false },
    ],
  },
  {
    name: "keeps a running last prompt without a duration",
    messages: [message({ turn_id: TURN_ID })],
    turns: [turn({})],
    expected: [{ messageId: MESSAGE_ID, durationSeconds: null, isLastPrompt: true }],
  },
  {
    name: "keeps a last prompt without a turn_id and without a later prompt at null",
    messages: [message({ id: "no-turn" })],
    turns: [],
    expected: [{ messageId: "no-turn", durationSeconds: null, isLastPrompt: true }],
  },
  {
    name: "treats an absent turn_id as no completion bound even when a turn is id 'undefined'",
    messages: [message({ id: "no-turn-undefined" })],
    turns: [turn({ id: "undefined", completed_at: "2026-01-01T00:00:05.000Z" })],
    expected: [{ messageId: "no-turn-undefined", durationSeconds: null, isLastPrompt: true }],
  },
  {
    name: "uses a following prompt when turn id is absent or dangling",
    messages: [
      message({ id: "missing", created_at: CREATED_AT }),
      message({
        id: "dangling",
        turn_id: "not-found",
        created_at: "2026-01-01T00:00:01.000Z",
      }),
      message({ id: "latest", created_at: "2026-01-01T00:00:04.900Z" }),
    ],
    turns: [],
    expected: [
      { messageId: "latest", durationSeconds: null, isLastPrompt: true },
      { messageId: "dangling", durationSeconds: 3, isLastPrompt: false },
      { messageId: "missing", durationSeconds: 1, isLastPrompt: false },
    ],
  },
  {
    name: "excludes invalid timestamps and uses the next valid prompt",
    messages: [
      message({ id: "first" }),
      message({ id: "invalid", created_at: "not-a-time" }),
      message({ id: "next", created_at: "2026-01-01T00:00:04.000Z" }),
    ],
    turns: [],
    expected: [
      { messageId: "next", durationSeconds: null, isLastPrompt: true },
      { messageId: "first", durationSeconds: 4, isLastPrompt: false },
    ],
  },
  {
    name: "treats an unparseable turn completed_at as absent with no later prompt",
    messages: [message({ id: "no-bound", turn_id: "turn-1" })],
    turns: [turn({ completed_at: "not-a-time" })],
    expected: [{ messageId: "no-bound", durationSeconds: null, isLastPrompt: true }],
  },
  {
    name: "lets the next-prompt bound win when the turn completed_at is unparseable",
    messages: [
      message({ id: "bad-turn", turn_id: "turn-1" }),
      message({ id: "later", created_at: "2026-01-01T00:00:04.000Z" }),
    ],
    turns: [turn({ completed_at: "not-a-time" })],
    expected: [
      { messageId: "later", durationSeconds: null, isLastPrompt: true },
      { messageId: "bad-turn", durationSeconds: 4, isLastPrompt: false },
    ],
  },
  {
    name: "scopes turn matching and last prompts to each session",
    messages: [
      message({ id: "a", session_id: "session-a" as Message["session_id"], turn_id: "turn-a" }),
      message({
        id: "b",
        session_id: "session-b" as Message["session_id"],
        turn_id: "turn-a",
        created_at: "2026-01-01T00:00:01.000Z",
      }),
    ],
    turns: [
      turn({
        id: "turn-a",
        session_id: "session-a" as Turn["session_id"],
        completed_at: "2026-01-01T00:00:02.000Z",
      }),
    ],
    expected: [
      { messageId: "b", durationSeconds: null, isLastPrompt: true },
      { messageId: "a", durationSeconds: 2, isLastPrompt: true },
    ],
  },
  {
    name: "bounds the earlier prompt by the later prompt when both share one turn",
    messages: [
      message({ id: "first", turn_id: TURN_ID }),
      message({ id: "second", turn_id: TURN_ID, created_at: "2026-01-01T00:00:04.000Z" }),
    ],
    turns: [turn({ completed_at: "2026-01-01T00:00:10.000Z" })],
    expected: [
      { messageId: "second", durationSeconds: 6, isLastPrompt: true },
      { messageId: "first", durationSeconds: 4, isLastPrompt: false },
    ],
  },
  {
    name: "orders equal timestamps by message id in code-unit order regardless of locale",
    messages: [message({ id: "z-0" }), message({ id: "a-1" }), message({ id: "Z-9" })],
    turns: [],
    expected: [
      { messageId: "z-0", durationSeconds: null, isLastPrompt: true },
      { messageId: "a-1", durationSeconds: 0, isLastPrompt: false },
      { messageId: "Z-9", durationSeconds: 0, isLastPrompt: false },
    ],
  },
  {
    name: "orders equal timestamps by message id and clamps negative durations",
    messages: [message({ id: "z", turn_id: TURN_ID }), message({ id: "a", turn_id: "turn-2" })],
    turns: [
      turn({ id: TURN_ID, completed_at: "2025-12-31T23:59:59.000Z" }),
      turn({ id: "turn-2", completed_at: CREATED_AT }),
    ],
    expected: [
      { messageId: "z", durationSeconds: 0, isLastPrompt: true },
      { messageId: "a", durationSeconds: 0, isLastPrompt: false },
    ],
  },
];

describe("buildPromptHistoryEntries", () => {
  it.each(ENTRY_CASES)("$name", ({ messages, turns, expected }) => {
    expect(buildPromptHistoryEntries(messages, turns)).toMatchObject(expected);
  });
  it("marks inter-task agent prompts without marking ordinary user prompts", () => {
    const entries = buildPromptHistoryEntries(
      [
        message({
          id: "agent-prompt",
          metadata: { sender_task_id: "sender-task" },
        }),
        message({ id: "user-prompt", created_at: "2026-01-01T00:00:01.000Z" }),
      ],
      [],
    );

    expect(entries[0]).toMatchObject({ messageId: "user-prompt", isAgentPrompt: false });
    expect(entries[1]).toMatchObject({ messageId: "agent-prompt", isAgentPrompt: true });
  });
});

describe("formatPromptDuration", () => {
  const units = { s: "s", m: "m", h: "h" };

  it.each([
    [-1, "0s"],
    [0, "0s"],
    [5, "5s"],
    [323, "5m 23s"],
    [3723, "1h 2m 3s"],
  ])("formats %i seconds as %s", (seconds, expected) => {
    expect(formatPromptDuration(seconds, units)).toBe(expected);
  });
});
