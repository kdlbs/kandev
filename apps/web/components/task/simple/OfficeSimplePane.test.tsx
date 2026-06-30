import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { Task, TaskSession } from "@/app/office/tasks/[id]/types";

vi.mock("@/components/routing/app-link", () => ({
  default: ({ href, children, ...props }: { href: string; children: ReactNode }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

vi.mock("@/app/office/components/office-topbar-portal", () => ({
  OfficeTopbarPortal: ({ children }: { children: ReactNode }) => (
    <div data-testid="office-topbar">{children}</div>
  ),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/app/office/components/execution-indicator", () => ({
  ExecutionIndicator: () => <div data-testid="execution-indicator" />,
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
    <div data-testid="chat-readonly">{readOnly ? "readonly" : "editable"}</div>
  ),
}));

import { OfficeSimplePane } from "./OfficeSimplePane";

afterEach(() => cleanup());

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

function renderPane(task: Task, sessions: TaskSession[]) {
  return render(
    <StateProvider>
      <OfficeSimplePane task={task} comments={[]} activity={[]} sessions={sessions} />
    </StateProvider>,
  );
}

describe("OfficeSimplePane comment composer", () => {
  it("keeps projectless office tasks editable after a completed session loads", () => {
    const task = { ...baseTask, status: "done" as const };
    const view = renderPane(task, []);

    view.rerender(
      <StateProvider>
        <OfficeSimplePane task={task} comments={[]} activity={[]} sessions={[completedSession]} />
      </StateProvider>,
    );

    expect(screen.getByTestId("chat-readonly").textContent).toBe("editable");
  });

  it("keeps closed tasks read-only when the latest session cannot be reused", () => {
    renderPane({ ...baseTask, status: "done" }, [completedSession, failedSession]);

    expect(screen.getByTestId("chat-readonly").textContent).toBe("readonly");
  });
});
