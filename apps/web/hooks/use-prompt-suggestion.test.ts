import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { ExecutePromptResponse } from "@/lib/api/domains/utility-api";
import type { Message, TaskSession, Turn } from "@/lib/types/http";
import { PROMPT_SUGGESTION_TIMEOUT_MS, readSuggestionCache } from "@/lib/prompt-suggestion";
import {
  usePromptSuggestionController,
  type PromptSuggestionInputs,
  type PromptSuggestionRequester,
} from "./use-prompt-suggestion";

const T0 = "2026-10-06T00:00:00Z";
const T1 = "2026-10-06T00:01:00Z";
const WAITING = "WAITING_FOR_INPUT";
const SUGGESTION = "sim, corre";

function message(id: string, author: "user" | "agent", content: string): Message {
  return {
    id,
    session_id: "s1",
    task_id: "t1",
    author_type: author,
    content,
    type: "message",
    created_at: T0,
  } as Message;
}

function turn(id: string): Turn {
  return {
    id,
    session_id: "s1",
    task_id: "t1",
    started_at: T0,
    completed_at: T1,
    created_at: T0,
    updated_at: T0,
  } as Turn;
}

function inputs(overrides: Partial<PromptSuggestionInputs> = {}): PromptSuggestionInputs {
  return {
    sessionId: "s1",
    taskTitle: "Add tags",
    enabled: true,
    fallbackEnabled: true,
    native: false,
    session: { id: "s1", state: WAITING, metadata: {} } as unknown as TaskSession,
    turns: [turn("turn-1")],
    activeTurnId: null,
    messages: [
      message("m1", "user", "adiciona a migration"),
      message("m2", "agent", "Corro os testes?"),
    ],
    queued: false,
    blocked: false,
    ...overrides,
  };
}

function okResponse(response: string): ExecutePromptResponse {
  return { success: true, response } as ExecutePromptResponse;
}

beforeEach(() => window.localStorage.clear());
afterEach(() => vi.useRealTimers());

