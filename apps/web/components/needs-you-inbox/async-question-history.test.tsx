import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ state: null as unknown }));

vi.mock("@/hooks/domains/needs-you-inbox/use-async-question-history", () => ({
  useAsyncQuestionHistory: () => ({
    state: mocks.state,
    retry: vi.fn(),
    loadMore: vi.fn(),
  }),
}));

vi.mock("./async-question-row", () => ({ AsyncQuestionRow: () => null }));

import { AsyncQuestionHistory } from "./async-question-history";

afterEach(() => {
  cleanup();
});

describe("AsyncQuestionHistory", () => {
  it("uses a loading label while loading another history page", () => {
    mocks.state = {
      status: "ready",
      questions: [
        {
          question_id: "q1",
          task_id: "task1",
          session_id: "session1",
          session_state: "COMPLETED",
          task_title: "Completed task",
          forwarded: false,
          waiting: false,
          status: "answered",
          context: "",
          created_at: "2026-10-30T10:00:00Z",
          questions: [],
        },
      ],
      hasMore: true,
      loadingMore: true,
    };

    render(<AsyncQuestionHistory workspaceId="workspace1" refreshTick={0} />);

    expect(screen.getByRole("button", { name: "Loading…" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Submitting…" })).toBeNull();
  });
});
