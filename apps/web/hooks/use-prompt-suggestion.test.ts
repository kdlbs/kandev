import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import type { Message, TaskSession, Turn } from "@/lib/types/http";
import { readSuggestionDismissal } from "@/lib/prompt-suggestion";
import {
  usePromptSuggestionController,
  type PromptSuggestionInputs,
} from "./use-prompt-suggestion";

const T0 = "2026-10-06T00:00:00Z";
const T1 = "2026-10-06T00:01:00Z";
const WAITING = "WAITING_FOR_INPUT";
const SUGGESTION = "sim, corre";

function message(id: string, author: "user" | "agent", turnId: string): Message {
  return {
    id,
    session_id: "s1",
    task_id: "t1",
    turn_id: turnId,
    author_type: author,
    content: id,
    type: "message",
    created_at: T0,
  } as Message;
}

function turn(id: string, completed = true): Turn {
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

function nativeSession(turnId = "turn-1", state = WAITING): TaskSession {
  return {
    id: "s1",
    state,
    metadata: {
      prompt_suggestion_source: "native",
      prompt_suggestion: { turn_id: turnId, text: SUGGESTION },
    },
  } as unknown as TaskSession;
}

function inputs(overrides: Partial<PromptSuggestionInputs> = {}): PromptSuggestionInputs {
  return {
    sessionId: "s1",
    enabled: true,
    session: nativeSession(),
    turns: [turn("turn-1")],
    activeTurnId: null,
    messages: [message("m1", "user", "turn-1"), message("m2", "agent", "turn-1")],
    queued: false,
    blocked: false,
    ...overrides,
  };
}

beforeEach(() => window.localStorage.clear());

// @covers AC-UI-PROMPT-SUGGEST-002.3 AC-UI-PROMPT-SUGGEST-001.4
describe("native suggestions", () => {
  it("shows the stored native suggestion for the current turn", () => {
    const { result } = renderHook(() => usePromptSuggestionController(inputs()));
    expect(result.current.suggestion).toBe(SUGGESTION);
  });

  it("follows the preference immediately", () => {
    const { result, rerender } = renderHook(
      (props: PromptSuggestionInputs) => usePromptSuggestionController(props),
      {
        initialProps: inputs(),
      },
    );
    rerender(inputs({ enabled: false }));
    expect(result.current.suggestion).toBeNull();
    rerender(inputs({ enabled: true }));
    expect(result.current.suggestion).toBe(SUGGESTION);
  });

  it("restores the suggestion from metadata and messages when turns are not loaded", () => {
    const { result } = renderHook(() =>
      usePromptSuggestionController(inputs({ turns: undefined })),
    );
    expect(result.current.suggestion).toBe(SUGGESTION);
  });
});

// @covers AC-UI-PROMPT-SUGGEST-003.1 AC-UI-PROMPT-SUGGEST-003.2
describe("sessions without native support", () => {
  it("never shows a suggestion payload on a session that did not negotiate native support", () => {
    const session = {
      id: "s1",
      state: WAITING,
      metadata: { prompt_suggestion: { turn_id: "turn-1", text: SUGGESTION } },
    } as unknown as TaskSession;
    const { result } = renderHook(() => usePromptSuggestionController(inputs({ session })));
    expect(result.current.suggestion).toBeNull();
  });

  it("shows nothing when the native agent sent no suggestion", () => {
    const session = {
      id: "s1",
      state: WAITING,
      metadata: { prompt_suggestion_source: "native" },
    } as unknown as TaskSession;
    const { result } = renderHook(() => usePromptSuggestionController(inputs({ session })));
    expect(result.current.suggestion).toBeNull();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-003.3 AC-UI-PROMPT-SUGGEST-004.7
describe("stale suggestions", () => {
  it("disappears when a new turn starts and stays hidden after it completes without one", () => {
    const { result, rerender } = renderHook(
      (props: PromptSuggestionInputs) => usePromptSuggestionController(props),
      {
        initialProps: inputs(),
      },
    );
    expect(result.current.suggestion).toBe(SUGGESTION);

    const running = nativeSession("turn-1", "RUNNING");
    rerender(
      inputs({
        session: running,
        turns: [turn("turn-1"), turn("turn-2", false)],
        activeTurnId: "turn-2",
      }),
    );
    expect(result.current.suggestion).toBeNull();

    rerender(inputs({ turns: [turn("turn-1"), turn("turn-2")] }));
    expect(result.current.suggestion).toBeNull();
  });

  it("hides an older suggestion after a reload that has only messages", () => {
    const messages = [
      message("m1", "user", "turn-1"),
      message("m2", "agent", "turn-1"),
      message("m3", "user", "turn-2"),
      message("m4", "agent", "turn-2"),
    ];
    const { result } = renderHook(() =>
      usePromptSuggestionController(inputs({ turns: undefined, messages })),
    );
    expect(result.current.suggestion).toBeNull();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-004.6
describe("dismiss", () => {
  it("hides the suggestion for its turn and persists the dismissal", () => {
    const { result } = renderHook(() => usePromptSuggestionController(inputs()));
    act(() => result.current.dismiss());
    expect(result.current.suggestion).toBeNull();
    expect(readSuggestionDismissal("s1")).toEqual({ sessionId: "s1", turnKey: "turn-1" });
  });

  it("keeps the dismissal after a remount", () => {
    const first = renderHook(() => usePromptSuggestionController(inputs()));
    act(() => first.result.current.dismiss());
    first.unmount();
    const { result } = renderHook(() => usePromptSuggestionController(inputs()));
    expect(result.current.suggestion).toBeNull();
  });

  it("shows the next turn's suggestion after a dismissal", () => {
    const { result, rerender } = renderHook(
      (props: PromptSuggestionInputs) => usePromptSuggestionController(props),
      {
        initialProps: inputs(),
      },
    );
    act(() => result.current.dismiss());
    rerender(inputs({ session: nativeSession("turn-2"), turns: [turn("turn-1"), turn("turn-2")] }));
    expect(result.current.suggestion).toBe(SUGGESTION);
  });
});
