import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({ updateMessage: vi.fn() }),
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
  fetchMock.mockResolvedValue(new Response(null, { status: 200 }));
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
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 200 }));
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
      resolveSecond?.(new Response(null, { status: 200 }));
      await secondSubmit;
    });
    expect(result.current.submitState).toBe("ok");
  });
});
