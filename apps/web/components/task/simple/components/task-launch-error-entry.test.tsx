import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { TaskLaunchErrorEntry } from "./task-launch-error-entry";
import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummaryActiveError } from "@/lib/types/task-status-summary";

const { requestMock, toastMock } = vi.hoisted(() => ({
  requestMock: vi.fn(),
  toastMock: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/components/task/task-launch-branch-picker", () => ({
  TaskLaunchBranchPicker: ({
    trigger,
    onSelect,
  }: {
    trigger: ReactNode;
    onSelect: (branch: string) => Promise<void>;
  }) => (
    <>
      {trigger}
      <button
        type="button"
        data-testid="mock-branch-option"
        onClick={() => void onSelect("develop")}
      >
        develop
      </button>
    </>
  ),
}));

const TASK_ID = "task-1";
const TASK_REPOSITORY_ID = "task-repo-1";
const ERROR_STAMP = "launch-stamp-1";

const error: TaskStatusSummaryActiveError = {
  task_repository_id: TASK_REPOSITORY_ID,
  stamp: ERROR_STAMP,
  occurred_at: "2026-08-19T10:00:00Z",
  preview: "The selected base branch is not available.",
  category: "base_branch_missing",
  recovery_actions: ["retry_default", "pick_base_branch", "mark_review_done"],
};

beforeEach(() => {
  requestMock.mockReset().mockResolvedValue({ ok: true });
  toastMock.mockReset();
});

afterEach(() => cleanup());

describe("TaskLaunchErrorEntry", () => {
  it("renders the bounded preview and sends the exact task recovery payload", async () => {
    render(
      <TaskLaunchErrorEntry
        taskId={TASK_ID}
        workspaceId="workspace-1"
        repositories={[
          {
            id: TASK_REPOSITORY_ID,
            task_id: TASK_ID as TaskRepository["task_id"],
            repository_id: "repo-1" as never,
            base_branch: "main",
            position: 0,
            created_at: "2026-08-19T00:00:00Z",
            updated_at: "2026-08-19T00:00:00Z",
          },
        ]}
        error={error}
      />,
    );

    expect(screen.getByTestId("task-launch-error-entry")).toBeTruthy();
    expect(screen.getByText(error.preview)).toBeTruthy();
    fireEvent.click(screen.getByTestId("task-launch-retry_default-button"));

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    expect(requestMock).toHaveBeenCalledWith("task.launch.recover", {
      task_id: TASK_ID,
      task_repository_id: TASK_REPOSITORY_ID,
      action: "retry_default",
      error_stamp: ERROR_STAMP,
    });
  });

  it("includes the selected branch in the row-scoped recovery payload", async () => {
    render(
      <TaskLaunchErrorEntry
        taskId={TASK_ID}
        workspaceId="workspace-1"
        repositories={[]}
        error={error}
      />,
    );

    fireEvent.click(screen.getByTestId("mock-branch-option"));
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    expect(requestMock).toHaveBeenCalledWith("task.launch.recover", {
      task_id: TASK_ID,
      task_repository_id: TASK_REPOSITORY_ID,
      action: "pick_base_branch",
      base_branch: "develop",
      error_stamp: ERROR_STAMP,
    });
  });

  it("waits for a branch selection before sending the picker action", async () => {
    render(
      <TaskLaunchErrorEntry
        taskId={TASK_ID}
        workspaceId="workspace-1"
        repositories={[]}
        error={error}
      />,
    );

    fireEvent.click(screen.getByTestId("task-launch-pick_base_branch-button"));
    await waitFor(() => expect(requestMock).not.toHaveBeenCalled());
  });

  it("coalesces concurrent recovery requests from duplicate task surfaces", async () => {
    let resolveRequest: ((value: { ok: boolean }) => void) | undefined;
    requestMock.mockImplementation(
      () =>
        new Promise<{ ok: boolean }>((resolve) => {
          resolveRequest = resolve;
        }),
    );
    render(
      <>
        <TaskLaunchErrorEntry
          taskId={TASK_ID}
          workspaceId="workspace-1"
          repositories={[]}
          error={error}
        />
        <TaskLaunchErrorEntry
          taskId={TASK_ID}
          workspaceId="workspace-1"
          repositories={[]}
          error={error}
        />
      </>,
    );

    const buttons = screen.getAllByTestId("task-launch-retry_default-button");
    fireEvent.click(buttons[0]);
    fireEvent.click(buttons[1]);

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    resolveRequest?.({ ok: true });
    await waitFor(() => expect(screen.queryAllByTestId("task-launch-error-entry")).toHaveLength(2));
  });
});
