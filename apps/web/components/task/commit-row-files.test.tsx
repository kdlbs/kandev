import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileInfo } from "@/lib/state/store";
import type { CommitDetailTarget } from "./changes-diff-target";

const mocks = vi.hoisted(() => ({
  layout: "flat" as "flat" | "tree",
  isMobile: false,
  isFinePointer: true,
  useCommitDetail: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ userSettings: { changesPanelLayout: mocks.layout } }),
}));

vi.mock("@/hooks/domains/session/use-commit-detail", () => ({
  useCommitDetail: mocks.useCommitDetail,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.isMobile,
    isFinePointer: mocks.isFinePointer,
  }),
}));

vi.mock("@/hooks/use-copy-repository-path", () => ({
  useCopyRepositoryPath: () => vi.fn(),
}));

vi.mock("@/components/diff", () => ({
  FileDiffViewer: () => <div data-testid="inline-diff" />,
}));

import { CommitRowFiles } from "./commit-row-files";

afterEach(() => {
  cleanup();
  mocks.layout = "flat";
  mocks.isMobile = false;
  mocks.isFinePointer = true;
  mocks.useCommitDetail.mockReset();
});

const target: CommitDetailTarget = { source: "local", sha: "abc123456", repo: "frontend" };

function file(path: string, overrides: Partial<FileInfo> = {}): FileInfo {
  return {
    path,
    status: "modified",
    staged: false,
    additions: 1,
    deletions: 2,
    diff: "diff",
    ...overrides,
  };
}

describe("CommitRowFiles flat presentation", () => {
  it("renders read-only historical files and activates a selected path", () => {
    mocks.useCommitDetail.mockReturnValue({
      files: { "src/b.ts": file("src/b.ts"), "src/a.ts": file("src/a.ts") },
      loading: false,
      error: null,
      refetch: vi.fn(),
    });
    const onOpenFile = vi.fn();
    render(<CommitRowFiles target={target} onOpenFile={onOpenFile} />);

    const rows = [...document.querySelectorAll('[data-commit-file-entry="true"]')];
    expect(rows.map((node) => node.getAttribute("data-file-path"))).toEqual([
      "src/a.ts",
      "src/b.ts",
    ]);
    expect(rows[0].textContent).toContain("+1-2");
    expect(screen.getByTestId("commit-file-src-a.ts").className).toContain("py-0.5");
    expect(screen.queryByTitle("Stage file")).toBeNull();
    fireEvent.click(screen.getByTestId("commit-file-src-a.ts"));
    expect(onOpenFile).toHaveBeenCalledWith("src/a.ts");
  });

  it("opens historical images in the commit diff without worktree actions", () => {
    mocks.useCommitDetail.mockReturnValue({
      files: { "assets/icon.png": file("assets/icon.png", { status: "added" }) },
      loading: false,
      error: null,
      refetch: vi.fn(),
    });
    const onOpenFile = vi.fn();
    render(<CommitRowFiles target={target} onOpenFile={onOpenFile} />);

    fireEvent.click(screen.getByTestId("commit-file-assets-icon.png"));
    expect(onOpenFile).toHaveBeenCalledWith("assets/icon.png");
    expect(screen.queryByTestId("file-row-hover-actions")).toBeNull();
  });
});

describe("CommitRowFiles loading", () => {
  it("loads after the row is expanded and keeps a loading error retry path", () => {
    const refetch = vi.fn();
    mocks.useCommitDetail.mockReturnValue({
      files: null,
      loading: true,
      error: null,
      refetch,
    });
    const { rerender } = render(<CommitRowFiles target={target} />);
    expect(screen.getByText("Loading files...")).toBeTruthy();

    mocks.useCommitDetail.mockReturnValue({
      files: null,
      loading: false,
      error: "Unable to load files",
      refetch,
    });
    rerender(<CommitRowFiles target={target} />);
    expect(screen.getByRole("alert").textContent).toContain("Unable to load files");
    const retryButton = screen.getByRole("button", { name: "Retry" });
    expect(retryButton.className).not.toContain("min-h-11");

    mocks.isFinePointer = false;
    rerender(<CommitRowFiles target={target} />);
    expect(screen.getByRole("button", { name: "Retry" }).className).toContain("min-h-11");
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalledOnce();
  });
});

describe("CommitRowFiles tree presentation", () => {
  it("uses the saved tree layout with independently collapsible directories", () => {
    mocks.layout = "tree";
    mocks.useCommitDetail.mockReturnValue({
      files: { "src/a.ts": file("src/a.ts"), "src/b.ts": file("src/b.ts") },
      loading: false,
      error: null,
      refetch: vi.fn(),
    });
    render(<CommitRowFiles target={target} />);

    const directory = screen.getByTestId("commit-file-tree-dir-src");
    expect(directory.className).toContain("-mx-1");
    expect(document.querySelectorAll('[data-commit-file-entry="true"]')).toHaveLength(2);
    fireEvent.click(directory);
    expect(document.querySelectorAll('[data-commit-file-entry="true"]')).toHaveLength(0);
  });

  it("preserves tree file statistics and rename metadata from the original detail payload", () => {
    const files = {
      "src/renamed.ts": file("src/renamed.ts"),
      "src/zero.ts": file("src/zero.ts", { additions: 0, deletions: 0 }),
      "src/missing.ts": file("src/missing.ts", {
        additions: undefined,
        deletions: undefined,
      }),
      "src/old-name.ts": file("src/old-name.ts", {
        status: "renamed",
        additions: 7,
        deletions: 3,
        old_path: "src/previous-name.ts",
      }),
    };
    mocks.useCommitDetail.mockReturnValue({
      files,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    const readPresentation = () =>
      Object.keys(files)
        .sort()
        .map((path) => {
          const button = screen.getByTestId(`commit-file-${path.replace(/[/\\]/g, "-")}`);
          return {
            stats: button.textContent?.match(/\+\d+-\d+/)?.[0],
            status: button.querySelector('[role="img"]')?.getAttribute("aria-label"),
          };
        });

    mocks.layout = "flat";
    render(<CommitRowFiles target={target} />);
    const flatPresentation = readPresentation();
    cleanup();

    mocks.layout = "tree";
    render(<CommitRowFiles target={target} />);
    expect(readPresentation()).toEqual(flatPresentation);

    const renamedButton = screen.getByTestId("commit-file-src-old-name.ts");
    expect(renamedButton.textContent).toContain("+7-3");
    expect(screen.getByRole("img", { name: "Moved from src/previous-name.ts" })).toBeTruthy();
    expect(screen.getByTestId("commit-file-src-missing.ts").textContent).not.toContain(
      "Unavailable",
    );
  });
});

describe("CommitRowFiles mobile presentation", () => {
  it("keeps the distinguishing filename above its directory on a phone", () => {
    mocks.isMobile = true;
    mocks.isFinePointer = false;
    mocks.useCommitDetail.mockReturnValue({
      files: { "src/deeply/nested/navigation.ts": file("src/deeply/nested/navigation.ts") },
      loading: false,
      error: null,
      refetch: vi.fn(),
    });
    render(<CommitRowFiles target={target} />);

    const fileButton = screen.getByTestId("commit-file-src-deeply-nested-navigation.ts");
    expect(fileButton.textContent).toContain("navigation.ts");
    expect(fileButton.textContent).toContain("src/deeply/nested");
    expect(fileButton.getAttribute("title")).toBe("src/deeply/nested/navigation.ts");
  });
});
