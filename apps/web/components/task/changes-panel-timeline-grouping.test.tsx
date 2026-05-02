import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, within } from "@testing-library/react";
import type { ComponentProps } from "react";

vi.mock("./changes-panel-file-row", () => ({
  FileRow: ({ file }: { file: { path: string } }) => <li data-testid="file-row">{file.path}</li>,
  BulkActionBar: () => null,
  DefaultActionButtons: () => null,
}));

vi.mock("./commit-row", () => ({
  CommitRow: ({ commit, isLatest }: { commit: { commit_sha: string }; isLatest: boolean }) => (
    <li data-testid="commit-row" data-sha={commit.commit_sha} data-is-latest={String(isLatest)}>
      {commit.commit_sha}
    </li>
  ),
}));

vi.mock("@/hooks/use-multi-select", () => ({
  useMultiSelect: () => ({
    selectedPaths: new Set<string>(),
    isSelected: () => false,
    handleClick: vi.fn(),
    clearSelection: vi.fn(),
  }),
}));

import { FileListSection, CommitsSection } from "./changes-panel-timeline";

const REPO_HEADER_TID = "changes-repo-header";
const COMMIT_ROW_TID = "commit-row";
const COMMITS_SECTION_TOGGLE_TID = "commits-section-collapse-toggle";

afterEach(cleanup);

type Props = ComponentProps<typeof FileListSection>;

const baseProps: Omit<Props, "files" | "variant" | "isLast" | "actionLabel" | "onAction"> = {
  pendingStageFiles: new Set(),
  onOpenDiff: vi.fn(),
  onEditFile: vi.fn(),
  onStage: vi.fn(),
  onUnstage: vi.fn(),
  onDiscard: vi.fn(),
};

function file(path: string, repo?: string): Props["files"][number] {
  return {
    path,
    status: "modified",
    staged: false,
    plus: 1,
    minus: 0,
    oldPath: undefined,
    repositoryName: repo,
  };
}

