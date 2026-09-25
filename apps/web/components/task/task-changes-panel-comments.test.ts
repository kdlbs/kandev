import { beforeEach, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useCommentsStore, type ReviewFileComment } from "@/lib/state/slices/comments";
import { useFixCommentsRequest } from "./task-changes-panel";

const { request } = vi.hoisted(() => ({ request: vi.fn().mockResolvedValue({}) }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({ tasks: { activeTaskId: "task-b" } }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request }) }));

beforeEach(() => {
  request.mockClear();
  useCommentsStore.setState({ byId: {}, bySession: {}, pendingForChat: [] });
});

it("sends and clears only the active session's review feedback", () => {
  for (const sessionId of ["session-a", "session-b"]) {
    const comment: ReviewFileComment = {
      source: "review-file",
      id: sessionId,
      sessionId,
      repositoryName: "",
      filePath: "README.md",
      text: `Feedback for ${sessionId}`,
      status: "pending",
      createdAt: "now",
    };
    useCommentsStore.getState().addComment(comment);
  }
  const { result } = renderHook(() => useFixCommentsRequest("session-b", false));
  act(() => result.current());
  expect(request).toHaveBeenCalledWith(
    "message.add",
    expect.objectContaining({
      session_id: "session-b",
      content: expect.stringContaining("Feedback for session-b"),
    }),
  );
  expect(request.mock.calls[0][1].content).not.toContain("Feedback for session-a");
  expect(
    useCommentsStore
      .getState()
      .getPendingComments()
      .map((c) => c.id),
  ).toEqual(["session-a"]);
});
