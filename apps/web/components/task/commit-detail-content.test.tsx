import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileInfo } from "@/lib/state/store";

const mocks = vi.hoisted(() => ({
  provider: "pierre-diffs" as "pierre-diffs" | "monaco",
  indexEntryTestId: "commit-file-index-entry",
  filePath: "src/app.ts",
  expandUnchangedAttribute: "data-expand-unchanged",
  toolbarTestId: "commit-file-toolbar",
  viewerTestId: "file-diff-viewer",
  foldUnchanged: true,
  setFoldUnchanged: vi.fn(),
  isMobile: false,
  isFinePointer: true,
}));

const DEFAULT_PROVIDER = mocks.provider;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ tasks: { activeTaskId: "task-1", activeSessionId: "session-1" } }),
}));

vi.mock("@/hooks/domains/kanban/use-task-repositories", () => ({
  useTaskRepositories: () => [{ name: "frontend" }, { name: "backend" }],
}));

vi.mock("@/hooks/use-editor-resolver", () => ({
  useEditorProvider: () => mocks.provider,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.isMobile,
    isFinePointer: mocks.isFinePointer,
  }),
}));

vi.mock("@/components/editors/monaco/use-global-folding", () => ({
  useGlobalFolding: () => [mocks.foldUnchanged, mocks.setFoldUnchanged],
}));

vi.mock("@/components/diff", () => ({
  FileDiffViewer: ({
    filePath,
    expandUnchanged,
    onToggleExpandUnchanged,
    sessionId,
  }: {
    filePath: string;
    expandUnchanged?: boolean;
    onToggleExpandUnchanged?: () => void;
    sessionId?: string;
  }) => (
    <div
      data-testid="file-diff-viewer"
      data-expand-unchanged={String(expandUnchanged)}
      data-session-id={sessionId ?? ""}
    >
      {filePath}
      <button
        type="button"
        data-testid="viewer-expansion-toggle"
        onClick={onToggleExpandUnchanged}
      />
    </div>
  ),
}));

vi.mock("./commit-file-toolbar", () => ({
  CommitFileToolbar: ({
    filePath,
    expandUnchanged,
    onToggleExpandUnchanged,
    sessionId,
    repo,
  }: {
    filePath: string;
    expandUnchanged?: boolean;
    onToggleExpandUnchanged?: () => void;
    sessionId?: string | null;
    repo?: string;
  }) => (
    <button
      type="button"
      data-testid="commit-file-toolbar"
      data-expand-unchanged={String(expandUnchanged)}
      data-has-expansion={String(Boolean(onToggleExpandUnchanged))}
      data-session-id={sessionId ?? ""}
      data-repository-name={repo ?? ""}
      onClick={onToggleExpandUnchanged}
    >
      {filePath}
    </button>
  ),
}));

import { CommitDetailContent } from "./commit-detail-content";

afterEach(() => {
  cleanup();
  mocks.provider = DEFAULT_PROVIDER;
  mocks.foldUnchanged = true;
  mocks.isMobile = false;
  mocks.isFinePointer = true;
  mocks.setFoldUnchanged.mockReset();
  vi.restoreAllMocks();
});

function file(path: string, overrides: Partial<FileInfo> = {}): FileInfo {
  return {
    path,
    status: "modified",
    staged: false,
    additions: 0,
    deletions: 0,
    diff: "@@ -1 +1 @@\n-old\n+new",
    ...overrides,
  };
}

