import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listAgentQuestions, type AgentQuestionPage } from "@/lib/api/domains/agent-questions-api";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";
import { StateProvider } from "@/components/state-provider";
import { useTaskAsyncQuestions } from "./use-task-async-questions";

vi.mock("@/lib/api/domains/agent-questions-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/agent-questions-api")>()),
  listAgentQuestions: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function question(id: string): ClarificationInboxAsyncQuestion {
  return {
    question_id: id,
    task_id: "task-a",
    session_id: "session-a",
    session_state: "RUNNING",
    task_title: "Task A",
    forwarded: false,
    waiting: false,
    status: "pending",
    context: "",
    created_at: "2026-10-30T10:00:00Z",
    questions: [],
  };
}

function page(...questions: ClarificationInboxAsyncQuestion[]): AgentQuestionPage {
  return { questions, count: questions.length, has_more: false };
}

function wrapper({ children }: { children: React.ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

beforeEach(() => {
  vi.mocked(listAgentQuestions).mockReset();
});

afterEach(cleanup);

describe("useTaskAsyncQuestions", () => {
  it("loads one task scope and ignores an older response", async () => {
    const first = deferred<AgentQuestionPage>();
    const second = deferred<AgentQuestionPage>();
    vi.mocked(listAgentQuestions)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { result } = renderHook(() => useTaskAsyncQuestions("workspace-a", "task-a"), {
      wrapper,
    });
    await waitFor(() => expect(listAgentQuestions).toHaveBeenCalledTimes(1));
    expect(listAgentQuestions).toHaveBeenLastCalledWith("workspace-a", {
      taskId: "task-a",
      status: "open",
      limit: 200,
    });

    let refresh!: Promise<void>;
    act(() => {
      refresh = result.current.refresh();
    });
    await waitFor(() => expect(listAgentQuestions).toHaveBeenCalledTimes(2));
    await act(async () => {
      second.resolve(page(question("newer")));
      await refresh;
    });
    await act(async () => {
      first.resolve(page(question("older")));
      await Promise.resolve();
    });

    expect(result.current.questions.map((item) => item.question_id)).toEqual(["newer"]);
    expect(result.current.status).toBe("ready");
  });

  it("scopes the next read independently after changing workspace or task", async () => {
    vi.mocked(listAgentQuestions).mockResolvedValue(page(question("scoped")));
    const { rerender } = renderHook(
      ({ workspaceId, taskId }) => useTaskAsyncQuestions(workspaceId, taskId),
      { initialProps: { workspaceId: "workspace-a", taskId: "task-a" }, wrapper },
    );
    await waitFor(() => expect(listAgentQuestions).toHaveBeenCalledTimes(1));
    rerender({ workspaceId: "workspace-b", taskId: "task-b" });
    await waitFor(() => expect(listAgentQuestions).toHaveBeenCalledTimes(2));
    expect(listAgentQuestions).toHaveBeenLastCalledWith("workspace-b", {
      taskId: "task-b",
      status: "open",
      limit: 200,
    });
  });
});
