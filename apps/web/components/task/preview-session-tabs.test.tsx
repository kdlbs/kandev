import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { ChatSubmitPayload } from "./chat/chat-input-container";
import type { EntityReference } from "@/lib/types/entity-reference";

const mocks = vi.hoisted(() => ({
  getWebSocketClient: vi.fn(),
  onSend: null as null | ((payload: ChatSubmitPayload) => Promise<void>),
  sessions: [] as TaskSession[],
  agentProfiles: [] as AgentProfileOption[],
  primarySessionId: null as string | null,
  kanbanTasks: [{ id: "task-1", primarySessionId: null as string | null }],
  kanbanSnapshots: {} as Record<
    string,
    { tasks: Array<{ id: string; primarySessionId: string | null }> }
  >,
  setPrimary: vi.fn(),
  stop: vi.fn(),
  resume: vi.fn(),
  remove: vi.fn(async (_sessionId?: string, _options?: unknown) => true),
  renameSession: vi.fn(async () => undefined),
  upsertTaskSessionFromEvent: vi.fn(),
  taskSessionItems: {} as Record<string, TaskSession>,
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
vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ sessions: mocks.sessions, isLoaded: true }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: mocks.agentProfiles },
      kanban: {
        tasks: mocks.kanbanTasks.map((task) => ({
          ...task,
          primarySessionId: mocks.primarySessionId ?? task.primarySessionId,
        })),
      },
      kanbanMulti: { snapshots: mocks.kanbanSnapshots },
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      taskSessions: { items: mocks.taskSessionItems },
      upsertTaskSessionFromEvent: mocks.upsertTaskSessionFromEvent,
    }),
  }),
}));
vi.mock("@/hooks/domains/session/use-session-resumption", () => ({
  useSessionResumption: () => ({ error: null, notice: null, resumeSession: vi.fn() }),
}));
vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: ({
    sessionId,
    onDeleted,
  }: {
    sessionId?: string;
    onDeleted?: () => void;
  }) => ({
    setPrimary: () => mocks.setPrimary(sessionId),
    stop: () => mocks.stop(sessionId),
    resume: () => mocks.resume(sessionId),
    remove: async (options?: unknown) => {
      const ok = await mocks.remove(sessionId, options);
      if (ok) onDeleted?.();
      return ok;
    },
  }),
  isSessionStoppable: (s: string) =>
    s === "RUNNING" || s === "STARTING" || s === "WAITING_FOR_INPUT",
  isSessionDeletable: (s: string) => s !== "RUNNING" && s !== "STARTING",
  isSessionResumable: (s: string) => s === "COMPLETED" || s === "FAILED" || s === "CANCELLED",
}));
vi.mock("@/components/agent-logo", () => ({
  AgentLogo: ({ agentName }: { agentName: string }) => (
    <span data-testid={`agent-logo-${agentName}`} />
  ),
}));
vi.mock("@/components/task/handoff-profile-menu-items", () => ({
  HandoffContextMenuSub: () => (
    <div role="menuitem" data-testid="session-handoff-submenu">
      Handoff
    </div>
  ),
}));
vi.mock("@/components/task/share/share-dialog", () => ({
  ShareDialog: () => null,
}));
vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));
vi.mock("@/lib/api/domains/session-api", () => ({
  renameSession: mocks.renameSession,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn(), updateToast: vi.fn() }),
}));
// Real TabRenameInput auto-focuses on mount inside a useEffect, which races
// against Radix's context-menu-close focus restoration under jsdom's
// synchronous focus/blur emulation (not reproducible in a real browser —
// the identical pattern already ships for terminal tabs). Stubbing it keeps
// this suite testing the preview's own wiring (open → commit through the
// shared rename hook), not TabRenameInput's unrelated focus management.
vi.mock("./tab-rename-input", () => ({
  TabRenameInput: ({
    initial,
    onCommit,
    testId,
  }: {
    initial: string;
    onCommit: (next: string) => void;
    testId?: string;
  }) => {
    let value = initial;
    return (
      <input
        data-testid={testId}
        defaultValue={initial}
        onChange={(e) => {
          value = e.target.value;
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") onCommit(value);
        }}
      />
    );
  },
}));

