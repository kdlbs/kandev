import { beforeEach, describe, expect, it } from "vitest";
import type { Message, TaskSession, Turn } from "@/lib/types/http";
import {
  isNativeSuggestionSession,
  readNativePromptSuggestion,
  readSuggestionDismissal,
  resolveSuggestionTurnKey,
  selectVisibleSuggestion,
  writeSuggestionDismissal,
  type SuggestionSelectionInput,
} from "./prompt-suggestion";

const T0 = "2026-10-06T00:00:00Z";
const T1 = "2026-10-06T00:01:00Z";
const WAITING = "WAITING_FOR_INPUT";
const SUGGESTION = "sim, corre";

function message(
  id: string,
  author: "user" | "agent",
  content: string,
  turnId?: string,
  type = "message",
): Message {
  return {
    id,
    session_id: "s1",
    task_id: "t1",
    turn_id: turnId,
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

function session(metadata: Record<string, unknown>, state = WAITING): TaskSession {
  return { id: "s1", state, metadata } as unknown as TaskSession;
}

const nativeMetadata = {
  prompt_suggestion_source: "native",
  prompt_suggestion: { turn_id: "turn-1", text: SUGGESTION },
};

function selection(overrides: Partial<SuggestionSelectionInput> = {}): SuggestionSelectionInput {
  return {
    enabled: true,
    session: session(nativeMetadata),
    turnKey: { key: "turn-1", isTurnId: true },
    activeTurn: false,
    blocked: false,
    queued: false,
    dismissedTurnKey: null,
    ...overrides,
  };
}

// @covers AC-UI-PROMPT-SUGGEST-002.3
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
    expect(
      readNativePromptSuggestion({ prompt_suggestion: { turn_id: "t", text: "  " } }),
    ).toBeNull();
    expect(readNativePromptSuggestion({ prompt_suggestion: "x" })).toBeNull();
  });
});

