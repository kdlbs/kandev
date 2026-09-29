import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

const mocks = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.isMobile,
    isFinePointer: mocks.isFinePointer,
  }),
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: ({
    repositoryName,
    taskId,
  }: {
    repositoryName?: string;
    taskId?: string;
  }) => (
    <span
      data-testid="external-vcs-link"
      data-repository-name={repositoryName}
      data-task-id={taskId}
    />
  ),
  ExternalVcsFileMenuItem: ({
    repositoryName,
    taskId,
  }: {
    repositoryName?: string;
    taskId?: string;
  }) => (
    <span
      data-testid="external-vcs-menu"
      data-repository-name={repositoryName}
      data-task-id={taskId}
    />
  ),
}));

import { CommitFileToolbar } from "./commit-file-toolbar";

beforeAll(() => {
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
    configurable: true,
  });
});

afterEach(() => {
  cleanup();
  mocks.isMobile = false;
  mocks.isFinePointer = true;
  localStorage.clear();
});

const baseProps = {
  filePath: "src/app.ts",
  diff: "-old\n+new",
  wordWrap: false,
  onToggleWordWrap: vi.fn(),
  expandUnchanged: false,
  onToggleExpandUnchanged: vi.fn(),
  onOpenFile: vi.fn(),
  repo: "frontend",
  sessionId: "session-1",
};

describe("CommitFileToolbar", () => {
  it("retains the desktop diff tools without review actions", () => {
    render(
      <TooltipProvider>
        <CommitFileToolbar {...baseProps} />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByLabelText("Copy diff"));
    fireEvent.click(screen.getByLabelText("Toggle word wrap"));
    fireEvent.click(screen.getByLabelText("Switch to split view"));
    fireEvent.click(screen.getByLabelText("Expand all lines"));
    fireEvent.click(screen.getByLabelText("Edit"));

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(baseProps.diff);
    expect(baseProps.onToggleWordWrap).toHaveBeenCalledOnce();
    expect(baseProps.onToggleExpandUnchanged).toHaveBeenCalledOnce();
    expect(baseProps.onOpenFile).toHaveBeenCalledWith(baseProps.filePath, baseProps.repo);
    expect(screen.queryByTestId("file-actions")).toBeNull();
    expect(screen.queryByLabelText("Revert changes")).toBeNull();
  });

  it("uses the touch-sized overflow trigger on mobile", () => {
    mocks.isMobile = true;
    mocks.isFinePointer = false;
    render(<CommitFileToolbar {...baseProps} />);

    const trigger = screen.getByRole("button", { name: "More actions for src/app.ts" });
    expect(trigger.className).toContain("size-11");
    fireEvent.click(trigger);
    expect(screen.getByRole("menu")).toBeTruthy();
  });

  it("keeps remote repository identity and omits local worktree actions on desktop", () => {
    render(
      <TooltipProvider>
        <CommitFileToolbar
          {...baseProps}
          taskId="task-1"
          repo="remote-widget"
          onOpenFile={undefined}
        />
      </TooltipProvider>,
    );

    expect(screen.queryByTestId("file-actions")).toBeNull();
    expect(screen.getByTestId("external-vcs-link").getAttribute("data-repository-name")).toBe(
      "remote-widget",
    );
    expect(screen.getByTestId("external-vcs-link").getAttribute("data-task-id")).toBe("task-1");
  });

  it("keeps remote repository identity and omits local worktree actions on mobile", () => {
    mocks.isMobile = true;
    render(
      <CommitFileToolbar
        {...baseProps}
        taskId="task-1"
        repo="remote-widget"
        onOpenFile={undefined}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "More actions for src/app.ts" }));
    expect(screen.queryByTestId("file-actions")).toBeNull();
    expect(screen.getByTestId("external-vcs-menu").getAttribute("data-repository-name")).toBe(
      "remote-widget",
    );
    expect(screen.getByTestId("external-vcs-menu").getAttribute("data-task-id")).toBe("task-1");
  });
});
