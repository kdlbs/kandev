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
});

describe("useClarificationGroup — submit failure and conflict handling", () => {
  beforeEach(setupFetchMock);

  // A 409 from /respond is the backend's "no longer active" outcome
  // (errClarificationNotActive / IsNotActiveError): the bundle expired
  // (session went terminal, task archived, or a later turn superseded it).
  // A genuine duplicate submit never reaches this status — it resolves
  // through the 200 win/loss envelope (claimed: true/false) instead. So a
  // 409 must NOT be reported to the user as a successful submit.
  it("submitState transitions to 'expired' on 409 with the not_active code", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: "clarification request is no longer active", code: "not_active" }),
        {
          status: 409,
        },
      ),
    );
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });
    expect(result.current.submitState).toBe("expired");
  });

  it("submitState transitions to 'expired' on a bodyless 409 (legacy backend)", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 409 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });
    expect(result.current.submitState).toBe("expired");
  });

  it("submitState transitions to 'error' on an unrecognized 409 code (fail closed, never silently 'ok')", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "unexpected", code: "some_future_code" }), {
        status: 409,
      }),
    );
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });
    expect(result.current.submitState).toBe("error");
  });

  it("skipAll transitions to 'expired' on 409 too", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 409 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.skipAll();
    });
    expect(result.current.submitState).toBe("expired");
  });

  it("submitCollected preserves recorded answers when the submit fails", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      result.current.recordAnswer("q1", { question_id: "q1", selected_options: ["o1"] });
    });
    await act(async () => {
      await result.current.submitCollected();
    });

    expect(result.current.submitState).toBe("error");
    expect(result.current.answers["q1"]).toEqual({ question_id: "q1", selected_options: ["o1"] });
  });
});

describe("useClarificationGroup — retry", () => {
  beforeEach(setupFetchMock);

  it("retry() re-POSTs the same batch after a failed submit", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 200 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });
    expect(result.current.submitState).toBe("error");

    await act(async () => {
      await result.current.retry();
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [, secondInit] = fetchMock.mock.calls[1];
    expect(JSON.parse(String(secondInit.body))).toEqual({
      answers: [{ question_id: "q1", selected_options: ["o1"] }],
      rejected: false,
    });
    expect(result.current.submitState).toBe("ok");
  });

  it("retry() re-POSTs the original skip reason after a failed skip", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 200 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.skipAll("Too vague");
    });
    expect(result.current.submitState).toBe("error");

    await act(async () => {
      await result.current.retry();
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [, secondInit] = fetchMock.mock.calls[1];
    expect(JSON.parse(String(secondInit.body))).toEqual({
      rejected: true,
      reject_reason: "Too vague",
    });
    expect(result.current.submitState).toBe("ok");
  });

  it("retry() is a no-op before any submit/skip has been attempted", async () => {
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.retry();
    });

    expect(fetchMock).not.toHaveBeenCalled();
  });

  // Regression: a new bundle (different pendingId) replacing a still-failed one
  // must not inherit the old bundle's error state, recorded answers, or
  // replayable retry action -- otherwise the live question B renders bundle A's
  // stale banner, and clicking Retry POSTs bundle A's answers to bundle B's
  // pendingId.
  it("resets submitState, answers, and the retry action when pendingId changes", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    const bundleA = [
      clarMessage({ id: "m-a", pendingId: "pA", questionId: "qA", index: 0, total: 1 }),
    ];
    const bundleB = [
      // Reusing a question ID is valid and must not make the old retry action
      // look compatible with the replacement bundle.
      clarMessage({ id: "m-b", pendingId: "pB", questionId: "qA", index: 0, total: 1 }),
    ];

    const { result, rerender } = renderHook(({ msgs }) => useClarificationGroup(msgs), {
      initialProps: { msgs: bundleA },
    });

    await act(async () => {
      result.current.recordAnswer("qA", { question_id: "qA", selected_options: ["o1"] });
    });
    await act(async () => {
      await result.current.submitCollected();
    });
    expect(result.current.submitState).toBe("error");
    expect(result.current.answers["qA"]).toBeDefined();

    rerender({ msgs: bundleB });

    expect(result.current.pendingId).toBe("pB");
    expect(result.current.submitState).toBe("idle");
    expect(result.current.answers).toEqual({});

    // retry() must be a no-op -- it must NOT replay bundle A's answers
    // against bundle B's pendingId.
    fetchMock.mockClear();
    await act(async () => {
      await result.current.retry();
    });
    expect(fetchMock).not.toHaveBeenCalled();
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

// W1: both POST call sites must send the session cookie, or an auth-enabled
// backend in split-origin dev mode rejects the request before it reaches the
// resolver.
describe("useClarificationGroup — credentials", () => {
  beforeEach(setupFetchMock);

  it("submitCollected sends credentials: include", async () => {
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["o1"] },
      });
    });

    const [, init] = fetchMock.mock.calls[0];
    expect(init.credentials).toBe("include");
  });

  it("skipAll sends credentials: include", async () => {
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.skipAll("Too vague");
    });

    const [, init] = fetchMock.mock.calls[0];
    expect(init.credentials).toBe("include");
  });
});

