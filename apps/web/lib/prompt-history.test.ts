import { describe, expect, it } from "vitest";
import type { Message, Turn } from "@/lib/types/http";
import {
  buildPromptHistoryEntries,
  formatPromptDuration,
  messageTurnDurationSeconds,
} from "./prompt-history";

const MESSAGE_ID = "message-1";
const TURN_ID = "turn-1";
const CREATED_AT = "2026-01-01T00:00:00.000Z";
const ONE_SECOND_LATER = "2026-01-01T00:00:01.000Z";
const FOUR_SECONDS_LATER = "2026-01-01T00:00:04.000Z";
const FIVE_SECONDS_LATER = "2026-01-01T00:00:05.000Z";
const INVALID_TIMESTAMP = "not-a-time";

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
    turns: [turn({ id: "undefined", completed_at: FIVE_SECONDS_LATER })],
    expected: [{ messageId: "no-turn-undefined", durationSeconds: null, isLastPrompt: true }],
  },
  {
    name: "uses a following prompt when turn id is absent or dangling",
    messages: [
      message({ id: "missing", created_at: CREATED_AT }),
      message({
        id: "dangling",
        turn_id: "not-found",
        created_at: ONE_SECOND_LATER,
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
      message({ id: "invalid", created_at: INVALID_TIMESTAMP }),
      message({ id: "next", created_at: FOUR_SECONDS_LATER }),
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
    turns: [turn({ completed_at: INVALID_TIMESTAMP })],
    expected: [{ messageId: "no-bound", durationSeconds: null, isLastPrompt: true }],
  },
  {
    name: "lets the next-prompt bound win when the turn completed_at is unparseable",
    messages: [
      message({ id: "bad-turn", turn_id: "turn-1" }),
      message({ id: "later", created_at: FOUR_SECONDS_LATER }),
    ],
    turns: [turn({ completed_at: INVALID_TIMESTAMP })],
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
        created_at: ONE_SECOND_LATER,
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
      message({ id: "second", turn_id: TURN_ID, created_at: FOUR_SECONDS_LATER }),
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
        message({ id: "user-prompt", created_at: ONE_SECOND_LATER }),
      ],
      [],
    );

    expect(entries[0]).toMatchObject({ messageId: "user-prompt", isAgentPrompt: false });
    expect(entries[1]).toMatchObject({ messageId: "agent-prompt", isAgentPrompt: true });
  });
  it("carries promptNumber from prompt_index regardless of input order", () => {
    const entries = buildPromptHistoryEntries(
      [
        message({ id: "second", prompt_index: 2, created_at: ONE_SECOND_LATER }),
        message({ id: "first", prompt_index: 1 }),
      ],
      [],
    );

    expect(entries[0]).toMatchObject({ messageId: "second", promptNumber: 2 });
    expect(entries[1]).toMatchObject({ messageId: "first", promptNumber: 1 });
  });
  it("treats absent, 0, and undefined prompt_index as null", () => {
    const entries = buildPromptHistoryEntries(
      [
        message({ id: "absent" }),
        message({ id: "zero", prompt_index: 0, created_at: ONE_SECOND_LATER }),
        message({
          id: "undefined",
          prompt_index: undefined,
          created_at: "2026-01-01T00:00:02.000Z",
        }),
      ],
      [],
    );

    expect(entries.every((entry) => entry.promptNumber === null)).toBe(true);
  });
  it("orders by microsecond-truncated timestamp then id when only digits 7-9 differ", () => {
    // Both timestamps share the microsecond .123456; full nanoseconds differ.
    // The later instant has the EARLIER id, so (microseconds, id) ordering
    // puts it first — and the earlier instant's duration uses full-nanosecond
    // subtraction (floor(788ns / 1e9) = 0 seconds, never negative).
    const entries = buildPromptHistoryEntries(
      [
        message({
          id: "b",
          created_at: "2026-01-01T00:00:00.123456789Z",
          turn_id: "turn-b",
        }),
        message({ id: "a", created_at: "2026-01-01T00:00:00.123456001Z" }),
      ],
      [turn({ id: "turn-b", completed_at: "2026-01-01T00:00:01.000000000Z" })],
    );

    expect(entries.map((entry) => entry.messageId)).toEqual(["b", "a"]);
    expect(entries[1]).toMatchObject({ messageId: "a", durationSeconds: 0 });
  });
  it("orders mixed-width fractions, offsets, and exact seconds by UTC microseconds", () => {
    const entries = buildPromptHistoryEntries(
      [
        message({ id: "ns-fraction", created_at: "2026-01-01T00:00:00.123456789Z" }),
        // .1Z = 100ms = .100000000s — same microsecond as .100000Z, no id tie
        // here because the microseconds differ from ns-fraction.
        message({ id: "tenth", created_at: "2026-01-01T00:00:00.1Z" }),
        // Exact second: no fraction at all.
        message({ id: "exact", created_at: "2026-01-01T00:00:00Z" }),
        // Offset +01:00 == 00:00:00.123456 UTC — same microsecond as
        // ns-fraction's truncated key, so the id tie-break decides.
        message({ id: "offset-aaa", created_at: "2026-01-01T01:00:00.123456+01:00" }),
      ],
      [],
    );

    // (microseconds, id) ascending: exact (.000000), tenth (.100000),
    // ns-fraction and offset-aaa share .123456 → id order ("ns-fraction" <
    // "offset-aaa" in code units) puts ns-fraction first.
    expect(entries.map((entry) => entry.messageId)).toEqual([
      "offset-aaa",
      "ns-fraction",
      "tenth",
      "exact",
    ]);
    // Entries are newest-first. Every row except the newest is bounded by the
    // next prompt: exact (.000000) is bounded by tenth (.1Z = 100ms later),
    // flooring to 0 seconds; the newest (offset-aaa) has no later bound and
    // no turn, so it shows no duration.
    expect(entries[0]).toMatchObject({ messageId: "offset-aaa", durationSeconds: null });
    expect(entries[1]).toMatchObject({ messageId: "ns-fraction", durationSeconds: 0 });
    expect(entries[2]).toMatchObject({ messageId: "tenth", durationSeconds: 0 });
    expect(entries[3]).toMatchObject({ messageId: "exact", durationSeconds: 0 });
  });
  it("keeps a running newest prompt without a duration while clamping overlaps", () => {
    // A reversed pair (later prompt's turn completed BEFORE the earlier
    // prompt's send time) clamps to zero; the newest prompt with a running
    // turn and no later prompt shows no duration.
    const entries = buildPromptHistoryEntries(
      [
        message({
          id: "newest",
          turn_id: "turn-newest",
          created_at: "2026-01-01T00:00:01.000000000Z",
        }),
        message({ id: "older", turn_id: "turn-older" }),
      ],
      [
        turn({ id: "turn-newest" }),
        turn({ id: "turn-older", completed_at: "2025-12-31T23:59:59.000000000Z" }),
      ],
    );

    expect(entries[0]).toMatchObject({ messageId: "newest", durationSeconds: null });
    expect(entries[1]).toMatchObject({ messageId: "older", durationSeconds: 0 });
  });
});

