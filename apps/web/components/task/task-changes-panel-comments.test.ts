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

it("sends and clears only the active session's review feedback", async () => {
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
  await act(() => result.current());
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

it("retries the same file-bearing Changes request after a lost acknowledgement", async () => {
  const comment: ReviewFileComment = {
    source: "review-file",
    id: "file-feedback",
    sessionId: "session-b",
    repositoryName: "",
    filePath: "src/main.go",
    text: "See evidence",
    status: "pending",
    createdAt: "now",
    attachments: [
      {
        attachment_id: "uploaded-file",
        type: "resource",
        mime_type: "text/plain",
        delivery_mode: "path",
      },
    ],
  };
  useCommentsStore.getState().addComment(comment);
  request.mockRejectedValueOnce(new Error("Lost response"));
  const { result } = renderHook(() => useFixCommentsRequest("session-b", false));
  await act(() => result.current());
  expect(useCommentsStore.getState().byId[comment.id]).toBe(comment);
  await act(() => result.current());
  expect(request.mock.calls[1]).toEqual(request.mock.calls[0]);
  expect(useCommentsStore.getState().byId[comment.id]).toBeUndefined();
});
