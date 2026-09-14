import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { TaskSharedError } from "./task-shared-error";

const { context, state, taskPreview } = vi.hoisted(() => {
  const taskPreview = "The task could not be prepared.";
  const state = { isMobile: false };
  const context = {
    taskId: "task-1",
    workspaceId: "workspace-1",
    statusSummary: {
      revision: 4,
      updated_at: "2026-09-14T10:00:00Z",
      task_error: {
        scope: "task" as const,
        stamp: "task-error-1",
        occurred_at: "2026-09-14T09:59:00Z",
        preview: taskPreview,
        recovery_actions: ["retry_launch" as const],
      },
    },
    repositories: [],
    claimTaskErrorAnnouncement: vi.fn((_stamp: string) => true),
  };
  return { context, state, taskPreview };
});

vi.mock("@/components/task/task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => context,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: state.isMobile }),
}));

vi.mock("./simple/components/task-launch-error-entry", () => ({
  TaskLaunchErrorEntry: () => <div data-testid="task-shared-error-details-content" />,
}));

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children, open }: { children: ReactNode; open?: boolean }) => (
    <div>{open ? children : null}</div>
  ),
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogDescription: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children, open }: { children: ReactNode; open?: boolean }) => (
    <div>{open ? children : null}</div>
  ),
  DrawerContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DrawerDescription: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DrawerHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DrawerTitle: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

describe("TaskSharedError", () => {
  beforeEach(() => {
    context.claimTaskErrorAnnouncement.mockReset();
    context.claimTaskErrorAnnouncement.mockReturnValue(true);
  });

  afterEach(() => {
    cleanup();
  });

  it("keeps a task-owned error above task content and opens desktop details", () => {
    state.isMobile = false;
    render(<TaskSharedError />);

    expect(screen.getByTestId("task-shared-error")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByTestId("task-shared-error-announcement").textContent).toContain(taskPreview);
    expect(screen.getByText(taskPreview)).toBeTruthy();
    expect(screen.queryByTestId("task-shared-error-details-content")).toBeNull();

    fireEvent.click(screen.getByTestId("task-shared-error-details"));
    expect(screen.getByTestId("task-shared-error-details-content")).toBeTruthy();
  });

  it("uses the mobile drawer for the same task error", () => {
    state.isMobile = true;
    render(<TaskSharedError reserveMobileTopBar />);

    expect(screen.getByTestId("task-shared-error").className).toContain(
      "mt-[calc(3.5rem+env(safe-area-inset-top,0px))]",
    );

    fireEvent.click(screen.getByTestId("task-shared-error-details"));
    expect(screen.getByTestId("task-shared-error-details-content")).toBeTruthy();
  });

  it("announces a new task error stamp once across component remounts", () => {
    let announcedStamp: string | null = null;
    context.claimTaskErrorAnnouncement.mockImplementation((stamp: string) => {
      if (announcedStamp === stamp) return false;
      announcedStamp = stamp;
      return true;
    });

    const first = render(<TaskSharedError />);
    expect(screen.getByTestId("task-shared-error-announcement").textContent).toContain(taskPreview);
    first.unmount();

    render(<TaskSharedError />);
    expect(screen.getByTestId("task-shared-error-announcement").textContent).toBe("");
    expect(context.claimTaskErrorAnnouncement).toHaveBeenCalledTimes(2);
  });
});
