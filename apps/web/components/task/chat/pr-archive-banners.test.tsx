import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import { taskId as toTaskId, workspaceId, workflowId, type Task } from "@/lib/types/http";

const archiveAndSwitchMock = vi.fn();
const toastMock = vi.fn();
const mockGetSubtaskCount = vi.fn();
const taskPRsByTaskId = vi.hoisted(() => ({
  value: {
    "task-1": [{ pr_number: 42, state: "merged" }],
  } as Record<string, Array<{ pr_number: number; state: string }>>,
}));

const mockState = {
  clearPendingPrUrlForTaskPR: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useArchiveAndSwitchTask: () => archiveAndSwitchMock,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  getSubtaskCount: (...args: unknown[]) => mockGetSubtaskCount(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

vi.mock("@/lib/local-storage", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/local-storage")>()),
  markPRClosedBannerDismissed: vi.fn(),
  markPRMergedBannerDismissed: vi.fn(),
  wasPRClosedBannerDismissed: () => false,
  wasPRMergedBannerDismissed: () => false,
}));

import { PRClosedBanner, PRMergedBanner } from "./pr-archive-banners";

const CREATED_AT = "2026-05-04T00:00:00Z";
const MERGED_ARCHIVE_BUTTON = "pr-merged-archive-button";
const MERGED_ARCHIVE_CONFIRM = "pr-merged-archive-confirm";

function taskForBanner(id: string, title: string): Task {
  return {
    id: toTaskId(id),
    workspace_id: workspaceId("workspace-1"),
    workflow_id: workflowId("workflow-1"),
    workflow_step_id: "step-1",
    position: 0,
    title,
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [],
    primary_executor_type: "worktree",
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function createTestQueryClient() {
  const queryClient = makeQueryClient();
  for (const [taskId, prs] of Object.entries(taskPRsByTaskId.value)) {
    queryClient.setQueryData(qk.integrations.github.taskPr(taskId), prs);
  }
  queryClient.setQueryData(qk.tasks.detail("task-1"), taskForBanner("task-1", "Task One"));
  return queryClient;
}

function renderWithQuery(ui: ReactNode) {
  return render(<QueryClientProvider client={createTestQueryClient()}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  archiveAndSwitchMock.mockResolvedValue(undefined);
  mockGetSubtaskCount.mockResolvedValue({ count: 0 });
  taskPRsByTaskId.value = {
    "task-1": [{ pr_number: 42, state: "merged" }],
  };
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("PRMergedBanner", () => {
  it("opens the confirmation dialog instead of archiving directly", async () => {
    renderWithQuery(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));

    expect(await screen.findByTestId(MERGED_ARCHIVE_CONFIRM)).toBeTruthy();
    expect(screen.getByText(/Are you sure you want to archive "Task One"\?/)).toBeTruthy();
    expect(archiveAndSwitchMock).not.toHaveBeenCalled();
  });

  it("archives after confirming, without a success toast", async () => {
    renderWithQuery(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    fireEvent.click(await screen.findByTestId(MERGED_ARCHIVE_CONFIRM));

    await waitFor(() =>
      expect(archiveAndSwitchMock).toHaveBeenCalledWith("task-1", { cascade: false }),
    );
    expect(toastMock).not.toHaveBeenCalled();
  });

  it("does not archive when the dialog is cancelled", async () => {
    renderWithQuery(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    fireEvent.click(await screen.findByText("Cancel"));

    await waitFor(() => expect(screen.queryByTestId(MERGED_ARCHIVE_CONFIRM)).toBeNull());
    expect(archiveAndSwitchMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("pr-merged-banner")).toBeTruthy();
  });

  it("keeps the failure toast when archive fails", async () => {
    archiveAndSwitchMock.mockRejectedValueOnce(new Error("archive failed"));
    renderWithQuery(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    fireEvent.click(await screen.findByTestId(MERGED_ARCHIVE_CONFIRM));

    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith({
        description: "Failed to archive task",
        variant: "error",
      }),
    );
  });

  it("passes cascade=true through when the subtask checkbox is ticked", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 2 });
    renderWithQuery(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    fireEvent.click(await screen.findByTestId("archive-cascade-checkbox"));
    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_CONFIRM));

    await waitFor(() =>
      expect(archiveAndSwitchMock).toHaveBeenCalledWith("task-1", { cascade: true }),
    );
  });

  it("disables the confirm button while an archive is in flight", async () => {
    let resolveArchive: () => void = () => {};
    archiveAndSwitchMock.mockImplementation(
      () => new Promise<void>((resolve) => (resolveArchive = resolve)),
    );
    renderWithQuery(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    fireEvent.click(await screen.findByTestId(MERGED_ARCHIVE_CONFIRM));
    await waitFor(() => expect(archiveAndSwitchMock).toHaveBeenCalledTimes(1));

    // Reopen while the async archive is still pending: confirm must be disabled.
    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    const confirm = await screen.findByTestId<HTMLButtonElement>(MERGED_ARCHIVE_CONFIRM);
    expect(confirm.disabled).toBe(true);
    fireEvent.click(confirm);
    expect(archiveAndSwitchMock).toHaveBeenCalledTimes(1);

    resolveArchive();
    await waitFor(() =>
      expect(screen.getByTestId<HTMLButtonElement>(MERGED_ARCHIVE_CONFIRM).disabled).toBe(false),
    );
  });

  it("falls back to a generic title when the task is not in the Query cache", async () => {
    taskPRsByTaskId.value = {
      "task-2": [{ pr_number: 7, state: "merged" }],
    };
    renderWithQuery(<PRMergedBanner taskId="task-2" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));

    expect(await screen.findByTestId(MERGED_ARCHIVE_CONFIRM)).toBeTruthy();
    expect(screen.getByText(/Are you sure you want to archive "this task"\?/)).toBeTruthy();
  });
});

describe("PRClosedBanner", () => {
  it("archives through the confirmation dialog", async () => {
    taskPRsByTaskId.value = {
      "task-1": [{ pr_number: 42, state: "closed" }],
    };
    renderWithQuery(<PRClosedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId("pr-closed-archive-button"));
    expect(archiveAndSwitchMock).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByTestId("pr-closed-archive-confirm"));

    await waitFor(() =>
      expect(archiveAndSwitchMock).toHaveBeenCalledWith("task-1", { cascade: false }),
    );
  });
});