// @covers AC-UI-PROMPT-SUGGEST-002.2 AC-UI-PROMPT-SUGGEST-002.5
describe("native suggestions", () => {
  it("shows the stored native suggestion for the current turn without a utility request", () => {
    const request = vi.fn<PromptSuggestionRequester>();
    const session = {
      id: "s1",
      state: WAITING,
      metadata: { prompt_suggestion: { turn_id: "turn-1", text: SUGGESTION } },
    } as unknown as TaskSession;
    const { result } = renderHook(() =>
      usePromptSuggestionController(inputs({ native: true, session }), request),
    );
    expect(result.current.suggestion).toBe(SUGGESTION);
    expect(request).not.toHaveBeenCalled();
  });

  it("trusts the backend turn binding when no turns are loaded", () => {
    const session = {
      id: "s1",
      state: WAITING,
      metadata: { prompt_suggestion: { turn_id: "turn-9", text: SUGGESTION } },
    } as unknown as TaskSession;
    const { result } = renderHook(() =>
      usePromptSuggestionController(inputs({ native: true, session, turns: undefined }), vi.fn()),
    );
    expect(result.current.suggestion).toBe(SUGGESTION);
    act(() => result.current.dismiss());
    expect(result.current.suggestion).toBeNull();
    expect(readSuggestionCache("s1")).toMatchObject({ turnKey: "turn-9", dismissed: true });
  });

  it("hides a native suggestion from an older turn", () => {
    const session = {
      id: "s1",
      state: WAITING,
      metadata: { prompt_suggestion: { turn_id: "turn-0", text: "old" } },
    } as unknown as TaskSession;
    const { result } = renderHook(() =>
      usePromptSuggestionController(inputs({ native: true, session }), vi.fn()),
    );
    expect(result.current.suggestion).toBeNull();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-003.1 AC-UI-PROMPT-SUGGEST-003.2
describe("utility fallback", () => {
  it("requests once with the latest exchange and caches the result", async () => {
    const request = vi.fn<PromptSuggestionRequester>().mockResolvedValue(okResponse(SUGGESTION));
    const { result, rerender } = renderHook(() => usePromptSuggestionController(inputs(), request));

    await waitFor(() => expect(result.current.suggestion).toBe(SUGGESTION));
    rerender();
    expect(request).toHaveBeenCalledTimes(1);
    expect(request.mock.calls[0][0]).toEqual({
      utility_agent_id: "builtin-suggest-next-prompt",
      session_id: "",
      task_title: "Add tags",
      conversation_history: "User: adiciona a migration\n\nAgent: Corro os testes?",
    });
    expect(readSuggestionCache("s1")).toMatchObject({ turnKey: "turn-1", text: SUGGESTION });
  });

  // @covers AC-UI-PROMPT-SUGGEST-003.9
  it("sends the session's agent profile as the last-resort fallback profile", async () => {
    const request = vi.fn<PromptSuggestionRequester>().mockResolvedValue(okResponse("sim"));
    const session = {
      id: "s1",
      state: WAITING,
      agent_profile_id: "profile-session",
      metadata: {},
    } as unknown as TaskSession;
    renderHook(() => usePromptSuggestionController(inputs({ session }), request));
    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expect(request.mock.calls[0][0]).toMatchObject({
      fallback_agent_profile_id: "profile-session",
    });
  });

  it("reuses the cached result after a remount", () => {
    window.localStorage.setItem(
      "kandev.promptSuggestion.v1",
      JSON.stringify([{ sessionId: "s1", turnKey: "turn-1", text: "cached", dismissed: false }]),
    );
    const request = vi.fn<PromptSuggestionRequester>();
    const { result } = renderHook(() => usePromptSuggestionController(inputs(), request));
    expect(result.current.suggestion).toBe("cached");
    expect(request).not.toHaveBeenCalled();
  });

  // @covers AC-UI-PROMPT-SUGGEST-003.7
  it("shows nothing and caches null when the server rejects the request", async () => {
    const request = vi
      .fn<PromptSuggestionRequester>()
      .mockRejectedValue(new ApiError("boom", 500, null));
    const { result } = renderHook(() => usePromptSuggestionController(inputs(), request));
    await waitFor(() => expect(readSuggestionCache("s1")).toMatchObject({ text: null }));
    expect(result.current.suggestion).toBeNull();
  });

  // @covers AC-UI-PROMPT-SUGGEST-003.7
  it("does not cache a request that never reached the server", async () => {
    const request = vi
      .fn<PromptSuggestionRequester>()
      .mockRejectedValue(new DOMException("aborted", "AbortError"));
    const { result } = renderHook(() => usePromptSuggestionController(inputs(), request));
    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    await act(async () => undefined);
    expect(readSuggestionCache("s1")).toBeNull();
    expect(result.current.suggestion).toBeNull();
  });

  // @covers AC-UI-PROMPT-SUGGEST-003.4
  it("abandons a request that exceeds the time budget", async () => {
    vi.useFakeTimers();
    let signal: AbortSignal | undefined;
    const request = vi.fn<PromptSuggestionRequester>((_req, s) => {
      signal = s;
      return new Promise(() => undefined);
    });
    const { result } = renderHook(() => usePromptSuggestionController(inputs(), request));
    expect(request).toHaveBeenCalledTimes(1);
    act(() => {
      vi.advanceTimersByTime(PROMPT_SUGGESTION_TIMEOUT_MS);
    });
    expect(signal?.aborted).toBe(true);
    expect(readSuggestionCache("s1")).toMatchObject({ turnKey: "turn-1", text: null });
    expect(result.current.suggestion).toBeNull();
  });

  // @covers AC-UI-PROMPT-SUGGEST-004.7
  it("drops a response that arrives after the turn changed", async () => {
    let resolve: (value: ExecutePromptResponse) => void = () => undefined;
    const request = vi.fn<PromptSuggestionRequester>(() => new Promise((r) => (resolve = r)));
    const { result, rerender } = renderHook(
      (props: PromptSuggestionInputs) => usePromptSuggestionController(props, request),
      { initialProps: inputs() },
    );
    rerender(
      inputs({ session: { id: "s1", state: "RUNNING", metadata: {} } as unknown as TaskSession }),
    );
    await act(async () => resolve(okResponse("late")));
    rerender(inputs({ turns: [turn("turn-1"), turn("turn-2")] }));
    expect(result.current.suggestion).not.toBe("late");
  });

  it("does not request for a native session or while blocked", () => {
    const request = vi.fn<PromptSuggestionRequester>();
    renderHook(() => usePromptSuggestionController(inputs({ native: true }), request));
    renderHook(() => usePromptSuggestionController(inputs({ blocked: true }), request));
    renderHook(() => usePromptSuggestionController(inputs({ fallbackEnabled: false }), request));
    expect(request).not.toHaveBeenCalled();
  });
});

// @covers AC-UI-PROMPT-SUGGEST-004.6
describe("dismiss", () => {
  it("hides the suggestion for its turn and persists the dismissal", () => {
    window.localStorage.setItem(
      "kandev.promptSuggestion.v1",
      JSON.stringify([{ sessionId: "s1", turnKey: "turn-1", text: "cached", dismissed: false }]),
    );
    const { result } = renderHook(() => usePromptSuggestionController(inputs(), vi.fn()));
    act(() => result.current.dismiss());
    expect(result.current.suggestion).toBeNull();
    expect(readSuggestionCache("s1")).toMatchObject({ dismissed: true });
  });

  it("hides everything when the preference is off", () => {
    window.localStorage.setItem(
      "kandev.promptSuggestion.v1",
      JSON.stringify([{ sessionId: "s1", turnKey: "turn-1", text: "cached", dismissed: false }]),
    );
    const { result } = renderHook(() =>
      usePromptSuggestionController(inputs({ enabled: false }), vi.fn()),
    );
    expect(result.current.suggestion).toBeNull();
  });
});
