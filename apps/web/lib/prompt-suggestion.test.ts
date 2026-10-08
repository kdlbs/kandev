import { beforeEach, describe, expect, it } from "vitest";
import type { Message, TaskSession, Turn } from "@/lib/types/http";
import {
  buildSuggestionTranscript,
  isNativeSuggestionSession,
  normalizeFallbackSuggestion,
  readNativePromptSuggestion,
  readSuggestionCache,
  resolveSuggestionTurnKey,
  shouldRequestFallback,
  writeSuggestionCache,
  type FallbackEligibility,
} from "./prompt-suggestion";

const T0 = "2026-10-06T00:00:00Z";
const T1 = "2026-10-06T00:01:00Z";

function message(id: string, author: "user" | "agent", content: string, type = "message"): Message {
  return {
    id,
    session_id: "s1",
    task_id: "t1",
    author_type: author,
    content,
    type,
    created_at: T0,
  } as Message;
}

function turn(id: string, completed: boolean): Turn {
  return {
    id,
    session_id: "s1",
    task_id: "t1",
    started_at: T0,
    completed_at: completed ? T1 : undefined,
    created_at: T0,
    updated_at: T0,
  } as Turn;
}

// @covers AC-UI-PROMPT-SUGGEST-002.2
describe("readNativePromptSuggestion", () => {
  it("reads the stored native suggestion", () => {
    expect(
      readNativePromptSuggestion({ prompt_suggestion: { turn_id: "turn-2", text: "sim" } }),
    ).toEqual({ turnKey: "turn-2", text: "sim" });
  });

  it("ignores missing or malformed metadata", () => {
    expect(readNativePromptSuggestion(undefined)).toBeNull();
    expect(
      readNativePromptSuggestion({ prompt_suggestion: { turn_id: "", text: "x" } }),
    ).toBeNull();
    expect(readNativePromptSuggestion({ prompt_suggestion: "x" })).toBeNull();
  });
});

