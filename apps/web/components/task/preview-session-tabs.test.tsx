import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import type { ChatSubmitPayload } from "./chat/chat-input-container";
import type { EntityReference } from "@/lib/types/entity-reference";
import type { SessionsDropdownCreateSessionData } from "./sessions-dropdown";

type SessionsDropdownStubProps = {
  taskId: string | null;
  activeSessionId?: string | null;
  primarySessionId?: string | null;
  onSelectSession?: (sessionId: string) => void;
  onCreateSession?: (data: SessionsDropdownCreateSessionData) => void;
};

const mocks = vi.hoisted(() => ({
  getWebSocketClient: vi.fn(),
  request: vi.fn(),
  toast: vi.fn(),
  onSend: null as null | ((payload: ChatSubmitPayload) => Promise<void>),
  dropdownProps: [] as SessionsDropdownStubProps[],
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
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
      <>
        <button
          type="button"
          data-testid="sessions-dropdown-trigger"
          onClick={() => props.onSelectSession?.("session-b")}
        >
          Agents
        </button>
        <button
          type="button"
          data-testid="sessions-dropdown-new-session"
          onClick={() =>
            props.onCreateSession?.({
              prompt: "hello",
              agentProfileId: "profile-1",
              executorId: "executor-1",
            })
          }
        >
          New
        </button>
      </>
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
  mocks.request.mockReset();
  mocks.toast.mockReset();
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

const PREVIEW_TASK_ID = "task-2";

function fixturePreviewSession(id: string, startedAt: string): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId(PREVIEW_TASK_ID),
    state: "COMPLETED",
    started_at: startedAt,
    updated_at: startedAt,
  };
}

function twoPreviewSessions(): TaskSession[] {
  return [
    fixturePreviewSession("session-a", "2026-07-21T00:00:00Z"),
    fixturePreviewSession("session-b", "2026-07-21T00:01:00Z"),
  ];
}

function renderPreview(
  sessions: TaskSession[],
  sessionId: string | null,
  onSessionChange = vi.fn(),
  primarySessionId: string | null = null,
) {
  render(
    <StateProvider
      initialState={{
        taskSessionsByTask: {
          itemsByTaskId: { [PREVIEW_TASK_ID]: sessions },
          loadedByTaskId: { [PREVIEW_TASK_ID]: true },
          loadingByTaskId: {},
        },
      }}
    >
      <PreviewSessionTabs
        taskId={PREVIEW_TASK_ID}
        sessionId={sessionId}
        primarySessionId={primarySessionId}
        onSessionChange={onSessionChange}
      />
    </StateProvider>,
  );
  return { onSessionChange };
}

describe("PreviewSessionTabs agents dropdown", () => {
  const TASK_ID = PREVIEW_TASK_ID;
  const twoSessions = twoPreviewSessions;

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

  it("threads the previewed task's primarySessionId to the dropdown instead of the global active task", () => {
    renderPreview(twoSessions(), "session-a", vi.fn(), "session-b");

    expect(mocks.dropdownProps.at(-1)).toMatchObject({ primarySessionId: "session-b" });
  });
});

describe("PreviewSessionTabs new session creation", () => {
  const TASK_ID = PREVIEW_TASK_ID;

  it("creating a session from the dropdown launches it inline and selects it locally, without navigating", async () => {
    mocks.request.mockResolvedValue({
      success: true,
      task_id: TASK_ID,
      session_id: "session-new",
      state: "STARTING",
    });
    mocks.getWebSocketClient.mockReturnValue({ request: mocks.request });
    const { onSessionChange } = renderPreview(twoPreviewSessions(), "session-a");

    fireEvent.click(screen.getByTestId("sessions-dropdown-new-session"));
    await vi.waitFor(() => expect(onSessionChange).toHaveBeenCalledWith("session-new"));

    expect(mocks.request).toHaveBeenCalledWith(
      "session.launch",
      expect.objectContaining({
        task_id: TASK_ID,
        intent: "start",
        agent_profile_id: "profile-1",
        executor_id: "executor-1",
        prompt: "hello",
      }),
      expect.any(Number),
    );
    expect(mocks.toast).not.toHaveBeenCalled();
  });

  it("surfaces a toast instead of navigating when inline session creation fails", async () => {
    mocks.request.mockRejectedValue(new Error("launch failed"));
    mocks.getWebSocketClient.mockReturnValue({ request: mocks.request });
    const { onSessionChange } = renderPreview(twoPreviewSessions(), "session-a");

    fireEvent.click(screen.getByTestId("sessions-dropdown-new-session"));
    await vi.waitFor(() => expect(mocks.toast).toHaveBeenCalled());

    expect(mocks.toast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "launch failed", variant: "error" }),
    );
    expect(onSessionChange).not.toHaveBeenCalledWith("session-new");
  });
});