describe("CommitDetailContent file index", () => {
  it("renders a sorted flat index with zero and unavailable statistics", () => {
    render(
      <CommitDetailContent
        target={{
          source: "local",
          sha: "abc123456",
          repo: "frontend",
        }}
        fileEntries={[
          ["z.ts", file("z.ts")],
          ["a.ts", file("a.ts", { additions: undefined, deletions: 2, diff: "" })],
        ]}
        commit={{
          authorName: "Developer",
          commitMessage: "Update files",
          committedAt: "2026-09-17T10:00:00Z",
        }}
      />,
    );

    const entries = screen.getAllByTestId(mocks.indexEntryTestId);
    expect(entries.map((entry) => entry.getAttribute("data-file-path"))).toEqual(["a.ts", "z.ts"]);
    expect(screen.getByTestId("commit-repository").textContent).toContain("frontend");
    expect(screen.getByTestId("commit-file-index-stats-a.ts").textContent).toContain("Unavailable");
    expect(screen.getByTestId("commit-file-index-stats-z.ts").textContent).toContain("+0");
    expect(screen.getByTestId("commit-file-index-stats-z.ts").textContent).toContain("-0");
  });

  it("shows an unavailable repository label when multi-repository identity is missing", () => {
    render(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
        commit={{
          authorName: "Developer",
          commitMessage: "Update files",
          committedAt: "2026-09-17T10:00:00Z",
        }}
      />,
    );

    expect(screen.getByTestId("commit-repository").textContent).toContain("Unavailable");
  });

  it("activates a file from the index and preserves the independent index toggle", () => {
    render(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
      />,
    );

    const indexToggle = screen.getByTestId("commit-file-index-toggle");
    expect(indexToggle.getAttribute("aria-expanded")).toBe(String(true));
    const indexListId = indexToggle.getAttribute("aria-controls");
    expect(indexListId).toBeTruthy();
    expect(document.getElementById(indexListId!)).toBeTruthy();

    fireEvent.click(screen.getByTestId(mocks.indexEntryTestId));
    const fileToggle = screen.getByRole("button", { name: `Collapse ${mocks.filePath}` });
    expect(fileToggle.getAttribute("aria-expanded")).toBe(String(true));

    fireEvent.click(indexToggle);
    expect(indexToggle.getAttribute("aria-expanded")).toBe(String(false));
    expect(document.getElementById(indexListId!)).toBeTruthy();
    const indexList = document.getElementById(indexListId!);
    expect(indexList?.hidden).toBe(true);
    expect(indexList?.querySelector(`[data-testid="${mocks.indexEntryTestId}"]`)).toBeTruthy();
    expect(screen.getByRole("button", { name: `Collapse ${mocks.filePath}` })).toBeTruthy();
  });
});

describe("CommitDetailContent desktop layout", () => {
  it("keeps the desktop file identity and toolbar in one header row", () => {
    render(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
      />,
    );

    const identity = screen.getByTestId("collapsible-file-identity");
    const header = identity.parentElement;
    expect(header?.className).toContain("md:flex");
    expect(header?.className).toContain("md:items-center");
    expect(header?.querySelector('[data-testid="commit-file-toolbar"]')).toBeTruthy();
  });
});

