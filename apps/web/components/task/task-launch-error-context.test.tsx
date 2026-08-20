import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { TaskLaunchErrorProvider, useTaskLaunchErrorContext } from "./task-launch-error-context";

const { fetchTaskMock, toastMock } = vi.hoisted(() => ({
  fetchTaskMock: vi.fn(),
  toastMock: vi.fn(),
}));

vi.mock("@/lib/api", () => ({ fetchTask: fetchTaskMock }));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

const initialSummary: TaskStatusSummary = {
  revision: 1,
  updated_at: "2026-08-20T00:00:00Z",
  active_error: {
    stamp: "initial",
    occurred_at: "2026-08-20T00:00:00Z",
    preview: "initial error",
    category: "base_branch_missing",
  },
};

function SummaryStamp() {
  const context = useTaskLaunchErrorContext();
  return (
    <span data-testid="summary-stamp">{context?.statusSummary?.active_error?.stamp ?? "none"}</span>
  );
}

function renderProvider(statusSummary: TaskStatusSummary) {
  return render(
    <TaskLaunchErrorProvider
      value={{ taskId: "task-1", workspaceId: "workspace-1", statusSummary }}
    >
      <SummaryStamp />
    </TaskLaunchErrorProvider>,
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

beforeEach(() => {
  fetchTaskMock.mockReset();
  toastMock.mockReset();
});

describe("TaskLaunchErrorProvider", () => {
  it("keeps a newer polled summary when an older response resolves later", async () => {
    vi.useFakeTimers();
    const older = deferred<{ status_summary: TaskStatusSummary }>();
    const newer = deferred<{ status_summary: TaskStatusSummary }>();
    fetchTaskMock.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    renderProvider(initialSummary);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });

    const newerSummary: TaskStatusSummary = {
      ...initialSummary,
      revision: 3,
      active_error: { ...initialSummary.active_error!, stamp: "newer" },
    };
    await act(async () => {
      newer.resolve({ status_summary: newerSummary });
      await newer.promise;
    });
    expect(screen.getByTestId("summary-stamp").textContent).toBe("newer");

    const olderSummary: TaskStatusSummary = {
      ...initialSummary,
      revision: 2,
      active_error: { ...initialSummary.active_error!, stamp: "older" },
    };
    await act(async () => {
      older.resolve({ status_summary: olderSummary });
      await older.promise;
    });
    expect(screen.getByTestId("summary-stamp").textContent).toBe("newer");
  });
});
