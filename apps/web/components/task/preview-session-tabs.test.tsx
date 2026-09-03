import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import type { ChatSubmitPayload } from "./chat/chat-input-container";
import type { EntityReference } from "@/lib/types/entity-reference";

type SessionsDropdownStubProps = {
  taskId: string | null;
  activeSessionId?: string | null;
  onSelectSession?: (sessionId: string) => void;
};

const mocks = vi.hoisted(() => ({
  getWebSocketClient: vi.fn(),
  onSend: null as null | ((payload: ChatSubmitPayload) => Promise<void>),
  dropdownProps: [] as SessionsDropdownStubProps[],
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
vi.mock("./sessions-dropdown", () => ({
  SessionsDropdown: (props: SessionsDropdownStubProps) => {
    mocks.dropdownProps.push(props);
    return (
      <button
        type="button"
        data-testid="sessions-dropdown-trigger"
        onClick={() => props.onSelectSession?.("session-b")}
      >
        Agents
      </button>
    );
  },
}));

import { PreviewSessionBody, PreviewSessionTabs } from "./preview-session-tabs";

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
  mocks.dropdownProps = [];
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
        client_message_id: expect.any(String),
        content: "reference",
        attachments: [{ type: "image", data: "base64", mime_type: "image/png" }],
        entity_references: [reference],
      },
      30000,
    );
  });
});

describe("PreviewSessionTabs agents dropdown", () => {
  const TASK_ID = "task-2";

  function fixtureSession(id: string, startedAt: string): TaskSession {
    return {
      id: toSessionId(id),
      task_id: toTaskId(TASK_ID),
      state: "COMPLETED",
      started_at: startedAt,
      updated_at: startedAt,
    };
  }

  function twoSessions(): TaskSession[] {
    return [
      fixtureSession("session-a", "2026-07-21T00:00:00Z"),
      fixtureSession("session-b", "2026-07-21T00:01:00Z"),
    ];
  }

  function renderPreview(
    sessions: TaskSession[],
    sessionId: string | null,
    onSessionChange = vi.fn(),
  ) {
    render(
      <StateProvider
        initialState={{
          taskSessionsByTask: {
            itemsByTaskId: { [TASK_ID]: sessions },
            loadedByTaskId: { [TASK_ID]: true },
            loadingByTaskId: {},
          },
        }}
      >
        <PreviewSessionTabs
          taskId={TASK_ID}
          sessionId={sessionId}
          onSessionChange={onSessionChange}
        />
      </StateProvider>,
    );
    return { onSessionChange };
  }

  it("renders the Agents dropdown in the tab row, wired to the active session", () => {
    renderPreview(twoSessions(), "session-a");

    const tabRow = within(screen.getByTestId("preview-session-tabs"));
    expect(tabRow.getByTestId("sessions-dropdown-trigger")).toBeTruthy();
    expect(mocks.dropdownProps.at(-1)).toMatchObject({
      taskId: TASK_ID,
      activeSessionId: "session-a",
    });
  });

  it("renders the Agents dropdown in the empty state so a new agent can be created", () => {
    renderPreview([], null);

    const emptyState = screen.getByTestId("preview-empty-state").parentElement;
    expect(emptyState).not.toBeNull();
    expect(within(emptyState as HTMLElement).getByTestId("sessions-dropdown-trigger")).toBeTruthy();
    expect(mocks.dropdownProps.at(-1)).toMatchObject({ taskId: TASK_ID, activeSessionId: null });
  });

  it("selecting a session from the dropdown updates only the local preview via onSessionChange", () => {
    const { onSessionChange } = renderPreview(twoSessions(), "session-a");

    fireEvent.click(screen.getByTestId("sessions-dropdown-trigger"));

    expect(onSessionChange).toHaveBeenCalledWith("session-b");
  });
});
