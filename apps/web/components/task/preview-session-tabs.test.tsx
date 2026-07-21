import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  getWebSocketClient: vi.fn(),
  onSend: null as null | ((content: string) => Promise<void>),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));
vi.mock("./task-chat-panel", () => ({
  TaskChatPanel: ({ onSend }: { onSend: (content: string) => Promise<void> }) => {
    mocks.onSend = onSend;
    return <div data-testid="preview-chat" />;
  },
}));

import { PreviewSessionBody } from "./preview-session-tabs";

const session: TaskSession = {
  id: toSessionId("session-1"),
  task_id: toTaskId("task-1"),
  state: "COMPLETED",
  started_at: "2026-07-21T00:00:00Z",
  updated_at: "2026-07-21T00:00:00Z",
};

afterEach(() => {
  cleanup();
  mocks.getWebSocketClient.mockReset();
  mocks.onSend = null;
});

describe("PreviewSessionBody send failures", () => {
  it("rejects when the WebSocket client is unavailable", async () => {
    mocks.getWebSocketClient.mockReturnValue(null);
    render(<PreviewSessionBody session={session} taskId="task-1" />);

    await expect(mocks.onSend?.("hello")).rejects.toThrow("WebSocket client unavailable");
  });

  it("rethrows message.add failures to the chat input", async () => {
    const error = new Error("message.add failed");
    mocks.getWebSocketClient.mockReturnValue({ request: vi.fn().mockRejectedValue(error) });
    render(<PreviewSessionBody session={session} taskId="task-1" />);

    await expect(mocks.onSend?.("hello")).rejects.toBe(error);
  });
});
