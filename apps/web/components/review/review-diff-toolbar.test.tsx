import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ isMobile: false }));
const clipboardMocks = vi.hoisted(() => ({ copyToClipboard: vi.fn() }));

vi.mock("@/lib/utils/copy-to-clipboard", () => clipboardMocks);

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  ExternalVcsFileMenuItem: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-menu-item-props" data-props={JSON.stringify(props)} />
  ),
}));

vi.mock("@/components/editors/file-actions-dropdown", () => ({
  FileActionsDropdown: (props: { includeCopyPath?: boolean }) => (
    <span
      data-testid="file-actions-dropdown"
      data-include-copy-path={String(props.includeCopyPath)}
    />
  ),
  FileActionsMenuItems: (props: { includeCopyPath?: boolean }) => (
    <span
      data-testid="file-actions-menu-items"
      data-include-copy-path={String(props.includeCopyPath)}
    />
  ),
}));

vi.mock("@/hooks/use-global-view-mode", () => ({
  useGlobalViewMode: () => ["split", vi.fn()],
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile }),
}));

import { FileDiffToolbar, type FileDiffToolbarProps } from "./review-diff-toolbar";

afterEach(() => {
  cleanup();
  clipboardMocks.copyToClipboard.mockReset();
  mocks.isMobile = false;
});

function externalLinkProps() {
  return JSON.parse(screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}");
}

describe("Markdown preview actions", () => {
  it("toggles an inline Markdown preview on desktop", () => {
    const onToggleMarkdownPreview = vi.fn();
    render(
      <TooltipProvider>
        <FileDiffToolbar
          diff="# README"
          filePath="README.md"
          sessionId="session-1"
          source="pr"
          wordWrap={false}
          expandUnchanged={false}
          onDiscard={vi.fn()}
          onToggleMarkdownPreview={onToggleMarkdownPreview}
          onToggleExpandUnchanged={vi.fn()}
          onToggleWordWrap={vi.fn()}
          repo="frontend"
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Preview markdown" }));

    expect(onToggleMarkdownPreview).toHaveBeenCalledOnce();
  });

  it("keeps Markdown preview reachable from the mobile actions menu", () => {
    mocks.isMobile = true;
    const onToggleMarkdownPreview = vi.fn();
    render(
      <TooltipProvider>
        <FileDiffToolbar
          diff="# README"
          filePath="README.md"
          sessionId="session-1"
          source="pr"
          wordWrap={false}
          expandUnchanged={false}
          onDiscard={vi.fn()}
          onToggleMarkdownPreview={onToggleMarkdownPreview}
          onToggleExpandUnchanged={vi.fn()}
          onToggleWordWrap={vi.fn()}
          repo="frontend"
        />
      </TooltipProvider>,
    );

    const trigger = screen.getByRole("button", { name: "More actions for README.md" });
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: "Preview markdown" }));

    expect(onToggleMarkdownPreview).toHaveBeenCalledOnce();
  });

  it("offers Show diff while a Markdown row is in preview mode", () => {
    const onToggleMarkdownPreview = vi.fn();
    const props: FileDiffToolbarProps & { markdownPreview: boolean } = {
      diff: "# README",
      filePath: "README.md",
      sessionId: "session-1",
      source: "pr",
      wordWrap: false,
      expandUnchanged: false,
      onDiscard: vi.fn(),
      onToggleMarkdownPreview,
      onToggleExpandUnchanged: vi.fn(),
      onToggleWordWrap: vi.fn(),
      markdownPreview: true,
    };
    render(
      <TooltipProvider>
        <FileDiffToolbar {...props} />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Show diff" }));

    expect(onToggleMarkdownPreview).toHaveBeenCalledOnce();
  });
});

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
    expect(screen.queryByRole("button", { name: "Copy diff" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Copy path" }));
    expect(clipboardMocks.copyToClipboard).toHaveBeenCalledWith("src/new-name.ts");
    expect(screen.getByTestId("file-actions-dropdown").dataset.includeCopyPath).toBe("false");
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
    expect(screen.queryByRole("menuitem", { name: "Copy diff" })).toBeNull();
    const copyPath = screen.getByRole("menuitem", { name: "Copy path" });
    expect(copyPath.className).toContain("min-h-11");
    expect(screen.getByTestId("file-actions-menu-items").dataset.includeCopyPath).toBe("false");
    const expand = screen.getByRole("menuitemcheckbox", { name: "Expand unchanged lines" });
    const wrap = screen.getByRole("menuitemcheckbox", { name: "Wrap long lines" });
    expect(expand.getAttribute("aria-checked")).toBe("false");
    expect(wrap.getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByRole("menuitem", { name: /Switch to unified view/ })).toBeNull();
    expect(screen.getByTestId("external-vcs-file-menu-item-props")).toBeTruthy();
    expect(screen.getByTestId("file-actions-menu-items")).toBeTruthy();

    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    expect(screen.queryByTestId("review-file-actions-menu")).toBeNull();

    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "Expand unchanged lines" }));
    expect(onToggleExpandUnchanged).toHaveBeenCalledOnce();

    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: "Copy path" }));
    expect(clipboardMocks.copyToClipboard).toHaveBeenCalledWith("src/app.ts");
  });
});
