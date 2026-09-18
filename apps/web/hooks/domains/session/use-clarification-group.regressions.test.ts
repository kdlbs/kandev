import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));

const mockUpdateMessage = vi.fn();
let mockMessagesBySession: Record<string, Message[]> = {};
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      updateMessage: mockUpdateMessage,
      messages: { bySession: mockMessagesBySession },
    }),
  }),
}));

import { useClarificationGroup } from "./use-clarification-group";

function clarMessage(opts: {
  id: string;
  pendingId: string;
  questionId: string;
  index: number;
  total: number;
  updatedAt?: string;
}): Message {
  return {
    id: opts.id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "Q",
    type: "clarification_request",
    created_at: "2026-05-04T00:00:00Z",
    ...(opts.updatedAt ? { updated_at: opts.updatedAt } : {}),
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
  mockMessagesBySession = {};
  fetchMock.mockResolvedValue(new Response(JSON.stringify({ success: true }), { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
}

describe("useClarificationGroup — malformed conflict responses", () => {
  beforeEach(setupFetchMock);

  it("sets error for a malformed nonempty 409 body", async () => {
    fetchMock.mockResolvedValueOnce(new Response("upstream proxy failure", { status: 409 }));
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

describe("useClarificationGroup — live retry answers", () => {
  beforeEach(setupFetchMock);

  it("retry uses answers edited after the failed submit", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true }), { status: 200 }),
    );
    const msgs = [
      clarMessage({ id: "m1", pendingId: "p1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m2", pendingId: "p1", questionId: "q2", index: 1, total: 2 }),
    ];
    const { result } = renderHook(() => useClarificationGroup(msgs));

    await act(async () => {
      result.current.recordAnswer("q1", { question_id: "q1", selected_options: ["old"] });
      result.current.recordAnswer("q2", { question_id: "q2", selected_options: ["keep"] });
      await result.current.submitCollected();
    });
    expect(result.current.submitState).toBe("error");

    await act(async () => {
      result.current.recordAnswer("q1", { question_id: "q1", selected_options: ["new"] });
      await result.current.retry();
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [, secondInit] = fetchMock.mock.calls[1];
    expect(JSON.parse(String(secondInit.body))).toEqual({
      answers: [
        { question_id: "q1", selected_options: ["new"] },
        { question_id: "q2", selected_options: ["keep"] },
      ],
      rejected: false,
    });
  });
});

describe("useClarificationGroup — legacy success envelope", () => {
  beforeEach(setupFetchMock);

  it("applies its own answers when a valid success envelope omits claimed", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true }), { status: 200 }),
    );
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

describe("useClarificationGroup — request generation guard", () => {
  beforeEach(setupFetchMock);

  it("ignores a stale request after switching A to B and back to A", async () => {
    let resolveFirst: ((res: Response) => void) | null = null;
    let resolveSecond: ((res: Response) => void) | null = null;
    fetchMock.mockImplementationOnce(
      () => new Promise<Response>((resolve) => (resolveFirst = resolve)),
    );
    fetchMock.mockImplementationOnce(
      () => new Promise<Response>((resolve) => (resolveSecond = resolve)),
    );
    const bundle = (pendingId: string) => [
      clarMessage({ id: `m-${pendingId}`, pendingId, questionId: "q1", index: 0, total: 1 }),
    ];
    const { result, rerender } = renderHook(({ msgs }) => useClarificationGroup(msgs), {
      initialProps: { msgs: bundle("pA") },
    });

    let firstSubmit!: Promise<void>;
    await act(async () => {
      firstSubmit = result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["first"] },
      });
    });
    rerender({ msgs: bundle("pB") });
    rerender({ msgs: bundle("pA") });

    let secondSubmit!: Promise<void>;
    await act(async () => {
      secondSubmit = result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["second"] },
      });
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      resolveFirst?.(new Response("nope", { status: 500 }));
      await firstSubmit;
    });
    expect(result.current.submitState).toBe("submitting");

    await act(async () => {
      await result.current.submitCollected({
        q1: { question_id: "q1", selected_options: ["third"] },
      });
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      resolveSecond?.(new Response(JSON.stringify({ success: true }), { status: 200 }));
      await secondSubmit;
    });
    expect(result.current.submitState).toBe("ok");
  });
});

