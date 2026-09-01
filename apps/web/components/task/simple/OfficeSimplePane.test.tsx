import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import type { Task, TaskSession } from "@/app/office/tasks/[id]/types";
import {
  OfficeTopbarChromeProvider,
  useOfficeTopbarChrome,
} from "@/app/office/components/office-topbar-context";

vi.mock("@/lib/api/domains/kanban-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/kanban-api")>(
    "@/lib/api/domains/kanban-api",
  );
  return {
    ...actual,
    updateTask: vi.fn(),
  };
});

const TITLE_HEADING_TESTID = "office-task-title";
const TITLE_INPUT_TESTID = "office-task-title-input";
const DESCRIPTION_READ_TESTID = "office-task-description";
const DESCRIPTION_TEXTAREA_TESTID = "office-task-description-textarea";
const SOME_DESCRIPTION = "Some description";

const { CHAT_EDITABLE, CHAT_READONLY, CHAT_READONLY_TEST_ID } = vi.hoisted(() => ({
  CHAT_EDITABLE: "editable",
  CHAT_READONLY: "readonly",
  CHAT_READONLY_TEST_ID: "chat-readonly",
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ href, children, ...props }: { href: string; children: ReactNode }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/app/office/components/execution-indicator", () => ({
  ExecutionIndicator: ({ status }: { status: string }) => (
    <div data-testid="execution-indicator">{status}</div>
  ),
}));

vi.mock("./components/topbar-working-indicator", () => ({
  TopbarWorkingIndicator: () => <div data-testid="topbar-working-indicator" />,
}));

vi.mock("./task-properties", () => ({
  TaskProperties: () => <aside data-testid="task-properties" />,
}));

vi.mock("./task-documents", () => ({
  TaskDocuments: () => <section data-testid="task-documents" />,
}));

vi.mock("../task-detail-context-panel", () => ({
  TaskDetailContextPanel: () => <section data-testid="task-detail-context" />,
}));

vi.mock("@/hooks/use-task-context", () => ({
  useTaskContext: () => ({ data: null, isLoading: false }),
}));

vi.mock("./stage-progress-bar", () => ({
  StageProgressBar: () => <div data-testid="stage-progress" />,
}));

vi.mock("./subtask-stepper", () => ({
  SubtaskStepper: () => <div data-testid="subtask-stepper" />,
}));

vi.mock("@/app/office/components/new-task-dialog", () => ({
  NewTaskDialog: () => null,
}));

vi.mock("@/components/task/TreeCancelDialog", () => ({
  TreeCancelDialog: () => null,
}));

vi.mock("@/lib/api/domains/tree-api", () => ({
  cancelTaskTree: vi.fn(),
  pauseTaskTree: vi.fn(),
  previewTaskTree: vi.fn().mockResolvedValue({ active_hold: null }),
  restoreTaskTree: vi.fn(),
  resumeTaskTree: vi.fn(),
}));

vi.mock("./chat-activity-tabs", () => ({
  ChatActivityTabs: ({ readOnly }: { readOnly: boolean }) => (
    <div data-testid={CHAT_READONLY_TEST_ID}>{readOnly ? CHAT_READONLY : CHAT_EDITABLE}</div>
  ),
}));

import { OfficeSimplePane } from "./OfficeSimplePane";
import { updateTask } from "@/lib/api/domains/kanban-api";

const mockedUpdateTask = vi.mocked(updateTask);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const baseTask: Task = {
  id: "task-1",
  workspaceId: "workspace-1",
  identifier: "E2E-1",
  title: "Projectless office task",
  status: "todo",
  priority: "medium",
  labels: [],
  assigneeAgentProfileId: "agent-1",
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "",
  createdAt: "2026-05-01T10:00:00Z",
  updatedAt: "2026-05-01T10:00:00Z",
};