import { PreviewSessionBody, PreviewSessionTabs } from "./preview-session-tabs";

const TASK_ID = "task-1";
const START_TIME = "2026-07-21T00:00:00Z";
const SESSION_A_TAB_TESTID = "preview-session-tab-session-a";

function makeSession(id: string, overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id,
    task_id: TASK_ID,
    state: "COMPLETED",
    started_at: START_TIME,
    updated_at: START_TIME,
    ...overrides,
  } as TaskSession;
}

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
  mocks.sessions = [];
  mocks.agentProfiles = [];
  mocks.primarySessionId = null;
  mocks.kanbanTasks = [{ id: "task-1", primarySessionId: null }];
  mocks.kanbanSnapshots = {};
  mocks.setPrimary.mockClear();
  mocks.stop.mockClear();
  mocks.resume.mockClear();
  mocks.remove.mockClear();
  mocks.remove.mockImplementation(async () => true);
  mocks.renameSession.mockClear();
  mocks.upsertTaskSessionFromEvent.mockClear();
  mocks.taskSessionItems = {};
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

describe("PreviewSessionTabs tab label", () => {
  it("shows the session's custom name instead of the agent-derived label", () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED", name: "My renamed agent" })];
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    expect(screen.getByTestId(SESSION_A_TAB_TESTID).textContent).toContain("My renamed agent");
  });
});

describe("PreviewSessionTabs session context menu", () => {
  it("renders the full lifecycle menu on right-click, without Close Others", () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));

    expect(screen.getByRole("menuitem", { name: "Rename…" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Set as Primary" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Resume" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Share" })).toBeTruthy();
    expect(screen.getByTestId("session-handoff-submenu")).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Close Others" })).toBeNull();
  });

  it("disables Set as Primary when the session is already primary", () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    mocks.primarySessionId = "session-a";
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));

    expect(
      screen.getByRole("menuitem", { name: "Set as Primary" }).getAttribute("aria-disabled"),
    ).toBe("true");
  });

  it("uses the latest primary session from a multi-workflow snapshot", () => {
    mocks.sessions = [
      makeSession("session-a", { state: "RUNNING" }),
      makeSession("session-b", { state: "RUNNING" }),
    ];
    mocks.kanbanTasks = [];
    mocks.kanbanSnapshots = {
      "workflow-2": { tasks: [{ id: TASK_ID, primarySessionId: "session-a" }] },
    };
    const { rerender } = render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-b" />);

    mocks.kanbanSnapshots = {
      "workflow-2": { tasks: [{ id: TASK_ID, primarySessionId: "session-b" }] },
    };
    rerender(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-b" />);
    fireEvent.contextMenu(screen.getByTestId("preview-session-tab-session-b"));

    expect(
      screen.getByRole("menuitem", { name: "Set as Primary" }).getAttribute("aria-disabled"),
    ).toBe("true");
  });

  it("shows Stop (not Resume) and hides Delete for a running session", () => {
    mocks.sessions = [makeSession("session-a", { state: "RUNNING" })];
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));

    expect(screen.getByRole("menuitem", { name: "Stop" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Resume" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Delete" })).toBeNull();
  });

  it("opens the inline rename input and commits through the shared rename hook", async () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    mocks.taskSessionItems = { "session-a": mocks.sessions[0] };
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));
    fireEvent.click(screen.getByRole("menuitem", { name: "Rename…" }));

    const input = await screen.findByTestId("preview-session-tab-rename-input");
    fireEvent.change(input, { target: { value: "renamed session" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await vi.waitFor(() =>
      expect(mocks.renameSession).toHaveBeenCalledWith("session-a", "renamed session"),
    );
    expect(screen.queryByTestId("preview-session-tab-rename-input")).toBeNull();
  });

  it("opens the delete confirmation popover and calls session.delete on confirm", async () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    const confirm = await screen.findByTestId("session-delete-confirm");
    fireEvent.click(confirm);

    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );
  });
});

