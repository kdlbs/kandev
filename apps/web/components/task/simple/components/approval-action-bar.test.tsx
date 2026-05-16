import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import { ApprovalActionBar } from "./approval-action-bar";
import type { Task, TaskDecision } from "@/app/office/tasks/[id]/types";

const approveMock = vi.hoisted(() => vi.fn());
const requestChangesMock = vi.hoisted(() => vi.fn());
const toastErrorMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-extended-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-extended-api")>(
    "@/lib/api/domains/office-extended-api",
  );
  return {
    ...actual,
    approveTask: approveMock,
    requestTaskChanges: requestChangesMock,
  };
});

vi.mock("sonner", async () => {
  const actual = await vi.importActual<typeof import("sonner")>("sonner");
  return {
    ...actual,
    toast: { ...actual.toast, error: toastErrorMock },
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const COMMENT_FIELD = "approval-action-comment";
const SUBMIT_BTN = "approval-action-submit";
const REQUEST_CHANGES_TEXT = "please update docs";

const baseTask: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "in_review",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: ["agent-1"],
  decisions: [],
  createdBy: "user",
  createdAt: "2026-05-01T00:00:00Z",
  updatedAt: "2026-05-01T00:00:00Z",
};

function Wrapper({ children, task }: { children: ReactNode; task: Task }) {
  const ctx = {
    task,
    applyPatch: vi.fn(),
    restore: vi.fn(),
  };
  return (
    <StateProvider>
      <TaskOptimisticContextProvider value={ctx}>{children}</TaskOptimisticContextProvider>
    </StateProvider>
  );
}

describe("ApprovalActionBar visibility", () => {
  // V1: the action bar is dormant for the singleton human user — see
  // resolveViewerRoles in approval-action-bar.tsx. The unified participants
  // store (workflow_step_participants per ADR 0005 Wave C-backend) only
  // stores agent IDs today, so the human is never a real participant and
  // showing the bar would invite a cosmetic approval that wouldn't unblock
  // the agent-approver gate. These visibility tests pin that behaviour;
  // the submission tests below are skipped until human participants land.
  it("renders nothing even when approvers are configured (v1 humans-not-participants)", () => {
    render(
      <Wrapper task={baseTask}>
        <ApprovalActionBar task={baseTask} />
      </Wrapper>,
    );
    expect(screen.queryByTestId("approval-action-bar")).toBeNull();
  });

  it("renders nothing when the task has no reviewers or approvers", () => {
    const t: Task = { ...baseTask, approvers: [], reviewers: [] };
    render(
      <Wrapper task={t}>
        <ApprovalActionBar task={t} />
      </Wrapper>,
    );
    expect(screen.queryByTestId("approval-action-bar")).toBeNull();
  });

  it("renders nothing when the user already has an active decision", () => {
    const decision: TaskDecision = {
      id: "d1",
      taskId: "t-1",
      deciderType: "user",
      deciderId: "user",
      deciderName: "You",
      role: "approver",
      decision: "approved",
      comment: "",
      createdAt: "2026-05-01T01:00:00Z",
    };
    const t: Task = { ...baseTask, decisions: [decision] };
    render(
      <Wrapper task={t}>
        <ApprovalActionBar task={t} />
      </Wrapper>,
    );
    expect(screen.queryByTestId("approval-action-bar")).toBeNull();
  });
});

// Submission tests are dormant in v1. They will be re-enabled when
// human participants land and resolveViewerRoles can return a non-empty
// list for the singleton user.
describe.skip("ApprovalActionBar submission (dormant in v1)", () => {
  it("calls approveTask with the optional comment", async () => {
    approveMock.mockResolvedValueOnce({
      id: "d1",
      task_id: "t-1",
      decider_type: "user",
      decider_id: "user",
      role: "approver",
      decision: "approved",
      comment: "lgtm",
      created_at: "2026-05-01T01:00:00Z",
    });

    render(
      <Wrapper task={baseTask}>
        <ApprovalActionBar task={baseTask} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId("approval-action-approve"));
    const textarea = await screen.findByTestId(COMMENT_FIELD);
    fireEvent.change(textarea, { target: { value: "lgtm" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId(SUBMIT_BTN));
    });

    expect(approveMock).toHaveBeenCalledWith("t-1", "lgtm");
    expect(requestChangesMock).not.toHaveBeenCalled();
  });

  it("disables submit on request-changes until a comment is entered", async () => {
    render(
      <Wrapper task={baseTask}>
        <ApprovalActionBar task={baseTask} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId("approval-action-request-changes"));
    const submit = (await screen.findByTestId(SUBMIT_BTN)) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);

    const textarea = screen.getByTestId(COMMENT_FIELD);
    fireEvent.change(textarea, { target: { value: REQUEST_CHANGES_TEXT } });
    expect(submit.disabled).toBe(false);
  });

  it("calls requestTaskChanges with the required comment", async () => {
    requestChangesMock.mockResolvedValueOnce({
      id: "d2",
      task_id: "t-1",
      decider_type: "user",
      decider_id: "user",
      role: "approver",
      decision: "changes_requested",
      comment: REQUEST_CHANGES_TEXT,
      created_at: "2026-05-01T01:00:00Z",
    });

    render(
      <Wrapper task={baseTask}>
        <ApprovalActionBar task={baseTask} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId("approval-action-request-changes"));
    const textarea = await screen.findByTestId(COMMENT_FIELD);
    fireEvent.change(textarea, { target: { value: REQUEST_CHANGES_TEXT } });
    await act(async () => {
      fireEvent.click(screen.getByTestId(SUBMIT_BTN));
    });
    expect(requestChangesMock).toHaveBeenCalledWith("t-1", REQUEST_CHANGES_TEXT);
  });

  it("toasts and rolls back when the API rejects", async () => {
    approveMock.mockRejectedValueOnce(new Error("forbidden"));

    render(
      <Wrapper task={baseTask}>
        <ApprovalActionBar task={baseTask} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId("approval-action-approve"));
    await screen.findByTestId(COMMENT_FIELD);
    await act(async () => {
      fireEvent.click(screen.getByTestId(SUBMIT_BTN));
    });
    expect(toastErrorMock).toHaveBeenCalledWith("forbidden");
  });
});
