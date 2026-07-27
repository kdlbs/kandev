import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ isMobile: false }));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  ExternalVcsFileMenuItem: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-menu-item-props" data-props={JSON.stringify(props)} />
  ),
}));

vi.mock("@/components/editors/file-actions-dropdown", () => ({
  FileActionsDropdown: () => <span data-testid="file-actions-dropdown" />,
  FileActionsMenuItems: () => <span data-testid="file-actions-menu-items" />,
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
      baseBranch: "main",
      size: "xs",
    });
    expect(screen.getByTestId("file-actions-dropdown")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Copy diff" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /More actions for/ })).toBeNull();
  });

  it("replaces the mobile icon strip with one labelled actions menu", () => {
    mocks.isMobile = true;
    const onToggleExpandUnchanged = vi.fn();
    const onToggleWordWrap = vi.fn();
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
          onToggleExpandUnchanged={onToggleExpandUnchanged}
          onToggleWordWrap={onToggleWordWrap}
        />
      </TooltipProvider>,
    );

    expect(screen.queryByRole("button", { name: "Copy diff" })).toBeNull();
    expect(screen.queryByTestId("file-actions-dropdown")).toBeNull();

    const trigger = screen.getByRole("button", { name: "More actions for src/app.ts" });
    expect(trigger.className).toContain("size-11");
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);

    const menu = screen.getByTestId("review-file-actions-menu");
    expect(menu).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Copy diff" })).toBeTruthy();
    const expand = screen.getByRole("menuitemcheckbox", { name: "Expand unchanged lines" });
    const wrap = screen.getByRole("menuitemcheckbox", { name: "Wrap long lines" });
    expect(expand.getAttribute("aria-checked")).toBe("false");
    expect(wrap.getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByRole("menuitem", { name: /Switch to unified view/ })).toBeNull();
    expect(screen.getByTestId("external-vcs-file-menu-item-props")).toBeTruthy();
    expect(screen.getByTestId("file-actions-menu-items")).toBeTruthy();

    fireEvent.click(expand);
    expect(onToggleExpandUnchanged).toHaveBeenCalledOnce();
  });
});
