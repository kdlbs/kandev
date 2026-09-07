import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { isTypedTaskLaunchError, TaskLaunchErrorEntry } from "./task-launch-error-entry";
import { TaskChatLaunchError } from "./task-chat-launch-error";
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
const WORKSPACE_ID = "workspace-1";
const OCCURRED_AT = "2026-08-19T10:00:00Z";
const ERROR_ENTRY_TEST_ID = "task-launch-error-entry";
const RETRY_LAUNCH_BUTTON_TEST_ID = "task-launch-retry_launch-button";

const error: TaskStatusSummaryActiveError = {
  task_repository_id: TASK_REPOSITORY_ID,
  stamp: ERROR_STAMP,
  occurred_at: OCCURRED_AT,
  preview: "The selected base branch is not available.",
  category: "base_branch_missing",
  recovery_actions: ["retry_default", "pick_base_branch", "mark_review_done"],
};

beforeEach(() => {
  requestMock.mockReset().mockResolvedValue({ ok: true });
  toastMock.mockReset();
});

afterEach(() => cleanup());

// eslint-disable-next-line max-lines-per-function -- shared recovery-entry fixtures keep these related flows together.
describe("TaskLaunchErrorEntry", () => {
  it.each(["provider_auth_required", "model_capacity"])(
    "does not classify ordinary failure code %s as a launch error",
    (category) => {
      expect(
        isTypedTaskLaunchError({
          ...error,
          category,
        }),
      ).toBe(false);
    },
  );

  it.each([
    "base_branch_missing",
    "default_branch_unresolved",
    "pr_already_closed",
    "workspace_checkout_failed",
    "generic_launch_failure",
  ])("classifies launch category %s as a typed launch error", (category) => {
    expect(
      isTypedTaskLaunchError({
        ...error,
        category,
      }),
    ).toBe(true);
  });

  it("renders checkout details and retries the launch without branch settings", async () => {
    const checkoutError: TaskStatusSummaryActiveError = {
      session_id: "session-1",
      stamp: "checkout-stamp-1",
      occurred_at: OCCURRED_AT,
      preview: "The workspace could not be prepared for this launch.",
      details: "The pull request head was fetched into its isolated ref.",
      category: "workspace_checkout_failed",
      recovery_actions: ["retry_launch"],
    };

    render(
      <TaskLaunchErrorEntry taskId={TASK_ID} workspaceId={WORKSPACE_ID} error={checkoutError} />,
    );

    expect(screen.getByText(checkoutError.preview)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /show details/i }));
    expect(screen.getByText(checkoutError.details!)).toBeTruthy();
    expect(screen.getByTestId(RETRY_LAUNCH_BUTTON_TEST_ID)).toBeTruthy();
    fireEvent.click(screen.getByTestId(RETRY_LAUNCH_BUTTON_TEST_ID));

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    expect(requestMock).toHaveBeenCalledWith("task.launch.recover", {
      task_id: TASK_ID,
      session_id: "session-1",
      action: "retry_launch",
      error_stamp: "checkout-stamp-1",
    });
  });

  it("keeps recovery failures inside the launch card", async () => {
    requestMock.mockRejectedValueOnce(new Error("recovery failed"));
    render(
      <TaskLaunchErrorEntry
        taskId={TASK_ID}
        workspaceId={WORKSPACE_ID}
        error={{ ...error, recovery_actions: ["retry_launch"] }}
      />,
    );

    fireEvent.click(screen.getByTestId(RETRY_LAUNCH_BUTTON_TEST_ID));

    await waitFor(() => expect(screen.getByTestId("task-launch-recovery-error")).toBeTruthy());
    expect(toastMock).not.toHaveBeenCalled();
  });

  it("clears a recovery failure when a newer launch error arrives", async () => {
    requestMock.mockRejectedValueOnce(new Error("recovery failed"));
    const { rerender } = render(
      <TaskLaunchErrorEntry
        taskId={TASK_ID}
        workspaceId={WORKSPACE_ID}
        error={{ ...error, recovery_actions: ["retry_launch"] }}
      />,
    );

    fireEvent.click(screen.getByTestId(RETRY_LAUNCH_BUTTON_TEST_ID));
    await waitFor(() => expect(screen.getByTestId("task-launch-recovery-error")).toBeTruthy());

    rerender(
      <TaskLaunchErrorEntry
        taskId={TASK_ID}
        workspaceId={WORKSPACE_ID}
        error={{
          ...error,
          stamp: "launch-stamp-2",
          recovery_actions: ["retry_launch"],
        }}
      />,
    );

    await waitFor(() => expect(screen.queryByTestId("task-launch-recovery-error")).toBeNull());
  });

  it("renders a typed summary error without recovery actions", () => {
    render(
      <TaskChatLaunchError
        taskId={TASK_ID}
        workspaceId={WORKSPACE_ID}
        statusSummary={{
          revision: 1,
          updated_at: OCCURRED_AT,
          active_error: { ...error, recovery_actions: [] },
        }}
      />,
    );

    expect(screen.getByTestId(ERROR_ENTRY_TEST_ID)).toBeTruthy();
    expect(screen.getByText(error.preview)).toBeTruthy();
    expect(screen.queryByTestId("task-launch-retry_default-button")).toBeNull();
  });

  it("renders a task-wide launch error while a prior session is selected", () => {
    render(
      <TaskChatLaunchError
        taskId={TASK_ID}
        workspaceId={WORKSPACE_ID}
        sessionId="prior-session"
        statusSummary={{
          revision: 1,
          updated_at: OCCURRED_AT,
          active_error: { ...error, recovery_actions: ["mark_review_done"] },
        }}
      />,
    );

    expect(screen.getByTestId(ERROR_ENTRY_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("task-launch-mark_review_done-button")).toBeTruthy();
  });

  it("renders the bounded preview and sends the exact task recovery payload", async () => {
    render(
      <TaskLaunchErrorEntry
        taskId={TASK_ID}
        workspaceId={WORKSPACE_ID}
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

    expect(screen.getByTestId(ERROR_ENTRY_TEST_ID)).toBeTruthy();
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
        workspaceId={WORKSPACE_ID}
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
        workspaceId={WORKSPACE_ID}
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
          workspaceId={WORKSPACE_ID}
          repositories={[]}
          error={error}
        />
        <TaskLaunchErrorEntry
          taskId={TASK_ID}
          workspaceId={WORKSPACE_ID}
          repositories={[]}
          error={error}
        />
      </>,
    );

    const buttons = screen.getAllByTestId("task-launch-retry_default-button");
    fireEvent.click(buttons[0]);
    fireEvent.click(buttons[1]);

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    await waitFor(() => {
      expect(buttons[0].getAttribute("disabled")).not.toBeNull();
      expect(buttons[1].getAttribute("disabled")).not.toBeNull();
    });
    resolveRequest?.({ ok: true });
    await waitFor(() => expect(screen.queryAllByTestId(ERROR_ENTRY_TEST_ID)).toHaveLength(2));
  });
});
