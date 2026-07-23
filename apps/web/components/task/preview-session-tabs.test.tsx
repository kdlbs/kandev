import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import type { ChatSubmitPayload } from "./chat/chat-input-container";
import type { EntityReference } from "@/lib/types/entity-reference";

const mocks = vi.hoisted(() => ({
  getWebSocketClient: vi.fn(),
  onSend: null as null | ((payload: ChatSubmitPayload) => Promise<void>),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));
vi.mock("./task-chat-panel", () => ({
  TaskChatPanel: ({ onSend }: { onSend: (payload: ChatSubmitPayload) => Promise<void> }) => {
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

    await expect(mocks.onSend?.({ message: "hello" })).rejects.toMatchObject({
      name: "MessageSendError",
      code: "connection-unavailable",
      message: "Connection unavailable. Reconnect and try again.",
    });
  });

  it("rethrows message.add failures to the chat input", async () => {
    const error = new Error("message.add failed");
    mocks.getWebSocketClient.mockReturnValue({ request: vi.fn().mockRejectedValue(error) });
    render(<PreviewSessionBody session={session} taskId="task-1" />);

    await expect(mocks.onSend?.({ message: "hello" })).rejects.toBe(error);
  });

  it("forwards attachments and entity references through preview direct send", async () => {
    const request = vi.fn().mockResolvedValue(undefined);
    mocks.getWebSocketClient.mockReturnValue({ request });
    const reference: EntityReference = {
      version: 1,
      ref: "mention:v1:github:issue:acme%2Frepo:42",
      provider: "github",
      kind: "issue",
      id: "42",
      key: "acme/repo#42",
      title: "Fix composer references",
      url: "https://github.com/acme/repo/issues/42",
      scope: "acme/repo",
    };
    render(<PreviewSessionBody session={session} taskId="task-1" />);

    await mocks.onSend?.({
      message: "reference",
      attachments: [{ type: "image", data: "base64", mime_type: "image/png" }],
      entityReferences: [reference],
    });

    expect(request).toHaveBeenCalledWith(
      "message.add",
      {
        task_id: "task-1",
        session_id: "session-1",
        content: "reference",
        attachments: [{ type: "image", data: "base64", mime_type: "image/png" }],
        entity_references: [reference],
      },
      30000,
    );
  });
});