const completedSession: TaskSession = {
  id: "session-1",
  agentProfileId: "agent-1",
  agentName: "CEO",
  agentRole: "agent",
  state: "COMPLETED",
  isPrimary: true,
  startedAt: "2026-05-01T10:00:00Z",
  completedAt: "2026-05-01T10:05:00Z",
  updatedAt: "2026-05-01T10:05:00Z",
};

const failedSession: TaskSession = {
  ...completedSession,
  id: "session-failed",
  state: "FAILED",
  completedAt: "2026-05-01T10:06:00Z",
  updatedAt: "2026-05-01T10:06:00Z",
};

const runningSession: TaskSession = {
  ...completedSession,
  id: "session-running",
  state: "RUNNING",
  completedAt: undefined,
  updatedAt: "2026-05-01T10:07:00Z",
};

function optimisticContextFor(task: Task) {
  return { task, applyPatch: vi.fn(), restore: vi.fn() };
}

function TopbarActions() {
  const chrome = useOfficeTopbarChrome();
  return <>{chrome?.actions}</>;
}

function PaneHarness({ task, sessions }: { task: Task; sessions: TaskSession[] }) {
  return (
    <StateProvider>
      <OfficeTopbarChromeProvider>
        <TaskOptimisticContextProvider value={optimisticContextFor(task)}>
          <OfficeSimplePane task={task} comments={[]} activity={[]} sessions={sessions} />
          <TopbarActions />
        </TaskOptimisticContextProvider>
      </OfficeTopbarChromeProvider>
    </StateProvider>
  );
}

function renderPane(task: Task, sessions: TaskSession[]) {
  return render(<PaneHarness task={task} sessions={sessions} />);
}

describe("OfficeSimplePane comment composer", () => {
  it("keeps projectless office tasks editable after a completed session loads", () => {
    const task = { ...baseTask, status: "done" as const };
    const view = renderPane(task, []);

    view.rerender(<PaneHarness task={task} sessions={[completedSession]} />);

    expect(screen.getByTestId(CHAT_READONLY_TEST_ID).textContent).toBe(CHAT_EDITABLE);
  });

  it("keeps closed tasks read-only when the latest session cannot be reused", () => {
    renderPane({ ...baseTask, status: "done" }, [completedSession, failedSession]);

    expect(screen.getByTestId(CHAT_READONLY_TEST_ID).textContent).toBe(CHAT_READONLY);
  });

  it("keeps completed tasks editable while a follow-up session is active", () => {
    renderPane({ ...baseTask, status: "done" }, [completedSession, runningSession]);

    expect(screen.getByTestId(CHAT_READONLY_TEST_ID).textContent).toBe(CHAT_EDITABLE);
  });

  it("keeps cancelled tasks read-only even with a reusable latest session", () => {
    renderPane({ ...baseTask, status: "cancelled" }, [completedSession]);

    expect(screen.getByTestId(CHAT_READONLY_TEST_ID).textContent).toBe(CHAT_READONLY);
  });
});

describe("OfficeSimplePane ExecutionIndicator wiring", () => {
  // Regression: the detail page's store-seeded task must feed ExecutionIndicator
  // the raw backend status, not the normalized canonical one, so it still tells
  // SCHEDULING (live) apart from WAITING_FOR_INPUT (not live) — the same
  // distinction task-row.tsx already preserves for the board/list views.
  it("prefers rawStatus over status so a SCHEDULING task still shows Live", () => {
    renderPane({ ...baseTask, status: "todo", rawStatus: "SCHEDULING" }, []);

    expect(screen.getByTestId("execution-indicator").textContent).toBe("SCHEDULING");
  });

  it("prefers rawStatus over status so a WAITING_FOR_INPUT task does not falsely show Live", () => {
    renderPane({ ...baseTask, status: "in_progress", rawStatus: "WAITING_FOR_INPUT" }, []);

    expect(screen.getByTestId("execution-indicator").textContent).toBe("WAITING_FOR_INPUT");
  });

  it("falls back to status when rawStatus is absent", () => {
    renderPane({ ...baseTask, status: "in_progress" }, []);

    expect(screen.getByTestId("execution-indicator").textContent).toBe("in_progress");
  });
});

