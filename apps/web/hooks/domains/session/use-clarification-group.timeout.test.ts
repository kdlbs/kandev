import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));

const mockUpdateMessage = vi.fn();
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({ updateMessage: mockUpdateMessage }),
  }),
}));

import { useClarificationGroup } from "./use-clarification-group";

function clarMessage(): Message {
  return {
    id: "m1",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "Q",
    type: "clarification_request",
    created_at: "2026-05-04T00:00:00Z",
    metadata: {
      pending_id: "p1",
      question_id: "q1",
      question_index: 0,
      question_total: 1,
      status: "pending",
      question: { id: "q1", prompt: "Q?" },
    },
  };
}

const fetchMock = vi.fn();

function setupFetchMock() {
  fetchMock.mockReset();
  mockUpdateMessage.mockReset();
  fetchMock.mockResolvedValue(new Response(null, { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
}

describe("useClarificationGroup — bounded submission", () => {
  beforeEach(setupFetchMock);

  it("aborts a non-resolving submit after 40 seconds, preserves answers, and releases retry", async () => {
    vi.useFakeTimers();
    try {
      let signal: AbortSignal | undefined;
      fetchMock.mockImplementationOnce((_url: string, init: RequestInit) => {
        signal = init.signal ?? undefined;
        return new Promise<Response>((_resolve, reject) => {
          init.signal?.addEventListener("abort", () => reject(new Error("aborted")), {
            once: true,
          });
        });
      });
      const { result } = renderHook(() => useClarificationGroup([clarMessage()]));

      let submit!: Promise<void>;
      await act(async () => {
        submit = result.current.submitCollected({
          q1: { question_id: "q1", selected_options: ["o1"] },
        });
      });
      expect(result.current.submitState).toBe("submitting");

      const settled = Promise.race([
        submit.then(() => true),
        new Promise<boolean>((resolve) => {
          setTimeout(() => resolve(false), 40_001);
        }),
      ]);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(40_001);
      });
      expect(await settled).toBe(true);

      expect(signal?.aborted).toBe(true);
      expect(result.current.submitState).toBe("error");
      expect(result.current.answers.q1).toEqual({
        question_id: "q1",
        selected_options: ["o1"],
      });
      expect(vi.getTimerCount()).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it("treats the backend 503 envelope as retryable while retaining the answer", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: "clarification response is temporarily unavailable",
          code: "temporarily_unavailable",
        }),
        { status: 503 },
      ),
    );
    const { result } = renderHook(() => useClarificationGroup([clarMessage()]));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });

    expect(result.current.submitState).toBe("error");
    expect(result.current.answers.q1).toEqual({
      question_id: "q1",
      selected_options: ["o1"],
    });
  });

  it("treats a malformed 200 body as retryable without applying optimistic success", async () => {
    fetchMock.mockResolvedValueOnce(new Response("not json", { status: 200 }));
    const { result } = renderHook(() => useClarificationGroup([clarMessage()]));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });

    expect(result.current.submitState).toBe("error");
    expect(mockUpdateMessage).not.toHaveBeenCalled();
    expect(result.current.answers.q1).toEqual({
      question_id: "q1",
      selected_options: ["o1"],
    });
  });
});
