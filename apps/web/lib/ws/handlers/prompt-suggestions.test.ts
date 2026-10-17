import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap, SessionPromptSuggestionPayload } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import { registerPromptSuggestionHandlers } from "./prompt-suggestions";

function makeStore(session: TaskSession | undefined) {
  const setTaskSession = vi.fn();
  const state = {
    taskSessions: { items: session ? { [session.id]: session } : {} },
    setTaskSession,
  } as unknown as AppState;
  const store = { getState: () => state } as unknown as StoreApi<AppState>;
  return { store, setTaskSession };
}

function message(
  payload: SessionPromptSuggestionPayload,
): BackendMessageMap["session.prompt_suggestion"] {
  return {
    id: "m1",
    type: "notification",
    action: "session.prompt_suggestion",
    payload,
    timestamp: "2026-10-06T00:00:00Z",
  } as BackendMessageMap["session.prompt_suggestion"];
}

const payload: SessionPromptSuggestionPayload = {
  task_id: "t1",
  session_id: "s1",
  turn_id: "turn-2",
  text: "sim, corre os testes",
};

// @covers AC-UI-PROMPT-SUGGEST-002.2
describe("session.prompt_suggestion", () => {
  it("stores the suggestion in session metadata without dropping other keys", () => {
    const session = {
      id: "s1",
      task_id: "t1",
      metadata: { acp: { x: 1 } },
    } as unknown as TaskSession;
    const { store, setTaskSession } = makeStore(session);

    registerPromptSuggestionHandlers(store)["session.prompt_suggestion"]?.(message(payload));

    expect(setTaskSession).toHaveBeenCalledWith({
      ...session,
      metadata: {
        acp: { x: 1 },
        prompt_suggestion: { turn_id: "turn-2", text: "sim, corre os testes" },
      },
    });
  });

  it("ignores unknown sessions and empty suggestions", () => {
    const { store, setTaskSession } = makeStore(undefined);
    const handler = registerPromptSuggestionHandlers(store)["session.prompt_suggestion"];
    handler?.(message(payload));
    handler?.(message({ ...payload, text: "  " }));
    expect(setTaskSession).not.toHaveBeenCalled();
  });
});
