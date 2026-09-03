import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import type { TaskPlan } from "@/lib/types/http-agents";
import type { ChatSubmitPayload } from "./chat/chat-input-container";
import type { EntityReference } from "@/lib/types/entity-reference";

const mocks = vi.hoisted(() => ({
  getWebSocketClient: vi.fn(),
  onSend: null as null | ((payload: ChatSubmitPayload) => Promise<void>),
  useTaskSessions: vi.fn(),
  useSessionResumption: vi.fn(),
  markTaskPlanSeen: vi.fn(),
  setTaskPlan: vi.fn(),
  setTaskPlanLoading: vi.fn(),
  getTaskPlan: vi.fn(),
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
  useTaskSessions: mocks.useTaskSessions,
}));
vi.mock("@/hooks/domains/session/use-session-resumption", () => ({
  useSessionResumption: mocks.useSessionResumption,
}));
vi.mock("@/lib/api/domains/plan-api", () => ({
  getTaskPlan: mocks.getTaskPlan,
}));
vi.mock("@/components/editors/tiptap/tiptap-plan-readonly", () => ({
  PlanReadOnlyMarkdown: (props: { content: string; testId?: string }) => (
    <div data-testid={props.testId}>{props.content}</div>
  ),
}));

const mockTaskPlansState = {
  byTaskId: {} as Record<string, TaskPlan | null>,
  loadedByTaskId: {} as Record<string, boolean>,
  loadingByTaskId: {} as Record<string, boolean>,
  lastSeenUpdatedAtByTaskId: {} as Record<string, string | undefined>,
};

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (state: any) => unknown) =>
    selector({
      agentProfiles: { items: [] },
      taskPlans: mockTaskPlansState,
      markTaskPlanSeen: mocks.markTaskPlanSeen,
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      setTaskPlan: mocks.setTaskPlan,
      setTaskPlanLoading: mocks.setTaskPlanLoading,
    }),
  }),
}));

import { PreviewSessionBody, PreviewSessionTabs } from "./preview-session-tabs";

const TASK_ID = "task-1";
const TIMESTAMP = "2026-07-21T00:00:00Z";
const PLAN_INDICATOR_TESTID = "preview-plan-tab-indicator";

const session: TaskSession = {
  id: toSessionId("session-1"),
  task_id: toTaskId(TASK_ID),
  state: "COMPLETED",
  started_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};

afterEach(() => {
  cleanup();
  mocks.getWebSocketClient.mockReset();
  mocks.onSend = null;
});

describe("PreviewSessionBody send failures", () => {
  it("rejects when the WebSocket client is unavailable", async () => {
    mocks.getWebSocketClient.mockReturnValue(null);
    render(<PreviewSessionBody session={session} taskId={TASK_ID} />);

    await expect(mocks.onSend?.({ message: "hello" })).rejects.toMatchObject({
      name: "MessageSendError",
      code: "connection-unavailable",
      message: "Connection unavailable. Reconnect and try again.",
    });
  });

  it("rethrows message.add failures to the chat input", async () => {
    const error = new Error("message.add failed");
    mocks.getWebSocketClient.mockReturnValue({ request: vi.fn().mockRejectedValue(error) });
    render(<PreviewSessionBody session={session} taskId={TASK_ID} />);

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
    render(<PreviewSessionBody session={session} taskId={TASK_ID} />);

    await mocks.onSend?.({
      message: "reference",
      attachments: [{ type: "image", data: "base64", mime_type: "image/png" }],
      entityReferences: [reference],
    });

    expect(request).toHaveBeenCalledWith(
      "message.add",
      {
        task_id: TASK_ID,
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

function agentPlan(overrides: Partial<TaskPlan> = {}): TaskPlan {
  return {
    id: "plan-1",
    task_id: TASK_ID,
    title: "Plan",
    content: "## Plan content",
    created_by: "agent",
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

describe("PreviewSessionTabs Plan tab", () => {
  beforeEach(() => {
    mocks.useTaskSessions.mockReturnValue({ sessions: [session], isLoaded: true });
    mocks.useSessionResumption.mockReturnValue({
      error: null,
      notice: null,
      resumeSession: vi.fn(),
    });
    mocks.getTaskPlan.mockResolvedValue(null);
    mockTaskPlansState.byTaskId = {};
    mockTaskPlansState.loadedByTaskId = {};
    mockTaskPlansState.loadingByTaskId = {};
    mockTaskPlansState.lastSeenUpdatedAtByTaskId = {};
  });

  afterEach(() => {
    cleanup();
    mocks.useTaskSessions.mockReset();
    mocks.useSessionResumption.mockReset();
    mocks.markTaskPlanSeen.mockReset();
    mocks.getTaskPlan.mockReset();
  });

  it("shows a Plan tab alongside session tabs", () => {
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.getByTestId("preview-plan-tab")).toBeTruthy();
    expect(screen.getByTestId(`preview-session-tab-${session.id}`)).toBeTruthy();
  });

  it("clicking the Plan tab swaps the chat body for the read-only plan and does not change the session", () => {
    mockTaskPlansState.byTaskId = { [TASK_ID]: agentPlan() };
    mockTaskPlansState.loadedByTaskId = { [TASK_ID]: true };
    const onSessionChange = vi.fn();

    render(
      <PreviewSessionTabs taskId={TASK_ID} sessionId={null} onSessionChange={onSessionChange} />,
    );
    expect(screen.getByTestId("preview-chat")).toBeTruthy();

    fireEvent.mouseDown(screen.getByTestId("preview-plan-tab"), { button: 0 });

    expect(screen.queryByTestId("preview-chat")).toBeNull();
    expect(screen.getByTestId("preview-plan-panel")).toBeTruthy();
    expect(screen.getByText("## Plan content")).toBeTruthy();
    expect(onSessionChange).not.toHaveBeenCalled();
  });

  it("shows the unseen indicator for an agent-authored plan the user hasn't seen", () => {
    mockTaskPlansState.byTaskId = { [TASK_ID]: agentPlan() };

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.getByTestId(PLAN_INDICATOR_TESTID)).toBeTruthy();
  });

  it("does not show the indicator for a user-authored plan", () => {
    mockTaskPlansState.byTaskId = { [TASK_ID]: agentPlan({ created_by: "user" }) };

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeNull();
  });

  it("does not show the indicator once the plan's updated_at has been seen", () => {
    mockTaskPlansState.byTaskId = { [TASK_ID]: agentPlan() };
    mockTaskPlansState.lastSeenUpdatedAtByTaskId = { [TASK_ID]: TIMESTAMP };

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeNull();
  });

  it("clears the indicator (marks seen) when the Plan tab is clicked", () => {
    mockTaskPlansState.byTaskId = { [TASK_ID]: agentPlan() };
    mockTaskPlansState.loadedByTaskId = { [TASK_ID]: true };

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    expect(screen.getByTestId(PLAN_INDICATOR_TESTID)).toBeTruthy();

    fireEvent.mouseDown(screen.getByTestId("preview-plan-tab"), { button: 0 });

    expect(mocks.markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
  });
});
