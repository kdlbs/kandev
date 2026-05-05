import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { Message } from "@/lib/types/http";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));

import { useClarificationGroup } from "./use-clarification-group";

function clarMessage(opts: {
  id: string;
  pendingId: string;
  questionId: string;
  index: number;
  total: number;
}): Message {
  return {
    id: opts.id,
    session_id: "s1",
    task_id: "t1",
    author_type: "agent",
    content: "Q",
    type: "clarification_request",
    created_at: "2026-05-04T00:00:00Z",
    metadata: {
      pending_id: opts.pendingId,
      question_id: opts.questionId,
      question_index: opts.index,
      question_total: opts.total,
      status: "pending",
      question: { id: opts.questionId, prompt: "Q?" },
    },
  };
}

const fetchMock = vi.fn();

function setupFetchMock() {
  fetchMock.mockReset();
  fetchMock.mockResolvedValue(new Response(null, { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
}

describe("useClarificationGroup — derived state", () => {
  beforeEach(setupFetchMock);

  it("derives total + pendingId from the message bundle", () => {
    const msgs = [
      clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m2", pendingId: "p1", questionId: "q2", index: 1, total: 2 }),
    ];
    const { result } = renderHook(() => useClarificationGroup(msgs));
    expect(result.current.pendingId).toBe("p1");
    expect(result.current.total).toBe(2);
    expect(result.current.answeredCount).toBe(0);
  });

  it("returns null pendingId when there are no messages", () => {
    const { result } = renderHook(() => useClarificationGroup([]));
    expect(result.current.pendingId).toBeNull();
    expect(result.current.total).toBe(0);
  });

  it("recordAnswer updates local state without posting", async () => {
    const msgs = [
      clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m2", pendingId: "p1", questionId: "q2", index: 1, total: 2 }),
    ];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      result.current.recordAnswer("q1", { question_id: "q1", selected_options: ["o1"] });
    });

    expect(result.current.answers["q1"]?.selected_options).toEqual(["o1"]);
    expect(result.current.answeredCount).toBe(1);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("useClarificationGroup — submit + skip", () => {
  beforeEach(setupFetchMock);

  it("submitCollected POSTs the batch only when every question has an answer", async () => {
    const msgs = [
      clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m2", pendingId: "p1", questionId: "q2", index: 1, total: 2 }),
    ];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      result.current.recordAnswer("q1", { question_id: "q1", selected_options: ["o1"] });
    });
    await act(async () => {
      await result.current.submitCollected();
    });
    expect(fetchMock).not.toHaveBeenCalled();

    await act(async () => {
      result.current.recordAnswer("q2", { question_id: "q2", custom_text: "free" });
    });
    await act(async () => {
      await result.current.submitCollected();
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toBe("https://api.test/api/v1/clarification/p1/respond");
    expect(JSON.parse(String(init.body))).toEqual({
      answers: [
        { question_id: "q1", selected_options: ["o1"] },
        { question_id: "q2", custom_text: "free" },
      ],
      rejected: false,
    });
    expect(result.current.submitState).toBe("ok");
  });

  it("submitCollected with override merges the freshly recorded answer", async () => {
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(result.current.submitState).toBe("ok");
  });

  it("skipAll POSTs rejected=true with the supplied reason", async () => {
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.skipAll("Too vague");
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(String(init.body))).toEqual({
      rejected: true,
      reject_reason: "Too vague",
    });
    expect(result.current.submitState).toBe("ok");
  });

  it("submitState transitions to 'error' when fetch returns non-OK", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 400 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });
    expect(result.current.submitState).toBe("error");
  });

  // 409 Conflict means a duplicate submit landed after the first one already
  // resolved the bundle on the backend. Treat it as success so the overlay
  // closes instead of getting stuck in an unrecoverable error state.
  it("submitState transitions to 'ok' on 409 (duplicate submit)", async () => {
    fetchMock.mockResolvedValueOnce(new Response("conflict", { status: 409 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });
    expect(result.current.submitState).toBe("ok");
  });

  it("skipAll transitions to 'ok' on 409 too", async () => {
    fetchMock.mockResolvedValueOnce(new Response("conflict", { status: 409 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.skipAll();
    });
    expect(result.current.submitState).toBe("ok");
  });
});
