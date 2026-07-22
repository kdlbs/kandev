import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ isMobile: false }));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
}));

vi.mock("@/components/editors/file-actions-dropdown", () => ({
  FileActionsDropdown: () => <span data-testid="file-actions-dropdown" />,
}));

vi.mock("@/hooks/use-global-view-mode", () => ({
  useGlobalViewMode: () => ["split", vi.fn()],
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile }),
}));

import { FileDiffToolbar } from "./review-diff-toolbar";

afterEach(() => {
  cleanup();
  mocks.isMobile = false;
});

function externalLinkProps() {
  return JSON.parse(screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}");
}

describe("FileDiffToolbar", () => {
  it("forwards exact review file and PR revision context to the external action", () => {
    render(
      <TooltipProvider>
        <FileDiffToolbar
          diff="@@ -1 +1 @@"
          filePath="src/new-name.ts"
          previousPath="src/old-name.ts"
          status="renamed"
          taskId="task-1"
          sessionId="session-1"
          repositoryId="repo-1"
          source="pr"
          publishedBranch="feature/review-link"
          publishedPullRequestNumber={42}
          baseBranch="main"
          wordWrap={false}
          expandUnchanged={false}
          onDiscard={vi.fn()}
          onToggleExpandUnchanged={vi.fn()}
          onToggleWordWrap={vi.fn()}
          repo="frontend"
        />
      </TooltipProvider>,
    );

    expect(externalLinkProps()).toEqual({
      filePath: "src/new-name.ts",
      previousPath: "src/old-name.ts",
      status: "renamed",
      taskId: "task-1",
      sessionId: "session-1",
      repositoryId: "repo-1",
      repositoryName: "frontend",
      publishedBranch: "feature/review-link",
      publishedPullRequestNumber: 42,
      baseBranch: "main",
      size: "xs",
    });
    expect(screen.getByTestId("file-actions-dropdown")).toBeTruthy();
  });

  it("uses the 44px touch action in the mobile diff drawer", () => {
    mocks.isMobile = true;
    render(
      <TooltipProvider>
        <FileDiffToolbar
          diff="@@ -1 +1 @@"
          filePath="src/app.ts"
          sessionId="session-1"
          source="uncommitted"
          wordWrap={false}
          expandUnchanged={false}
          onDiscard={vi.fn()}
          onToggleExpandUnchanged={vi.fn()}
          onToggleWordWrap={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(externalLinkProps().size).toBe("touch");
  });
});
