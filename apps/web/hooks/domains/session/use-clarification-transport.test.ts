import { createElement, type ReactNode } from "react";
import { ClarificationTransportContext } from "@/components/task/chat/clarification-transport";
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

function successResponse(): Response {
  return new Response(JSON.stringify({ success: true }), { status: 200 });
}

function setupFetchMock() {
  fetchMock.mockReset();
  mockUpdateMessage.mockReset();
  fetchMock.mockResolvedValue(successResponse());
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
}

describe("useClarificationGroup injected native transport", () => {
  beforeEach(setupFetchMock);
  it("routes answers and skips through the scoped resolver without broad native fetches or store writes", async () => {
    const respond = vi.fn().mockResolvedValue({ state: "ok", claimed: true, status: "answered" });
    const updateMessage = vi.fn();
    const transport = { respond, updateMessage };
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(ClarificationTransportContext.Provider, { value: transport }, children);
    const message = clarMessage({
      id: "projected",
      pendingId: "pending",
      questionId: "question",
      index: 0,
      total: 1,
    });
    const { result } = renderHook(() => useClarificationGroup([message]), { wrapper });
    await act(async () => {
      result.current.recordAnswer("question", {
        question_id: "question",
        custom_text: "A sample heading",
      });
    });
    await act(async () => {
      await result.current.submitCollected();
    });
    expect(respond).toHaveBeenCalledWith("pending", {
      answers: [{ question_id: "question", custom_text: "A sample heading" }],
      rejected: false,
    });
    expect(updateMessage).toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(mockUpdateMessage).not.toHaveBeenCalled();
  });
});
