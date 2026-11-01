import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

const mocks = vi.hoisted(() => ({
  getAgentQuestion: vi.fn(),
  refreshAgentQuestion: vi.fn(),
  handler: null as ((message: { payload: { question_id: string } }) => void) | null,
  unsubscribe: vi.fn(),
}));
const questionID = "question-1";

vi.mock("@/lib/api/domains/agent-questions-api", () => ({
  getAgentQuestion: mocks.getAgentQuestion,
  refreshAgentQuestion: mocks.refreshAgentQuestion,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    on: (_event: string, handler: typeof mocks.handler) => {
      mocks.handler = handler;
      return mocks.unsubscribe;
    },
  }),
}));

import { useAgentQuestionDetail } from "./use-agent-question-detail";

function question(
  status: ClarificationInboxAsyncQuestion["status"],
): ClarificationInboxAsyncQuestion {
  return {
    question_id: questionID,
    task_id: "task-1",
    session_id: "session-1",
    session_state: "RUNNING",
    task_title: "Task",
    forwarded: false,
    waiting: false,
    status,
    context: "",
    created_at: "2026-10-30T10:00:00Z",
    questions: [],
  };
}

beforeEach(() => {
  mocks.getAgentQuestion.mockReset();
  mocks.refreshAgentQuestion.mockReset();
  mocks.handler = null;
  mocks.unsubscribe.mockReset();
});

afterEach(cleanup);

describe("useAgentQuestionDetail", () => {
  it("loads the request and refreshes it when its change event arrives", async () => {
    const open = question("pending");
    const answered = question("answered");
    mocks.getAgentQuestion.mockResolvedValue(open);
    mocks.refreshAgentQuestion.mockResolvedValue(answered);

    const { result, unmount } = renderHook(() => useAgentQuestionDetail(questionID));
    await waitFor(() => expect(result.current.question).toBe(open));
    expect(mocks.getAgentQuestion).toHaveBeenCalledWith(questionID);

    act(() => mocks.handler?.({ payload: { question_id: "other-question" } }));
    expect(mocks.refreshAgentQuestion).not.toHaveBeenCalled();

    act(() => mocks.handler?.({ payload: { question_id: questionID } }));
    await waitFor(() => expect(result.current.question).toBe(answered));
    expect(mocks.refreshAgentQuestion).toHaveBeenCalledWith(questionID);

    unmount();
    expect(mocks.unsubscribe).toHaveBeenCalledTimes(1);
  });
});
