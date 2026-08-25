import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TaskLayout } from "./task-layout";

afterEach(cleanup);

// Every hop between the task page and the phone header is an optional prop, so
// a dropped one is not a type error. This asserts the hop TaskLayout owns: the
// repository label reaches the phone layout. `session-mobile-top-bar-repository`
// covers what the header does with it.
vi.mock("./mobile", () => ({
  SessionMobileLayout: ({ repositoryLabel }: { repositoryLabel?: string | null }) => (
    <div data-testid="mobile-layout" data-repository-label={repositoryLabel ?? ""} />
  ),
  SessionTabletLayout: () => <div data-testid="tablet-layout" />,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true, usesDesktopWorkbench: false }),
}));

vi.mock("./task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => ({ error: null, repositories: [] }),
}));

describe("TaskLayout repository label", () => {
  it("hands the repository label to the phone layout", () => {
    render(<TaskLayout workspaceId="ws-1" workflowId="wf-1" repositoryLabel="kdlbs/kandev" />);

    expect(screen.getByTestId("mobile-layout").getAttribute("data-repository-label")).toBe(
      "kdlbs/kandev",
    );
  });

  it("passes nothing along for a task with no repository", () => {
    render(<TaskLayout workspaceId="ws-1" workflowId="wf-1" />);

    expect(screen.getByTestId("mobile-layout").getAttribute("data-repository-label")).toBe("");
  });
});
