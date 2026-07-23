import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ isMobile: false }));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile }),
}));

import { DiffViewerToolbar } from "./diff-viewer-toolbar";

afterEach(() => {
  cleanup();
  mocks.isMobile = false;
});

describe("DiffViewerToolbar external file action", () => {
  it("forwards Monaco diff repository, revision, and rename context", () => {
    render(
      <TooltipProvider>
        <DiffViewerToolbar
          data={{
            filePath: "src/new-name.ts",
            oldContent: "old",
            newContent: "new",
            additions: 1,
            deletions: 1,
          }}
          sessionId="session-1"
          taskId="task-1"
          repositoryId="repo-1"
          repositoryName="frontend"
          status="renamed"
          previousPath="src/old-name.ts"
          publishedBranch="feature/external-links"
          baseBranch="main"
          foldUnchanged={false}
          setFoldUnchanged={vi.fn()}
          wordWrap={false}
          setWordWrap={vi.fn()}
          globalViewMode="split"
          setGlobalViewMode={vi.fn()}
          onCopyDiff={vi.fn()}
        />
      </TooltipProvider>,
    );

    const props = JSON.parse(
      screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}",
    );
    expect(props).toEqual({
      filePath: "src/new-name.ts",
      previousPath: "src/old-name.ts",
      status: "renamed",
      taskId: "task-1",
      sessionId: "session-1",
      repositoryId: "repo-1",
      repositoryName: "frontend",
      publishedBranch: "feature/external-links",
      baseBranch: "main",
      size: "xs",
    });
  });

  it("uses the 44px touch action at the mobile breakpoint", () => {
    mocks.isMobile = true;
    render(
      <TooltipProvider>
        <DiffViewerToolbar
          data={{
            filePath: "src/app.ts",
            oldContent: "old",
            newContent: "new",
            additions: 1,
            deletions: 1,
          }}
          sessionId="session-1"
          foldUnchanged={false}
          setFoldUnchanged={vi.fn()}
          wordWrap={false}
          setWordWrap={vi.fn()}
          globalViewMode="split"
          setGlobalViewMode={vi.fn()}
          onCopyDiff={vi.fn()}
        />
      </TooltipProvider>,
    );

    const props = JSON.parse(
      screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}",
    );
    expect(props.size).toBe("touch");
  });
});