// eslint-disable-next-line max-lines-per-function -- inactive response races share one deterministic hook harness.
describe("useClarificationGroup — inactive response reconciliation", () => {
  beforeEach(setupFetchMock);

  it("keeps a newer authoritative pending restoration when the inactive response settles late", async () => {
    let resolveA: ((res: Response) => void) | null = null;
    fetchMock.mockImplementationOnce(
      () => new Promise<Response>((resolve) => (resolveA = resolve)),
    );

    const submitted = clarMessage({
      id: "m-a",
      pendingId: "pA",
      questionId: "qA",
      index: 0,
      total: 1,
      updatedAt: "2026-05-04T00:00:00.000000001Z",
    });
    const restored = {
      ...submitted,
      updated_at: "2026-05-04T00:00:00.000000002Z",
      metadata: { ...submitted.metadata, status: "pending" as const },
    };
    mockMessagesBySession = { [submitted.session_id]: [submitted] };
    const onOutcome = vi.fn();
    const { result, rerender } = renderHook(({ msgs }) => useClarificationGroup(msgs, onOutcome), {
      initialProps: { msgs: [submitted] },
    });

    let pendingRequest!: Promise<void>;
    await act(async () => {
      pendingRequest = result.current.skipAll();
    });

    mockMessagesBySession = { [restored.session_id]: [restored] };
    rerender({ msgs: [restored] });

    await act(async () => {
      resolveA?.(new Response(null, { status: 409 }));
      await pendingRequest;
    });

    expect(result.current.submitState).toBe("idle");
    expect(onOutcome).not.toHaveBeenCalled();
    expect(mockUpdateMessage).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.skipAll();
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.submitState).toBe("ok");
  });

  it("does not normalize an invalid current timestamp as newer authority", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 409 }));
    const submitted = clarMessage({
      id: "m-invalid",
      pendingId: "p-invalid",
      questionId: "q-invalid",
      index: 0,
      total: 1,
      updatedAt: "2026-02-28T00:00:00.000000001Z",
    });
    const invalidCurrent = {
      ...submitted,
      updated_at: "2026-02-30T00:00:00.000000002Z",
      metadata: { ...submitted.metadata, status: "pending" as const },
    };
    mockMessagesBySession = { [submitted.session_id]: [invalidCurrent] };
    const { result, rerender } = renderHook(({ msgs }) => useClarificationGroup(msgs), {
      initialProps: { msgs: [submitted] },
    });
    rerender({ msgs: [invalidCurrent] });

    await act(async () => {
      await result.current.skipAll();
    });

    expect(result.current.submitState).toBe("expired");
    expect(mockUpdateMessage).toHaveBeenCalledWith({
      ...invalidCurrent,
      metadata: { ...invalidCurrent.metadata, status: "expired" },
    });
  });

  it("does not expire bundle A after it leaves and returns to the active generation", async () => {
    let resolveA: ((res: Response) => void) | null = null;
    fetchMock.mockImplementationOnce(
      () => new Promise<Response>((resolve) => (resolveA = resolve)),
    );

    const bundle = (pendingId: string) => [
      clarMessage({ id: "m-shared", pendingId, questionId: "q1", index: 0, total: 1 }),
    ];
    const bundleA = bundle("pA");
    const bundleB = bundle("pB");
    mockMessagesBySession = { [bundleA[0].session_id]: bundleA };
    const { result, rerender } = renderHook(({ msgs }) => useClarificationGroup(msgs), {
      initialProps: { msgs: bundleA },
    });

    let pendingRequest!: Promise<void>;
    await act(async () => {
      pendingRequest = result.current.skipAll();
    });
    rerender({ msgs: bundleB });
    mockMessagesBySession = { [bundleA[0].session_id]: bundleA };
    rerender({ msgs: bundleA });

    await act(async () => {
      resolveA?.(new Response(null, { status: 409 }));
      await pendingRequest;
    });

    expect(result.current.submitState).toBe("idle");
    expect(mockUpdateMessage).not.toHaveBeenCalled();
  });

  it("keeps bundle B active when bundle A settles late with an inactive response", async () => {
    let resolveA: ((res: Response) => void) | null = null;
    fetchMock.mockImplementationOnce(
      () => new Promise<Response>((resolve) => (resolveA = resolve)),
    );
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true }), { status: 200 }),
    );

    const bundleA = [
      clarMessage({ id: "m-a", pendingId: "pA", questionId: "qA", index: 0, total: 1 }),
    ];
    const bundleB = [
      clarMessage({ id: "m-b", pendingId: "pB", questionId: "qB", index: 0, total: 1 }),
    ];
    mockMessagesBySession = { [bundleA[0].session_id]: [...bundleA, ...bundleB] };
    const onOutcome = vi.fn();
    const { result, rerender } = renderHook(({ msgs }) => useClarificationGroup(msgs, onOutcome), {
      initialProps: { msgs: bundleA },
    });

    let firstSkip!: Promise<void>;
    await act(async () => {
      firstSkip = result.current.skipAll();
    });
    rerender({ msgs: bundleB });

    let secondSubmit!: Promise<void>;
    await act(async () => {
      secondSubmit = result.current.submitCollected({
        qB: { question_id: "qB", selected_options: ["second"] },
      });
      await secondSubmit;
    });
    expect(result.current.submitState).toBe("ok");
    expect(onOutcome).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveA?.(new Response(null, { status: 409 }));
      await firstSkip;
    });

    expect(result.current.submitState).toBe("ok");
    expect(onOutcome).toHaveBeenCalledTimes(1);
    expect(mockUpdateMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "m-a",
        metadata: expect.objectContaining({ status: "expired" }),
      }),
    );
  });
});
