import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const archiveAndSwitchMock = vi.fn();
const toastMock = vi.fn();
const mockGetSubtaskCount = vi.fn();
const taskPRsByTaskId = vi.hoisted(() => ({
  value: {
    "task-1": [{ pr_number: 42, state: "merged" }],
  } as Record<string, Array<{ pr_number: number; state: string }>>,
}));

const mockState = {
  taskPRs: {
    get byTaskId() {
      return taskPRsByTaskId.value;
    },
  },
  kanban: {
    tasks: [{ id: "task-1", title: "Task One", primaryExecutorType: "worktree" }],
  },
  kanbanMulti: { snapshots: {} },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useArchiveAndSwitchTask: () => archiveAndSwitchMock,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/lib/api", () => ({
  getSubtaskCount: (...args: unknown[]) => mockGetSubtaskCount(...args),
}));

vi.mock("@/lib/local-storage", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/local-storage")>()),
  markPRClosedBannerDismissed: vi.fn(),
  markPRMergedBannerDismissed: vi.fn(),
  wasPRClosedBannerDismissed: () => false,
  wasPRMergedBannerDismissed: () => false,
}));

import { PRClosedBanner, PRMergedBanner } from "./pr-archive-banners";

const MERGED_ARCHIVE_BUTTON = "pr-merged-archive-button";
const MERGED_ARCHIVE_CONFIRM = "pr-merged-archive-confirm";

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
    render(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));

    expect(await screen.findByTestId(MERGED_ARCHIVE_CONFIRM)).toBeTruthy();
    expect(screen.getByText(/Are you sure you want to archive "Task One"\?/)).toBeTruthy();
    expect(archiveAndSwitchMock).not.toHaveBeenCalled();
  });

  it("archives after confirming, without a success toast", async () => {
    render(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    fireEvent.click(await screen.findByTestId(MERGED_ARCHIVE_CONFIRM));

    await waitFor(() =>
      expect(archiveAndSwitchMock).toHaveBeenCalledWith("task-1", { cascade: false }),
    );
    expect(toastMock).not.toHaveBeenCalled();
  });

  it("does not archive when the dialog is cancelled", async () => {
    render(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    fireEvent.click(await screen.findByText("Cancel"));

    await waitFor(() => expect(screen.queryByTestId(MERGED_ARCHIVE_CONFIRM)).toBeNull());
    expect(archiveAndSwitchMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("pr-merged-banner")).toBeTruthy();
  });

  it("keeps the failure toast when archive fails", async () => {
    archiveAndSwitchMock.mockRejectedValueOnce(new Error("archive failed"));
    render(<PRMergedBanner taskId="task-1" />);

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
    render(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_BUTTON));
    fireEvent.click(await screen.findByTestId("archive-cascade-checkbox"));
    fireEvent.click(screen.getByTestId(MERGED_ARCHIVE_CONFIRM));

    await waitFor(() =>
      expect(archiveAndSwitchMock).toHaveBeenCalledWith("task-1", { cascade: true }),
    );
  });

  it("falls back to a generic title when the task is not in the store", async () => {
    taskPRsByTaskId.value = {
      "task-2": [{ pr_number: 7, state: "merged" }],
    };
    render(<PRMergedBanner taskId="task-2" />);

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
    render(<PRClosedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId("pr-closed-archive-button"));
    expect(archiveAndSwitchMock).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByTestId("pr-closed-archive-confirm"));

    await waitFor(() =>
      expect(archiveAndSwitchMock).toHaveBeenCalledWith("task-1", { cascade: false }),
    );
  });
});