// Queues a claimed:false R10 envelope as the next fetch response, with the
// winner's status/answers the assertion cares about.
function mockLostRaceResponse(opts: {
  status: "answered" | "rejected";
  answers?: Array<{ question_id: string; selected_options?: string[]; custom_text?: string }>;
}) {
  fetchMock.mockResolvedValueOnce(
    new Response(
      JSON.stringify({
        success: true,
        claimed: false,
        status: opts.status,
        response: opts.answers ? { pending_id: "p1", answers: opts.answers } : null,
      }),
      { status: 200 },
    ),
  );
}

// W2/W3: a `claimed: false` response is a successful submit (the overlay
// closes exactly as it does on 409), but the optimistic update must reflect
// the winner's own status and answers — never this client's losing ones,
// since R2 guarantees no later WS broadcast will ever correct that write.
describe("useClarificationGroup — losing a race (claimed: false)", () => {
  beforeEach(setupFetchMock);

  it("submitCollected treats claimed:false as a successful submit", async () => {
    mockLostRaceResponse({
      status: "answered",
      answers: [{ question_id: "q1", selected_options: ["o2"] }],
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

  it("submitCollected applies the winner's answers, not this client's own submitted answers", async () => {
    mockLostRaceResponse({
      status: "answered",
      answers: [{ question_id: "q1", selected_options: ["winner-option"] }],
    });
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["my-losing-option"] },
      });
    });

    expect(mockUpdateMessage).toHaveBeenCalledTimes(1);
    const call = mockUpdateMessage.mock.calls[0][0];
    expect(call.metadata.status).toBe("answered");
    expect(call.metadata.response).toEqual({
      question_id: "q1",
      selected_options: ["winner-option"],
    });
  });
});

describe("useClarificationGroup — losing a race, other call sites & fallback", () => {
  beforeEach(setupFetchMock);

  it("skipAll applies the winner's answers on claimed:false instead of an empty response", async () => {
    mockLostRaceResponse({
      status: "answered",
      answers: [{ question_id: "q1", custom_text: "winner text" }],
    });
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      await result.current.skipAll("Too vague");
    });

    expect(mockUpdateMessage).toHaveBeenCalledTimes(1);
    const call = mockUpdateMessage.mock.calls[0][0];
    expect(call.metadata.status).toBe("answered");
    expect(call.metadata.response).toEqual({ question_id: "q1", custom_text: "winner text" });
  });

  it("submitCollected keeps applying its own answers when claimed is absent (older backend)", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 200 }));
    const msgs = [clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 1 })];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    const ownAnswer = { question_id: "q1", selected_options: ["my-own-option"] };
    await act(async () => {
      await result.current.submitCollected({ q1: ownAnswer });
    });

    expect(mockUpdateMessage).toHaveBeenCalledTimes(1);
    const call = mockUpdateMessage.mock.calls[0][0];
    expect(call.metadata.status).toBe("answered");
    expect(call.metadata.response).toEqual(ownAnswer);
  });
});

// Regression: bundle A's request is still in flight (not yet settled) when
// the swap to bundle B happens -- the reachable window in production, since
// these POSTs run 60-75s and the panel can swap at any point during that.
describe("useClarificationGroup — bundle swap while a request is in flight", () => {
  beforeEach(setupFetchMock);

  it("does not paint bundle A's late failure onto bundle B, and does not block B's own submit", async () => {
    let resolveA: ((res: Response) => void) | null = null;
    fetchMock.mockImplementationOnce(() => new Promise<Response>((r) => (resolveA = r)));
    const msg = (id: string, p: string, q: string) => [
      clarMessage({ id, pendingId: p, questionId: q, index: 0, total: 1 }),
    ];
    const bundleA = msg("m-a", "pA", "qA");
    const bundleB = msg("m-b", "pB", "qB");
    const { result, rerender } = renderHook(({ msgs }) => useClarificationGroup(msgs), {
      initialProps: { msgs: bundleA },
    });

    let pendingASubmit!: Promise<void>; // A's submit never settles yet.
    await act(async () => {
      pendingASubmit = result.current.submitCollected({
        qA: { question_id: "qA", selected_options: ["o1"] },
      });
    });
    expect(result.current.submitState).toBe("submitting");

    rerender({ msgs: bundleB }); // the next clarification streams in mid-flight
    expect(result.current.submitState).toBe("idle");

    // B must be submittable right away, not blocked by A's stale request.
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 200 }));
    await act(async () => {
      await result.current.submitCollected({ qB: { question_id: "qB", selected_options: ["o2"] } });
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.submitState).toBe("ok");

    // A's stale request finally fails -- must not clobber B's "ok" state.
    await act(async () => {
      resolveA?.(new Response("nope", { status: 500 }));
      await pendingASubmit;
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
