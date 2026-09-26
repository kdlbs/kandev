import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";

const FORK_ID = "fork-1";
const REMOVE_FORK_LABEL = "Remove fork";
const NEW_SESSION_FLOW_TEST_ID = "new-session-flow";
const FORK_ID_ATTRIBUTE = "data-fork-id";

const mockLoadSource = vi.fn();
const mockCreateSnapshot = vi.fn();
const mockLoadAttachments = vi.fn();
const mockRefreshEstimate = vi.fn();
const mockDiscardSnapshot = vi.fn();
const mockReset = vi.fn();
const mockOnOpenChange = vi.fn();
const mockTask = {
  id: "task-1",
  workspaceId: "workspace-1",
  workflowId: "workflow-1",
  workflowStepId: "step-1",
  title: "Source task",
};
const mockStoreState = {
  kanban: {
    tasks: [mockTask],
    workflowId: "workflow-1",
    steps: [{ id: "step-1", title: "Work" }],
  },
  kanbanMulti: { snapshots: {} },
  workspaces: { activeId: "workspace-1" },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockStoreState) => unknown) => selector(mockStoreState),
}));

vi.mock("@/hooks/domains/task/use-conversation-fork", () => ({
  useConversationFork: () => ({
    source: {
      taskId: "task-1",
      sessionId: "session-1",
      title: "Source task",
      revision: 1,
      cutoffMessageId: "message-2",
      cutoffTurnComplete: true,
      boundaries: [],
      attachments: [],
    },
    sourceLoading: false,
    attachmentsLoading: false,
    sourceError: null,
    snapshot: {
      descriptor: {
        id: FORK_ID,
        source_task_id: "task-1",
        source_session_id: "session-1",
        source_message_id: "message-2",
        source_revision: 1,
        compiler_version: "v1",
        content_hash: "hash-1",
        message_count: 2,
        text_bytes: 42,
        omissions: {},
        attachments: [],
        estimate: { estimated_tokens: 12, method: "o200k", attachments_unmeasured: false },
        created_at: "2026-09-23T10:00:00Z",
        expires_at: "2026-09-24T10:00:00Z",
        state: "draft",
      },
      content: { content: "frozen history", content_hash: "hash-1", compiler_version: "v1" },
    },
    snapshotLoading: false,
    snapshotError: null,
    estimateLoading: false,
    estimateError: null,
    loadSource: mockLoadSource,
    createSnapshot: mockCreateSnapshot,
    loadAttachments: mockLoadAttachments,
    refreshEstimate: mockRefreshEstimate,
    discardSnapshot: mockDiscardSnapshot,
    reset: mockReset,
  }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: ({
    open,
    conversationFork,
  }: {
    open: boolean;
    conversationFork?: { snapshot: { descriptor: { id: string } }; onRemove: () => void };
  }) =>
    open ? (
      <div
        data-testid={NEW_SESSION_FLOW_TEST_ID}
        data-fork-id={conversationFork?.snapshot.descriptor.id}
      >
        {conversationFork && (
          <button onClick={conversationFork.onRemove}>{REMOVE_FORK_LABEL}</button>
        )}
      </div>
    ) : null,
}));

vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: (props: {
    open: boolean;
    workspaceId: string;
    workflowId: string;
    defaultStepId: string;
    conversationFork?: {
      snapshot: { descriptor: { id: string } };
      creationRequestId: string;
      onRemove: () => void;
    };
  }) =>
    props.open ? (
      <div
        data-testid="conversation-fork-task-form"
        data-workspace-id={props.workspaceId}
        data-workflow-id={props.workflowId}
        data-step-id={props.defaultStepId}
        data-fork-id={props.conversationFork?.snapshot.descriptor.id}
        data-request-id={props.conversationFork?.creationRequestId}
      >
        {props.conversationFork && (
          <button onClick={props.conversationFork.onRemove}>{REMOVE_FORK_LABEL}</button>
        )}
      </div>
    ) : null,
}));

vi.mock("@/components/task/new-subtask-dialog", () => ({
  NewSubtaskDialog: (props: {
    open: boolean;
    parentTaskId: string;
    conversationFork?: {
      snapshot: { descriptor: { id: string } };
      creationRequestId: string;
      onRemove: () => void;
    };
  }) =>
    props.open ? (
      <div
        data-testid="conversation-fork-child-form"
        data-parent-id={props.parentTaskId}
        data-fork-id={props.conversationFork?.snapshot.descriptor.id}
        data-request-id={props.conversationFork?.creationRequestId}
      >
        {props.conversationFork && (
          <button onClick={props.conversationFork.onRemove}>{REMOVE_FORK_LABEL}</button>
        )}
      </div>
    ) : null,
}));

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div role="dialog">{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div role="dialog">{children}</div> : null,
  DrawerContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DrawerHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DrawerTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
  DrawerDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
}));

