import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { createRef, type ReactNode } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ReviewFile } from "./types";

// FileDiffViewer is heavy and irrelevant to the grouping test — stub it.
vi.mock("@/components/diff", () => ({
  FileDiffViewer: ({ filePath }: { filePath: string }) => (
    <div data-testid="diff-stub">{filePath}</div>
  ),
  DiffErrorBoundary: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/editors/file-actions-dropdown", () => ({
  FileActionsDropdown: () => null,
}));

// External link resolution is unrelated to grouping and requires a fully
// hydrated repository store, which this focused test intentionally omits.
vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
}));

vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => null }));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: vi.fn(),
  updateFileContent: vi.fn(),
}));
vi.mock("@/hooks/use-global-view-mode", () => ({
  useGlobalViewMode: () => ["unified", vi.fn()],
}));
vi.mock("@/hooks/domains/comments/use-run-comment", () => ({
  useRunComment: () => ({ runComment: vi.fn() }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

import { ReviewDiffList } from "./review-diff-list";

afterEach(cleanup);

function file(path: string, repo?: string): ReviewFile {
  return {
    path,
    diff: "@@ -1 +1 @@\n-a\n+b\n",
    status: "modified",
    additions: 1,
    deletions: 1,
    staged: false,
    source: "uncommitted",
    repository_name: repo,
  };
}

const baseProps = {
  reviewedFiles: new Set<string>(),
  staleFiles: new Set<string>(),
  sessionId: "sess",
  autoMarkOnScroll: false,
  wordWrap: false,
  selectedFile: null,
  onToggleReviewed: () => undefined,
  onDiscard: () => undefined,
  fileRefs: new Map<string, React.RefObject<HTMLDivElement | null>>(),
};

// FileDiffSection's toolbar uses Radix Tooltip which needs a provider.
function withTooltips(node: ReactNode) {
  return <TooltipProvider>{node}</TooltipProvider>;
}

function renderSingleFile(reviewFile: ReviewFile, selected = true) {
  const refs = new Map([[reviewFile.path, createRef<HTMLDivElement>()]]);
  render(
    withTooltips(
      <ReviewDiffList
        {...baseProps}
        files={[reviewFile]}
        selectedFile={selected ? reviewFile.path : null}
        fileRefs={refs}
      />,
    ),
  );
}

describe("ReviewDiffList — multi-repo grouping", () => {
  it("renders no repo header when all files lack a repository_name", () => {
    const refs = new Map([["a.ts", createRef<HTMLDivElement>()]]);
    render(withTooltips(<ReviewDiffList {...baseProps} files={[file("a.ts")]} fileRefs={refs} />));
    expect(screen.queryAllByTestId("changes-repo-header")).toHaveLength(0);
  });

  it("renders one header per repo when 2+ repos are present", () => {
    const files: ReviewFile[] = [
      file("src/app.tsx", "frontend"),
      file("src/api.ts", "frontend"),
      file("handlers/task.go", "backend"),
    ];
    const refs = new Map(files.map((f) => [f.path, createRef<HTMLDivElement>()]));
    render(withTooltips(<ReviewDiffList {...baseProps} files={files} fileRefs={refs} />));

    const headers = screen.getAllByTestId("changes-repo-header");
    expect(headers).toHaveLength(2);
    expect(headers[0].textContent).toContain("frontend");
    expect(headers[0].textContent).toContain("2 files");
    expect(headers[1].textContent).toContain("backend");
    expect(headers[1].textContent).toContain("1 file");
  });

  it("shows a header for a single named repo too (so user sees the label)", () => {
    const refs = new Map([["a.ts", createRef<HTMLDivElement>()]]);
    render(
      withTooltips(
        <ReviewDiffList {...baseProps} files={[file("a.ts", "only-repo")]} fileRefs={refs} />,
      ),
    );
    expect(screen.getByTestId("changes-repo-header").textContent).toContain("only-repo");
  });

  it("groups files into per-repo sections with the right counts", () => {
    const files: ReviewFile[] = [file("a.ts", "x"), file("b.ts", "y"), file("c.ts", "x")];
    const refs = new Map(files.map((f) => [f.path, createRef<HTMLDivElement>()]));
    render(withTooltips(<ReviewDiffList {...baseProps} files={files} fileRefs={refs} />));
    const groups = screen.getAllByTestId("changes-repo-group");
    expect(groups).toHaveLength(2);
    // The diff body itself is lazy-loaded via IntersectionObserver (which
    // doesn't fire in happy-dom), so verify grouping by counting file path
    // labels in each group's header rather than the inner diff body.
    const xGroup = groups.find((g) => g.getAttribute("data-repository-name") === "x");
    const yGroup = groups.find((g) => g.getAttribute("data-repository-name") === "y");
    // FileDiffHeader always renders the file path in a span; count those.
    expect(xGroup?.textContent).toContain("a.ts");
    expect(xGroup?.textContent).toContain("c.ts");
    expect(xGroup?.textContent).not.toContain("b.ts");
    expect(yGroup?.textContent).toContain("b.ts");
    expect(yGroup?.textContent).not.toContain("a.ts");
  });
});

describe("ReviewDiffList — file status rendering", () => {
  it("shows moved status in the mobile header and honest copy for a patchless rename", () => {
    const movedFile = {
      ...file("new-name.ts"),
      diff: "",
      status: "renamed",
      old_path: "old-name.ts",
    } as ReviewFile;
    renderSingleFile(movedFile);

    const marker = screen.getByRole("img", { name: "Moved from old-name.ts" });
    expect(marker.className).toContain("sm:hidden");
    expect(screen.getByText("Moved from old-name.ts; no textual changes")).toBeTruthy();
  });

  it("treats a nonempty 100%-rename metadata diff as patchless", () => {
    const movedFile = {
      ...file("new-name.ts"),
      diff: [
        "diff --git a/old-name.ts b/new-name.ts",
        "similarity index 100%",
        "rename from old-name.ts",
        "rename to new-name.ts",
      ].join("\n"),
      status: "renamed",
      old_path: "old-name.ts",
    } as ReviewFile;
    renderSingleFile(movedFile);

    expect(screen.getByText("Moved from old-name.ts; no textual changes")).toBeTruthy();
    expect(screen.queryByTestId("diff-stub")).toBeNull();
  });

  it("treats a synthetic added-file hunk for a zero-stat rename as patchless", () => {
    const movedFile = {
      ...file("new-name.ts"),
      diff: [
        "diff --git a/new-name.ts b/new-name.ts",
        "new file mode 100644",
        "--- /dev/null",
        "+++ b/new-name.ts",
        "@@ -0,0 +1,2 @@",
        "+first line",
        "+second line",
      ].join("\n"),
      status: "renamed",
      additions: 0,
      deletions: 0,
      old_path: "old-name.ts",
    } as ReviewFile;
    renderSingleFile(movedFile);

    expect(screen.getByText("Moved from old-name.ts; no textual changes")).toBeTruthy();
    expect(screen.queryByTestId("diff-stub")).toBeNull();
  });

  it("keeps the loading state for a nonempty diff deferred by lazy rendering", () => {
    const deferredFile = file("deferred.ts");
    renderSingleFile(deferredFile, false);

    expect(screen.getByText("Loading diff...")).toBeTruthy();
    expect(screen.queryByText("No textual diff available")).toBeNull();
  });
});