describe("CommitDetailContent renderer behavior", () => {
  it("uses local expansion state for Pierre without writing the Monaco preference", () => {
    mocks.provider = DEFAULT_PROVIDER;
    render(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
      />,
    );

    const toolbar = screen.getByTestId(mocks.toolbarTestId);
    expect(toolbar.getAttribute(mocks.expandUnchangedAttribute)).toBe(String(false));
    expect(
      screen.getByTestId(mocks.viewerTestId).getAttribute(mocks.expandUnchangedAttribute),
    ).toBe(String(false));
    fireEvent.click(toolbar);
    expect(toolbar.getAttribute(mocks.expandUnchangedAttribute)).toBe(String(true));
    expect(
      screen.getByTestId(mocks.viewerTestId).getAttribute(mocks.expandUnchangedAttribute),
    ).toBe(String(true));
    expect(mocks.setFoldUnchanged).not.toHaveBeenCalled();
  });

  it("keeps the mobile index filename-first and touch-sized with a complete accessible path", () => {
    mocks.isMobile = true;
    mocks.isFinePointer = false;
    const path = "src/deeply/nested/navigation.ts";
    render(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[path, file(path, { additions: 7, deletions: 3 })]]}
      />,
    );

    const entry = screen.getByTestId(mocks.indexEntryTestId);
    expect(entry.textContent).toContain("navigation.ts");
    expect(entry.textContent).toContain("src/deeply/nested");
    expect(entry.getAttribute("aria-label")).toBe(path);
    expect(screen.getByTestId("commit-file-index-toggle").className).toContain("min-h-11");
    expect(
      screen.getByRole("button", { name: "Collapse src/deeply/nested/navigation.ts" }).className,
    ).toContain("min-h-11");
  });

  it("does not expose Pierre expansion controls for remote commit diffs", () => {
    mocks.provider = DEFAULT_PROVIDER;
    render(
      <CommitDetailContent
        target={{
          source: "github",
          sha: "abc123456",
          workspaceId: "workspace-1",
          owner: "acme",
          repo: "widget",
          repositoryName: "linked-widget",
        }}
        sessionId="active-local-session"
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
      />,
    );

    expect(screen.getByTestId(mocks.toolbarTestId).getAttribute("data-has-expansion")).toBe(
      String(false),
    );
    expect(screen.getByTestId(mocks.toolbarTestId).getAttribute("data-session-id")).toBe("");
    expect(screen.getByTestId(mocks.viewerTestId).getAttribute("data-session-id")).toBe("");
    expect(screen.getByTestId("commit-repository").textContent).toContain("acme/widget");
    expect(screen.getByTestId(mocks.toolbarTestId).getAttribute("data-repository-name")).toBe(
      "linked-widget",
    );
  });

  it("derives Monaco expansion from fold state and inverts the global setter", () => {
    mocks.provider = "monaco";
    mocks.foldUnchanged = true;
    const { rerender } = render(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
      />,
    );

    const toolbar = screen.getByTestId(mocks.toolbarTestId);
    expect(toolbar.getAttribute(mocks.expandUnchangedAttribute)).toBe(String(false));
    fireEvent.click(toolbar);
    expect(mocks.setFoldUnchanged).toHaveBeenCalledWith(false);

    mocks.foldUnchanged = false;
    rerender(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
      />,
    );
    expect(
      screen.getByTestId(mocks.toolbarTestId).getAttribute(mocks.expandUnchangedAttribute),
    ).toBe(String(true));
    expect(
      screen.getByTestId(mocks.viewerTestId).getAttribute(mocks.expandUnchangedAttribute),
    ).toBe(String(true));
  });
});

describe("CommitDetailContent missing navigation", () => {
  it("ignores a navigation request for a path not in the file list", () => {
    const scrollIntoView = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => undefined);
    const focus = vi.spyOn(HTMLButtonElement.prototype, "focus");

    render(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
        fileNavigation={{ path: "does-not-exist.ts", token: 1 }}
      />,
    );

    expect(scrollIntoView).not.toHaveBeenCalled();
    expect(focus).not.toHaveBeenCalled();
  });
});

describe("CommitDetailContent navigation", () => {
  it("consumes a navigation request after successful focus and scroll", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callback(0);
      return 1;
    });
    vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => undefined);
    const scrollIntoView = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => undefined);
    const focus = vi.spyOn(HTMLButtonElement.prototype, "focus");
    const element = (
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
        fileNavigation={{ path: mocks.filePath, token: 1 }}
      />
    );
    const { rerender } = render(element);
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
    expect(focus).toHaveBeenCalledTimes(1);

    rerender(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
        fileNavigation={{ path: mocks.filePath, token: 1 }}
      />,
    );
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
    expect(focus).toHaveBeenCalledTimes(1);

    rerender(
      <CommitDetailContent
        target={{ source: "local", sha: "abc123456" }}
        fileEntries={[[mocks.filePath, file(mocks.filePath)]]}
        fileNavigation={{ path: mocks.filePath, token: 2 }}
      />,
    );
    expect(scrollIntoView).toHaveBeenCalledTimes(2);
    expect(focus).toHaveBeenCalledTimes(2);
  });
});