import { ConversationForkFlow } from "./conversation-fork-flow";

const MESSAGE: Message = {
  id: "message-2",
  session_id: "session-1" as Message["session_id"],
  task_id: "task-1" as Message["task_id"],
  author_type: "agent",
  type: "message",
  content: "Answer",
  created_at: "2026-09-23T10:00:00Z",
};

describe("ConversationForkFlow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSnapshot.mockResolvedValue({
      descriptor: { id: FORK_ID },
      content: { content: "frozen history", content_hash: "hash-1", compiler_version: "v1" },
    });
  });

  afterEach(() => cleanup());

  it("prepares a snapshot only after New agent is selected and opens the existing launch form", async () => {
    render(<ConversationForkFlow open onOpenChange={mockOnOpenChange} message={MESSAGE} />);

    expect(screen.getByText(/New agent on this task/)).toBeTruthy();
    expect(mockCreateSnapshot).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("conversation-fork-destination-agent"));
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() =>
      expect(screen.getByTestId(NEW_SESSION_FLOW_TEST_ID).getAttribute(FORK_ID_ATTRIBUTE)).toBe(
        FORK_ID,
      ),
    );
    expect(mockCreateSnapshot).toHaveBeenCalledWith({
      includeToolEvidence: false,
      attachmentIds: [],
    });
  });

  it.each([
    ["task", "conversation-fork-task-form"],
    ["child_task", "conversation-fork-child-form"],
  ] as const)(
    "opens the %s form with the frozen snapshot and stable request ID",
    async (destination, formTestId) => {
      render(<ConversationForkFlow open onOpenChange={mockOnOpenChange} message={MESSAGE} />);

      const row = screen.getByTestId(`conversation-fork-destination-${destination}`);
      expect(row.hasAttribute("disabled")).toBe(false);
      fireEvent.click(row);
      fireEvent.click(screen.getByRole("button", { name: "Continue" }));

      const form = await screen.findByTestId(formTestId);
      expect(form.getAttribute(FORK_ID_ATTRIBUTE)).toBe(FORK_ID);
      expect(form.getAttribute("data-request-id")).toMatch(/^[\w-]+$/);
      if (destination === "task") {
        expect(form.getAttribute("data-workspace-id")).toBe("workspace-1");
        expect(form.getAttribute("data-workflow-id")).toBe("workflow-1");
        expect(form.getAttribute("data-step-id")).toBe("step-1");
      } else {
        expect(form.getAttribute("data-parent-id")).toBe("task-1");
      }
      expect(mockCreateSnapshot).toHaveBeenCalledWith({
        includeToolEvidence: false,
        attachmentIds: [],
      });
    },
  );

  it.each([
    ["task", "conversation-fork-task-form"],
    ["child_task", "conversation-fork-child-form"],
  ] as const)(
    "removes fork context from the open %s form without closing it",
    async (destination, formTestId) => {
      render(<ConversationForkFlow open onOpenChange={mockOnOpenChange} message={MESSAGE} />);
      fireEvent.click(screen.getByTestId(`conversation-fork-destination-${destination}`));
      fireEvent.click(screen.getByRole("button", { name: "Continue" }));
      const form = await screen.findByTestId(formTestId);
      expect(form.getAttribute(FORK_ID_ATTRIBUTE)).toBe(FORK_ID);

      fireEvent.click(screen.getByRole("button", { name: REMOVE_FORK_LABEL }));

      await waitFor(() => expect(form.getAttribute(FORK_ID_ATTRIBUTE)).toBe(null));
      expect(screen.getByTestId(formTestId)).toBe(form);
      expect(mockDiscardSnapshot).toHaveBeenCalledOnce();
      expect(mockOnOpenChange).not.toHaveBeenCalled();
    },
  );

  it("removes fork context from the open agent form without closing it", async () => {
    render(<ConversationForkFlow open onOpenChange={mockOnOpenChange} message={MESSAGE} />);
    fireEvent.click(screen.getByTestId("conversation-fork-destination-agent"));
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    const form = await screen.findByTestId(NEW_SESSION_FLOW_TEST_ID);
    expect(form.getAttribute(FORK_ID_ATTRIBUTE)).toBe(FORK_ID);

    fireEvent.click(screen.getByRole("button", { name: REMOVE_FORK_LABEL }));

    await waitFor(() => expect(form.getAttribute(FORK_ID_ATTRIBUTE)).toBe(null));
    expect(screen.getByTestId(NEW_SESSION_FLOW_TEST_ID)).toBe(form);
    expect(mockDiscardSnapshot).toHaveBeenCalledOnce();
    expect(mockOnOpenChange).not.toHaveBeenCalled();
  });
});