describe("resolveSuggestionTurnKey", () => {
  it("uses the latest completed turn", () => {
    expect(resolveSuggestionTurnKey([turn("a", true), turn("b", true)], [])).toEqual({
      key: "b",
      isTurnId: true,
    });
  });

  it("returns null while the latest turn is still active", () => {
    expect(resolveSuggestionTurnKey([turn("a", true), turn("b", false)], [])).toBeNull();
  });

  it("uses the newest agent message's turn without turn records", () => {
    const messages = [
      message("m1", "user", "hi", "turn-1"),
      message("m2", "agent", "hello", "turn-1"),
    ];
    expect(resolveSuggestionTurnKey(undefined, messages)).toEqual({
      key: "turn-1",
      isTurnId: true,
    });
  });

  it("falls back to the newest agent message ID when it has no turn", () => {
    const messages = [message("m1", "user", "hi"), message("m2", "agent", "hello")];
    expect(resolveSuggestionTurnKey(undefined, messages)).toEqual({ key: "m2", isTurnId: false });
  });

  it("has no turn key when the user spoke last", () => {
    expect(resolveSuggestionTurnKey([], [message("m1", "user", "hi", "turn-2")])).toBeNull();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-002.2
describe("isNativeSuggestionSession", () => {
  it("is native only when the session negotiated native suggestions", () => {
    expect(isNativeSuggestionSession(session({ prompt_suggestion_source: "native" }))).toBe(true);
    expect(isNativeSuggestionSession(session({ prompt_suggestion_source: "none" }))).toBe(false);
    expect(isNativeSuggestionSession(session({}))).toBe(false);
    expect(isNativeSuggestionSession(null)).toBe(false);
  });
});

// @covers AC-UI-PROMPT-SUGGEST-001.2 AC-UI-PROMPT-SUGGEST-004.7
describe("selectVisibleSuggestion", () => {
  it("shows the native suggestion for the current turn", () => {
    expect(selectVisibleSuggestion(selection())).toEqual({ turnKey: "turn-1", text: SUGGESTION });
  });

  it.each<[string, Partial<SuggestionSelectionInput>]>([
    ["the preference is off", { enabled: false }],
    ["the session is running", { session: session(nativeMetadata, "RUNNING") }],
    ["a turn is active", { activeTurn: true }],
    ["the composer is blocked", { blocked: true }],
    ["a prompt is queued", { queued: true }],
    ["the suggestion was dismissed for its turn", { dismissedTurnKey: "turn-1" }],
    [
      "the session needs recovery",
      { session: { ...session(nativeMetadata), error_message: "boom" } as TaskSession },
    ],
  ])("shows nothing when %s", (_name, override) => {
    expect(selectVisibleSuggestion(selection(override))).toBeNull();
  });

  it("keeps showing after a dismissal of an older turn", () => {
    expect(selectVisibleSuggestion(selection({ dismissedTurnKey: "turn-0" }))?.text).toBe(
      SUGGESTION,
    );
  });

  it("trusts the backend turn binding when the newest agent message has no turn", () => {
    const metadata = {
      prompt_suggestion_source: "native",
      prompt_suggestion: { turn_id: "turn-9", text: SUGGESTION },
    };
    const input = selection({
      session: session(metadata),
      turnKey: { key: "m2", isTurnId: false },
    });
    expect(selectVisibleSuggestion(input)).toEqual({ turnKey: "turn-9", text: SUGGESTION });
  });
});

// @covers AC-UI-PROMPT-SUGGEST-003.1 AC-UI-PROMPT-SUGGEST-003.2
describe("selectVisibleSuggestion without native support", () => {
  it("never shows a suggestion payload on a session that did not negotiate native support", () => {
    const metadata = { prompt_suggestion: { turn_id: "turn-1", text: SUGGESTION } };
    expect(selectVisibleSuggestion(selection({ session: session(metadata) }))).toBeNull();
    const none = { ...metadata, prompt_suggestion_source: "none" };
    expect(selectVisibleSuggestion(selection({ session: session(none) }))).toBeNull();
  });

  it("shows nothing when a native session sent no suggestion for the turn", () => {
    const metadata = { prompt_suggestion_source: "native" };
    expect(selectVisibleSuggestion(selection({ session: session(metadata) }))).toBeNull();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-003.3
describe("selectVisibleSuggestion with a stale suggestion", () => {
  it("hides a suggestion bound to an older turn", () => {
    expect(
      selectVisibleSuggestion(selection({ turnKey: { key: "turn-2", isTurnId: true } })),
    ).toBeNull();
  });

  it("hides a suggestion from an older turn after a reload without turn records", () => {
    const messages = [
      message("m1", "user", "go on", "turn-2"),
      message("m2", "agent", "done", "turn-2"),
    ];
    const turnKey = resolveSuggestionTurnKey(undefined, messages);
    expect(selectVisibleSuggestion(selection({ turnKey }))).toBeNull();
  });

  it("hides a suggestion while the next turn has not completed", () => {
    expect(selectVisibleSuggestion(selection({ turnKey: null }))).toBeNull();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-004.6
describe("suggestion dismissals", () => {
  beforeEach(() => window.localStorage.clear());

  it("stores one dismissal per session and replaces it on a newer turn", () => {
    writeSuggestionDismissal({ sessionId: "s1", turnKey: "a" });
    writeSuggestionDismissal({ sessionId: "s1", turnKey: "b" });
    expect(readSuggestionDismissal("s1")).toEqual({ sessionId: "s1", turnKey: "b" });
  });

  it("keeps at most 50 sessions", () => {
    for (let i = 0; i < 55; i += 1) {
      writeSuggestionDismissal({ sessionId: `s${i}`, turnKey: "t" });
    }
    expect(readSuggestionDismissal("s0")).toBeNull();
    expect(readSuggestionDismissal("s54")?.turnKey).toBe("t");
  });

  it("ignores malformed storage", () => {
    window.localStorage.setItem("kandev.promptSuggestionDismissals.v1", "{not json");
    expect(readSuggestionDismissal("s1")).toBeNull();
    window.localStorage.setItem(
      "kandev.promptSuggestionDismissals.v1",
      JSON.stringify([{ sessionId: "s1" }]),
    );
    expect(readSuggestionDismissal("s1")).toBeNull();
  });
});
