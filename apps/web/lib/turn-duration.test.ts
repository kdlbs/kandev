import { describe, expect, it } from "vitest";
import type { Message, Turn } from "@/lib/types/http";
import { formatPromptDuration, messageTurnDurationSeconds } from "./turn-duration";

const MESSAGE_ID = "message-1";
const TURN_ID = "turn-1";
const CREATED_AT = "2026-01-01T00:00:00.000Z";
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
