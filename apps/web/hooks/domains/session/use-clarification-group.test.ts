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

function clarMessage(opts: {
  id: string;
  pendingId: string;
  questionId: string;
  index: number;
  total: number;
}): Message {
  return {
    id: opts.id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
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
  mockUpdateMessage.mockReset();
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

  it("clearAnswer removes the entry and decrements answeredCount", async () => {
    const msgs = [
      clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m2", pendingId: "p1", questionId: "q2", index: 1, total: 2 }),
    ];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      result.current.recordAnswer("q1", { question_id: "q1", custom_text: "draft" });
    });
    expect(result.current.answeredCount).toBe(1);

    await act(async () => {
      result.current.clearAnswer("q1");
    });
    expect(result.current.answers["q1"]).toBeUndefined();
    expect(result.current.answeredCount).toBe(0);

    // Clearing a question that was never recorded is a no-op.
    await act(async () => {
      result.current.clearAnswer("q-missing");
    });
    expect(result.current.answeredCount).toBe(0);
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

// Silent WS dead-socket scenario: when the question has been hanging long enough
// that the underlying WebSocket has gone half-dead (NAT timeout, browser throttle),
// the answer submit still completes via HTTP but the backend's session.message.updated
// broadcast never reaches the dead socket — so the overlay would stay stuck on the
// pending bundle until the user refreshes. To stay robust against that we mark
// each bundle message as answered/rejected in the store the moment the HTTP POST
// resolves, mirroring the backend update the WS event would have delivered.
describe("useClarificationGroup — optimistic store update on resolve", () => {
  beforeEach(setupFetchMock);

  it("submitCollected marks every bundle message as answered in the store", async () => {
    const msgs = [
      clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m2", pendingId: "p1", questionId: "q2", index: 1, total: 2 }),
    ];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    const answer1 = { question_id: "q1", selected_options: ["o1"] };
    const answer2 = { question_id: "q2", custom_text: "free" };
    await act(async () => {
      result.current.recordAnswer("q1", answer1);
      result.current.recordAnswer("q2", answer2);
    });
    await act(async () => {
      await result.current.submitCollected();
    });

    expect(result.current.submitState).toBe("ok");
    expect(mockUpdateMessage).toHaveBeenCalledTimes(2);
    const firstCall = mockUpdateMessage.mock.calls[0][0];
    const secondCall = mockUpdateMessage.mock.calls[1][0];
    expect(firstCall.id).toBe("m1");
    expect(firstCall.metadata.status).toBe("answered");
    expect(firstCall.metadata.response).toEqual(answer1);
    expect(firstCall.metadata.pending_id).toBe("p1");
    expect(secondCall.id).toBe("m2");
    expect(secondCall.metadata.status).toBe("answered");
    expect(secondCall.metadata.response).toEqual(answer2);
  });

  it("skipAll marks every bundle message as rejected in the store", async () => {
    const msgs = [
      clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m2", pendingId: "p1", questionId: "q2", index: 1, total: 2 }),
    ];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.skipAll("Too vague");
    });

    expect(result.current.submitState).toBe("ok");
    expect(mockUpdateMessage).toHaveBeenCalledTimes(2);
    const calls = mockUpdateMessage.mock.calls.map((c) => c[0]);
    expect(calls.map((m) => m.id).sort()).toEqual(["m1", "m2"]);
    for (const m of calls) {
      expect(m.metadata.status).toBe("rejected");
      expect(m.metadata.pending_id).toBe("p1");
    }
  });

  it("submitCollected does NOT touch the store when the HTTP request fails", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 400 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });

    expect(result.current.submitState).toBe("error");
    expect(mockUpdateMessage).not.toHaveBeenCalled();
  });
});

describe("useClarificationGroup — optimistic store update edge cases", () => {
  beforeEach(setupFetchMock);

  // Race guard (cubic P1): if the parent re-renders the hook with a different
  // bundle after the POST is in flight (e.g. the next clarification has
  // already streamed in), the optimistic update must still target the bundle
  // that was *submitted* — not whatever the live messages prop points at when
  // the await resolves. We capture a snapshot at submit time.
  it("submitCollected applies the optimistic update to the submit-time bundle, not the latest one", async () => {
    let resolveFetch: ((res: Response) => void) | null = null;
    fetchMock.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const initial = [
      clarMessage({ id: "m-old", pendingId: "p-old", questionId: "q-old", index: 0, total: 1 }),
    ];
    const next = [
      clarMessage({ id: "m-new", pendingId: "p-new", questionId: "q-new", index: 0, total: 1 }),
    ];

    const { result, rerender } = renderHook(({ msgs }) => useClarificationGroup(msgs), {
      initialProps: { msgs: initial },
    });

    await act(async () => {
      // Submit kicks off the POST; rerender swaps the bundle while in flight;
      // resolveFetch unblocks the POST and the optimistic update runs.
      const pending = result.current.submitCollected({
        "q-old": { question_id: "q-old", selected_options: ["o1"] },
      });
      rerender({ msgs: next });
      resolveFetch?.(new Response(null, { status: 200 }));
      await pending;
    });

    expect(mockUpdateMessage).toHaveBeenCalledTimes(1);
    expect(mockUpdateMessage.mock.calls[0][0].id).toBe("m-old");
    expect(mockUpdateMessage.mock.calls[0][0].metadata.pending_id).toBe("p-old");
  });

  // Failure-isolation guard (greptile P1): the optimistic store update is a
  // best-effort UI nicety — if it blows up (e.g. the store action is missing,
  // immer throws on a frozen object), the HTTP submit still succeeded and the
  // user must see submitState === "ok", not "error".
  it("submitCollected stays 'ok' when the optimistic store update throws", async () => {
    mockUpdateMessage.mockImplementationOnce(() => {
      throw new Error("store update boom");
    });
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });

    expect(result.current.submitState).toBe("ok");
  });

  it("skipAll stays 'ok' when the optimistic store update throws", async () => {
    mockUpdateMessage.mockImplementationOnce(() => {
      throw new Error("store update boom");
    });
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.skipAll();
    });

    expect(result.current.submitState).toBe("ok");
  });
});

describe("useClarificationGroup — inflight guard", () => {
  beforeEach(setupFetchMock);

  // Cmd+Enter inside the custom-text input historically reached both onSubmit
  // and onRequestFinalSubmit, which could fire submitCollected twice in the
  // same tick. The hook's inflight ref must keep the wire count at 1 even if
  // the UI races; the backend would otherwise see a duplicate POST.
  it("submitCollected guards against concurrent calls", async () => {
    let resolveFetch: ((res: Response) => void) | null = null;
    fetchMock.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        }),
    );
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      result.current.recordAnswer("q1", { question_id: "q1", selected_options: ["o1"] });
    });

    await act(async () => {
      const first = result.current.submitCollected();
      const second = result.current.submitCollected();
      resolveFetch?.(new Response(null, { status: 200 }));
      await Promise.all([first, second]);
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(result.current.submitState).toBe("ok");
  });
});