describe("OfficeSimplePane editor composition (AC-35, AC-57)", () => {
  it("commits the title while a dirty description draft stays open with its draft intact (AC-35)", async () => {
    mockedUpdateTask.mockResolvedValueOnce({
      title: "New title",
      updated_at: "2026-05-02T00:00:00Z",
    } as never);
    const task = { ...baseTask, description: SOME_DESCRIPTION };
    renderPane(task, []);

    fireEvent.click(screen.getByTestId(DESCRIPTION_READ_TESTID));
    const textarea = screen.getByTestId(DESCRIPTION_TEXTAREA_TESTID) as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "Dirty draft" } });

    fireEvent.doubleClick(screen.getByTestId(TITLE_HEADING_TESTID));
    const titleInput = screen.getByTestId(TITLE_INPUT_TESTID);
    fireEvent.change(titleInput, { target: { value: "New title" } });
    await act(async () => {
      fireEvent.keyDown(titleInput, { key: "Enter" });
    });

    expect(mockedUpdateTask).toHaveBeenCalledWith(task.id, { title: "New title" });
    expect(screen.queryByTestId(TITLE_INPUT_TESTID)).toBeNull();
    expect((screen.getByTestId(DESCRIPTION_TEXTAREA_TESTID) as HTMLTextAreaElement).value).toBe(
      "Dirty draft",
    );
  });

  it("closes the title editor and discards its draft when the description editor opens (AC-57)", () => {
    const task = { ...baseTask, description: SOME_DESCRIPTION };
    renderPane(task, []);

    fireEvent.doubleClick(screen.getByTestId(TITLE_HEADING_TESTID));
    const titleInput = screen.getByTestId(TITLE_INPUT_TESTID);
    fireEvent.change(titleInput, { target: { value: "Uncommitted draft" } });

    // Opening the description editor moves focus off the title input.
    fireEvent.blur(titleInput);
    fireEvent.click(screen.getByTestId(DESCRIPTION_READ_TESTID));

    expect(screen.queryByTestId(TITLE_INPUT_TESTID)).toBeNull();
    expect(screen.getByTestId(TITLE_HEADING_TESTID).textContent).toBe(task.title);
    expect(screen.getByTestId(DESCRIPTION_TEXTAREA_TESTID)).toBeTruthy();
    expect(mockedUpdateTask).not.toHaveBeenCalled();
  });
});

describe("OfficeSimplePane editors have no status/lifecycle/archived gate (AC-30, AC-47)", () => {
  it("still opens the title and description editors when the task is cancelled with a failed session", () => {
    const task = { ...baseTask, status: "cancelled" as const, description: SOME_DESCRIPTION };
    renderPane(task, [completedSession, failedSession]);

    fireEvent.doubleClick(screen.getByTestId(TITLE_HEADING_TESTID));
    expect(screen.getByTestId(TITLE_INPUT_TESTID)).toBeTruthy();

    fireEvent.click(screen.getByTestId(DESCRIPTION_READ_TESTID));
    expect(screen.getByTestId(DESCRIPTION_TEXTAREA_TESTID)).toBeTruthy();
  });

  it("still opens the title and description editors while a session is RUNNING on a done task", () => {
    const task = { ...baseTask, status: "done" as const, description: SOME_DESCRIPTION };
    renderPane(task, [runningSession]);

    fireEvent.doubleClick(screen.getByTestId(TITLE_HEADING_TESTID));
    expect(screen.getByTestId(TITLE_INPUT_TESTID)).toBeTruthy();

    fireEvent.click(screen.getByTestId(DESCRIPTION_READ_TESTID));
    expect(screen.getByTestId(DESCRIPTION_TEXTAREA_TESTID)).toBeTruthy();
  });
});