describe("resolveSuggestionTurnKey", () => {
  it("uses the latest completed turn", () => {
    expect(resolveSuggestionTurnKey([turn("a", true), turn("b", true)], [])).toBe("b");
  });

  it("returns null while the latest turn is still active", () => {
    expect(resolveSuggestionTurnKey([turn("a", true), turn("b", false)], [])).toBeNull();
  });

  it("falls back to the newest agent message without turn records", () => {
    const messages = [message("m1", "user", "hi"), message("m2", "agent", "hello")];
    expect(resolveSuggestionTurnKey(undefined, messages)).toBe("m2");
    expect(resolveSuggestionTurnKey([], [message("m1", "user", "hi")])).toBeNull();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-003.1
describe("buildSuggestionTranscript", () => {
  it("keeps only the latest exchange", () => {
    const transcript = buildSuggestionTranscript([
      message("m1", "user", "old question"),
      message("m2", "agent", "old answer"),
      message("m3", "user", "adiciona a migration"),
      message("m4", "agent", "tool output", "tool_call"),
      message("m5", "agent", "Feito. Corro os testes?"),
    ]);
    expect(transcript).toBe("User: adiciona a migration\n\nAgent: Feito. Corro os testes?");
  });

  it("caps the user prompt and keeps the tail of a long agent reply", () => {
    const transcript = buildSuggestionTranscript([
      message("m1", "user", "u".repeat(3000)),
      message("m2", "agent", `${"a".repeat(8000)}QUESTION?`),
    ]);
    expect(transcript.length).toBeLessThanOrEqual(6000);
    expect(transcript.startsWith(`User: ${"u".repeat(1500)}`)).toBe(true);
    expect(transcript.endsWith("QUESTION?")).toBe(true);
  });
});

// @covers AC-UI-PROMPT-SUGGEST-003.6
describe("normalizeFallbackSuggestion", () => {
  it.each([
    ["  run the tests  ", "run the tests"],
    ['"commit this"', "commit this"],
    ["\u201ccommit this\u201d", "commit this"],
    ["<suggestion>push it</suggestion>", "push it"],
    ["Suggestion: sim, corre os testes", "sim, corre os testes"],
  ])("accepts %j", (raw, want) => {
    expect(normalizeFallbackSuggestion(raw)).toBe(want);
  });

  it.each([
    "",
    "NONE",
    "No suggestion.",
    "(silence)",
    "one two three four five six seven eight nine ten eleven twelve thirteen",
    "x".repeat(100),
    "Run the tests. Then commit.",
    "first line\nsecond line",
    "**bold** move",
    "Looks good",
    "Thanks!",
    "Let me check",
    "I'll do it",
  ])("rejects %j", (raw) => {
    expect(normalizeFallbackSuggestion(raw)).toBeNull();
  });
});

const eligible: FallbackEligibility = {
  enabled: true,
  fallbackEnabled: true,
  native: false,
  sessionState: "WAITING_FOR_INPUT",
  needsRecovery: false,
  activeTurn: false,
  pendingClarification: false,
  queued: false,
  lastEntryIsAgent: true,
  turnKey: "turn-1",
  cached: false,
  inFlight: false,
};

// @covers AC-UI-PROMPT-SUGGEST-001.2 AC-UI-PROMPT-SUGGEST-002.5 AC-UI-PROMPT-SUGGEST-003.3
describe("shouldRequestFallback", () => {
  it("requests for an eligible non-native session", () => {
    expect(shouldRequestFallback(eligible)).toBe(true);
  });

  it.each<[string, Partial<FallbackEligibility>]>([
    ["preference off", { enabled: false }],
    ["fallback off", { fallbackEnabled: false }],
    ["native session", { native: true }],
    ["busy", { sessionState: "RUNNING" }],
    ["starting", { sessionState: "STARTING" }],
    ["needs recovery", { needsRecovery: true }],
    ["active turn", { activeTurn: true }],
    ["clarification pending", { pendingClarification: true }],
    ["queued prompt", { queued: true }],
    ["user spoke last", { lastEntryIsAgent: false }],
    ["no turn key", { turnKey: null }],
    ["already cached", { cached: true }],
    ["already in flight", { inFlight: true }],
  ])("skips when %s", (_name, override) => {
    expect(shouldRequestFallback({ ...eligible, ...override })).toBe(false);
  });
});

// @covers AC-UI-PROMPT-SUGGEST-003.2
describe("suggestion cache", () => {
  beforeEach(() => window.localStorage.clear());

  it("stores one entry per session and replaces it on a newer turn", () => {
    writeSuggestionCache({ sessionId: "s1", turnKey: "a", text: "x", dismissed: false });
    writeSuggestionCache({ sessionId: "s1", turnKey: "b", text: null, dismissed: false });
    expect(readSuggestionCache("s1")).toEqual({
      sessionId: "s1",
      turnKey: "b",
      text: null,
      dismissed: false,
    });
  });

  it("keeps at most 50 sessions", () => {
    for (let i = 0; i < 55; i += 1) {
      writeSuggestionCache({ sessionId: `s${i}`, turnKey: "t", text: "x", dismissed: false });
    }
    expect(readSuggestionCache("s0")).toBeNull();
    expect(readSuggestionCache("s54")?.text).toBe("x");
  });

  it("ignores malformed storage", () => {
    window.localStorage.setItem("kandev.promptSuggestion.v1", "{not json");
    expect(readSuggestionCache("s1")).toBeNull();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-002.5
describe("isNativeSuggestionSession", () => {
  it("is native only when the session negotiated native suggestions", () => {
    const native = { metadata: { prompt_suggestion_source: "native" } } as unknown as TaskSession;
    const none = { metadata: { prompt_suggestion_source: "none" } } as unknown as TaskSession;
    expect(isNativeSuggestionSession(native)).toBe(true);
    expect(isNativeSuggestionSession(none)).toBe(false);
    expect(isNativeSuggestionSession({ metadata: {} } as unknown as TaskSession)).toBe(false);
    expect(isNativeSuggestionSession(null)).toBe(false);
  });
});
