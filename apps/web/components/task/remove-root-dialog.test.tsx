import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { TaskRepositoryRoot } from "@/hooks/domains/session/use-task-repository-roots";

const detachMock = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  detachTaskRepository: (...args: unknown[]) => detachMock(...args),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ clearGitRepositoryStatus: vi.fn() }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isTablet: false, isFinePointer: true }),
}));

import { RemoveRootDialog } from "./remove-root-dialog";

afterEach(() => {
  cleanup();
  detachMock.mockReset();
});

const root: TaskRepositoryRoot = {
  key: "tr-adopted",
  taskId: "task-1",
  sessionId: "session-1",
  taskRepositoryId: "tr-adopted",
  repositoryId: "repo-1",
  repositoryName: "kandev",
  trackerName: "fix-b",
  path: "/task-root/.claude/worktrees/fix-b",
  relativePath: ".claude/worktrees/fix-b",
  branch: "fix-b",
  worktreeId: "wt-adopted",
  environmentId: "env-1",
  isPrimary: false,
  removable: true,
  adopted: true,
  displayLabel: "kandev · fix-b",
};

describe("RemoveRootDialog", () => {
  it("detaches the root and states that files, branch, and PRs stay", async () => {
    detachMock.mockResolvedValue({ detached: true });
    const onOpenChange = vi.fn();
    render(<RemoveRootDialog open root={root} onOpenChange={onOpenChange} />);

    expect(screen.getByText(/folder, its branch, and any pull requests stay/i)).toBeTruthy();
    fireEvent.click(screen.getByTestId("remove-root-confirm"));

    await vi.waitFor(() => expect(detachMock).toHaveBeenCalledWith("task-1", "tr-adopted"));
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });
});
