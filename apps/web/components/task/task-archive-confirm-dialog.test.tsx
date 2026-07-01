import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

const mockGetSubtaskCount = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  getSubtaskCount: (...args: unknown[]) => mockGetSubtaskCount(...args),
}));

import { TaskArchiveConfirmDialog } from "./task-archive-confirm-dialog";

function renderWithQuery(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>);
}

beforeEach(() => {
  mockGetSubtaskCount.mockReset();
  mockGetSubtaskCount.mockResolvedValue({ count: 0 });
});

afterEach(cleanup);

describe("TaskArchiveConfirmDialog cleanup copy", () => {
  it("renders local-executor reassurance about untouched repo", () => {
    renderWithQuery(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="local"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/directly in your repo/i)).toBeTruthy();
  });

  it("renders worktree-executor copy about worktree + branch removal", () => {
    renderWithQuery(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="worktree"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/worktree and its branch will be deleted/i)).toBeTruthy();
  });

  it("warns about sandbox destruction for sprites executor", () => {
    renderWithQuery(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="sprites"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/Sprites cloud sandbox/i)).toBeTruthy();
    expect(screen.getByText(/uncommitted work/i)).toBeTruthy();
  });

  it("describes Docker container removal for local_docker", () => {
    renderWithQuery(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="local_docker"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/Docker container/i)).toBeTruthy();
  });

  it("renders grouped copy for bulk archive", () => {
    renderWithQuery(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        isBulkOperation
        count={3}
        taskIds={["a", "b", "c"]}
        executorTypes={["sprites", "worktree", "worktree"]}
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/2 worktrees/i)).toBeTruthy();
    expect(screen.getByText(/1 Sprites sandbox/i)).toBeTruthy();
  });

  it("no longer renders the old hardcoded worktree line for non-worktree executors", () => {
    renderWithQuery(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="local"
        onConfirm={() => {}}
      />,
    );
    expect(
      screen.queryByText(
        /This will delete the task's worktree and stop any running agent sessions/,
      ),
    ).toBeNull();
  });
});