describe("FileListSection — multi-repo grouping", () => {
  it("renders a single repo header (uniform UI) for files without a repository_name", () => {
    // Per-repo headers always render now, including in single-repo / untagged
    // mode — the resolver fills in the workspace primary repo name (or a
    // neutral "Repository" fallback). This keeps the UX consistent across
    // single-repo and multi-repo workspaces and gives us per-repo Stage all /
    // Commit / Unstage all buttons in both modes.
    render(
      <FileListSection
        {...baseProps}
        variant="unstaged"
        isLast={false}
        actionLabel="Stage all"
        onAction={() => undefined}
        files={[file("a.ts"), file("b.ts")]}
      />,
    );
    expect(screen.getAllByTestId(REPO_HEADER_TID)).toHaveLength(1);
    expect(screen.getAllByTestId("file-row")).toHaveLength(2);
  });

  it("renders one header per repo when 2+ repos are present", () => {
    render(
      <FileListSection
        {...baseProps}
        variant="unstaged"
        isLast={false}
        actionLabel="Stage all"
        onAction={() => undefined}
        files={[
          file("src/app.tsx", "frontend"),
          file("src/api.ts", "frontend"),
          file("handlers/task.go", "backend"),
        ]}
      />,
    );
    const headers = screen.getAllByTestId(REPO_HEADER_TID);
    expect(headers).toHaveLength(2);
    expect(headers[0].textContent).toContain("frontend");
    expect(headers[0].textContent).toContain("2");
    expect(headers[1].textContent).toContain("backend");
    expect(headers[1].textContent).toContain("1");
  });

  it("shows a header for a single named repo too", () => {
    render(
      <FileListSection
        {...baseProps}
        variant="unstaged"
        isLast={false}
        actionLabel="Stage all"
        onAction={() => undefined}
        files={[file("a.ts", "only-repo")]}
      />,
    );
    expect(screen.getByTestId(REPO_HEADER_TID).textContent).toContain("only-repo");
  });

  it("collapses one repo independently when its header is clicked", () => {
    render(
      <FileListSection
        {...baseProps}
        variant="unstaged"
        isLast={false}
        actionLabel="Stage all"
        onAction={() => undefined}
        files={[
          file("src/app.tsx", "frontend"),
          file("src/api.ts", "frontend"),
          file("handlers/task.go", "backend"),
        ]}
      />,
    );

    expect(screen.getAllByTestId("file-row")).toHaveLength(3);

    const groups = screen.getAllByTestId("changes-repo-group");
    const frontendGroup = groups.find(
      (g) => g.getAttribute("data-repository-name") === "frontend",
    )!;
    const frontendHeader = within(frontendGroup).getByTestId(REPO_HEADER_TID);

    fireEvent.click(frontendHeader);

    // frontend's two rows hidden, backend's one row still visible
    expect(screen.getAllByTestId("file-row")).toHaveLength(1);
    expect(frontendHeader.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(frontendHeader);
    expect(screen.getAllByTestId("file-row")).toHaveLength(3);
    expect(frontendHeader.getAttribute("aria-expanded")).toBe("true");
  });
});

describe("CommitsSection", () => {
  type CommitProps = ComponentProps<typeof CommitsSection>;

  function commit(sha: string, message: string, repo?: string): CommitProps["commits"][number] {
    return {
      commit_sha: sha,
      commit_message: message,
      insertions: 1,
      deletions: 0,
      pushed: false,
      repository_name: repo,
    };
  }

  it("renders the commits section header with a collapse toggle", () => {
    render(<CommitsSection commits={[commit("abc123", "first")]} isLast />);
    // Section is expanded by default — file changes and commit history are
    // both first-class signals in the changes panel.
    expect(screen.getByTestId(COMMIT_ROW_TID)).toBeTruthy();
    const toggle = screen.getByTestId(COMMITS_SECTION_TOGGLE_TID);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
  });

  it("groups commits per repo when 2+ repos are present", () => {
    render(
      <CommitsSection
        commits={[
          commit("c1", "frontend change", "frontend"),
          commit("c2", "backend change", "backend"),
          commit("c3", "another frontend", "frontend"),
        ]}
        isLast
      />,
    );
    const headers = screen.getAllByTestId("commits-repo-header");
    expect(headers).toHaveLength(2);
    expect(headers[0].textContent).toContain("frontend");
    expect(headers[0].textContent).toContain("2");
    expect(headers[1].textContent).toContain("backend");
    expect(headers[1].textContent).toContain("1");
    expect(screen.getAllByTestId(COMMIT_ROW_TID)).toHaveLength(3);
  });

  it("renders a single commits-repo header (uniform UI) for untagged commits", () => {
    // The Commits section, like the file lists, always groups by repo now.
    // For untagged or single-repo commits we get one group with the empty
    // repository_name; the header still renders so the per-repo Push / PR
    // buttons live in a consistent place across single-repo and multi-repo.
    render(<CommitsSection commits={[commit("c1", "msg"), commit("c2", "msg")]} isLast />);
    expect(screen.getAllByTestId("commits-repo-header")).toHaveLength(1);
    expect(screen.getAllByTestId(COMMIT_ROW_TID)).toHaveLength(2);
  });

  // Regression: previously isLatest was computed against the merged list, so
  // only ONE commit globally was marked latest. Each repo's newest unpushed
  // commit must be latest in its own group — otherwise revert/amend buttons
  // are absent on every repo except one, and clicking revert via context menu
  // hits the backend's "can only revert latest commit" gate.
  it("marks each repo's newest unpushed commit as latest (per-repo, not global)", () => {
    render(
      <CommitsSection
        commits={[
          // Insertion order is newest-first within each repo.
          commit("frontend-new", "frontend latest", "frontend"),
          commit("frontend-old", "frontend older", "frontend"),
          commit("backend-only", "backend latest", "backend"),
        ]}
        isLast
        onRevertCommit={() => undefined}
      />,
    );
    const rows = screen.getAllByTestId(COMMIT_ROW_TID);
    const latestByShas = rows
      .filter((r) => r.getAttribute("data-is-latest") === "true")
      .map((r) => r.getAttribute("data-sha"));
    expect(latestByShas.sort()).toEqual(["backend-only", "frontend-new"]);
  });
});