describe("buildPromptHistoryEntries — precision edges", () => {
  it("floors the microsecond key for pre-epoch timestamps (never truncates toward zero)", () => {
    // 1969-12-31T23:59:59.999999999Z is -1ns from epoch: the microsecond key
    // must be -1 (floor), NOT 0 — otherwise it ties with the first post-epoch
    // microsecond and the id tie-break would misorder the pair.
    const entries = buildPromptHistoryEntries(
      [
        message({ id: "post-epoch", created_at: "1970-01-01T00:00:00.000000001Z" }),
        message({ id: "pre-epoch", created_at: "1969-12-31T23:59:59.999999999Z" }),
      ],
      [],
    );

    expect(entries.map((entry) => entry.messageId)).toEqual(["post-epoch", "pre-epoch"]);
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

describe("messageTurnDurationSeconds", () => {
  it.each([
    {
      name: "floors completed turn duration to whole seconds",
      message: message({ turn_id: TURN_ID }),
      turn: turn({ completed_at: "2026-01-01T00:00:05.900Z" }),
      expected: 5,
    },
    {
      name: "keeps whole-second completed turn duration",
      message: message({ turn_id: TURN_ID }),
      turn: turn({ completed_at: FIVE_SECONDS_LATER }),
      expected: 5,
    },
    {
      name: "floors sub-second completed turn duration to zero",
      message: message({ turn_id: TURN_ID }),
      turn: turn({ completed_at: "2026-01-01T00:00:00.999Z" }),
      expected: 0,
    },
    {
      name: "returns null for a running turn",
      message: message({ turn_id: TURN_ID }),
      turn: turn({}),
      expected: null,
    },
    {
      name: "returns null for an unparseable completion timestamp",
      message: message({ turn_id: TURN_ID }),
      turn: turn({ completed_at: INVALID_TIMESTAMP }),
      expected: null,
    },
    {
      name: "returns null for an unparseable prompt timestamp",
      message: message({ turn_id: TURN_ID, created_at: INVALID_TIMESTAMP }),
      turn: turn({ completed_at: FIVE_SECONDS_LATER }),
      expected: null,
    },
    {
      name: "returns null for an agent message",
      message: message({ author_type: "agent", turn_id: TURN_ID }),
      turn: turn({ completed_at: FIVE_SECONDS_LATER }),
      expected: null,
    },
    {
      name: "returns null for a missing turn id",
      message: message({}),
      turn: turn({ completed_at: FIVE_SECONDS_LATER }),
      expected: null,
    },
    {
      name: "returns null for an empty turn id",
      message: message({ turn_id: "" }),
      turn: turn({ completed_at: FIVE_SECONDS_LATER }),
      expected: null,
    },
    {
      name: "returns null for an absent turn",
      message: message({ turn_id: TURN_ID }),
      turn: null,
      expected: null,
    },
    {
      name: "returns null for a mismatched turn id",
      message: message({ turn_id: TURN_ID }),
      turn: turn({ id: "different", completed_at: FIVE_SECONDS_LATER }),
      expected: null,
    },
    {
      name: "returns null for a mismatched turn session",
      message: message({ turn_id: TURN_ID }),
      turn: turn({
        session_id: "different-session" as Turn["session_id"],
        completed_at: FIVE_SECONDS_LATER,
      }),
      expected: null,
    },
    {
      name: "clamps clock skew to zero",
      message: message({ turn_id: TURN_ID }),
      turn: turn({ completed_at: "2025-12-31T23:59:59.000Z" }),
      expected: 0,
    },
  ])("$name", ({ message: prompt, turn: completedTurn, expected }) => {
    expect(messageTurnDurationSeconds(prompt, completedTurn)).toBe(expected);
  });

  it("measures shared completed turns from each prompt's own timestamp", () => {
    const completedTurn = turn({ completed_at: "2026-01-01T00:00:10.000Z" });

    expect(messageTurnDurationSeconds(message({ turn_id: TURN_ID }), completedTurn)).toBe(10);
    expect(
      messageTurnDurationSeconds(
        message({ id: "second", turn_id: TURN_ID, created_at: FOUR_SECONDS_LATER }),
        completedTurn,
      ),
    ).toBe(6);
  });
});