async function deleteViaContextMenu(tabTestId: string) {
  fireEvent.contextMenu(screen.getByTestId(tabTestId));
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
  const confirm = await screen.findByTestId("session-delete-confirm");
  fireEvent.click(confirm);
}

/** Makes `mocks.remove` hang until the returned resolver is called, so a
 * test can change props (simulate the user switching tabs) while a
 * `session.delete` round-trip is still in flight. */
function deferDelete() {
  let resolveDelete: (ok: boolean) => void = () => {};
  mocks.remove.mockImplementation(
    () =>
      new Promise<boolean>((resolve) => {
        resolveDelete = resolve;
      }),
  );
  return {
    settle: async (ok: boolean) => {
      await act(async () => {
        resolveDelete(ok);
        await Promise.resolve();
        await Promise.resolve();
      });
    },
  };
}

describe("PreviewSessionTabs delete session selection", () => {
  it("does not change the viewed session when a non-active tab is deleted", async () => {
    mocks.sessions = [
      makeSession("session-a", { state: "COMPLETED" }),
      makeSession("session-b", { state: "COMPLETED" }),
      makeSession("session-c", { state: "COMPLETED" }),
    ];
    const onSessionChange = vi.fn();
    render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-b"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu("preview-session-tab-session-a");

    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );
    expect(onSessionChange).not.toHaveBeenCalled();
  });

  it("re-points to a remaining session when the active tab is deleted", async () => {
    mocks.sessions = [
      makeSession("session-a", { state: "COMPLETED" }),
      makeSession("session-b", { state: "COMPLETED" }),
    ];
    const onSessionChange = vi.fn();
    render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-a"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu("preview-session-tab-session-a");

    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );
    expect(onSessionChange).toHaveBeenCalledWith("session-b");
  });

  it("calls onSessionChange with null when the last remaining session is deleted", async () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    const onSessionChange = vi.fn();
    render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-a"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu(SESSION_A_TAB_TESTID);

    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );
    expect(onSessionChange).toHaveBeenCalledWith(null);
  });
});

describe("PreviewSessionTabs deferred delete", () => {
  it("does not redirect when the active tab changes while its own delete is still pending", async () => {
    mocks.sessions = [
      makeSession("session-a", { state: "COMPLETED" }),
      makeSession("session-b", { state: "COMPLETED" }),
      makeSession("session-c", { state: "COMPLETED" }),
    ];
    const onSessionChange = vi.fn();
    const pendingDelete = deferDelete();
    const { rerender } = render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-a"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu(SESSION_A_TAB_TESTID);
    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );

    // User switches away from the tab being deleted before session.delete settles.
    rerender(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-b"
        onSessionChange={onSessionChange}
      />,
    );
    await pendingDelete.settle(true);

    expect(onSessionChange).not.toHaveBeenCalled();
  });

  it("redirects once the pending delete resolves for a tab that became active in the meantime", async () => {
    mocks.sessions = [
      makeSession("session-a", { state: "COMPLETED" }),
      makeSession("session-b", { state: "COMPLETED" }),
    ];
    const onSessionChange = vi.fn();
    const pendingDelete = deferDelete();
    const { rerender } = render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-a"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu("preview-session-tab-session-b");
    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-b", { feedback: "toast" }),
    );

    // User switches to the tab now pending deletion before session.delete settles.
    rerender(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-b"
        onSessionChange={onSessionChange}
      />,
    );
    await pendingDelete.settle(true);

    expect(onSessionChange).toHaveBeenCalledWith("session-a");
  });
});
