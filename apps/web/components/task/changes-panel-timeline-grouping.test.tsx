import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import type { ComponentProps } from "react";

vi.mock("./commit-row", () => ({
  CommitRow: ({ commit, isLatest }: { commit: { commit_sha: string }; isLatest: boolean }) => (
    <li data-testid="commit-row" data-sha={commit.commit_sha} data-is-latest={String(isLatest)}>
      {commit.commit_sha}
    </li>
  ),
}));

import { CommitsSection, PRFilesSection } from "./changes-panel-timeline";

const COMMIT_ROW_TID = "commit-row";
const COMMITS_SECTION_TOGGLE_TID = "commits-section-collapse-toggle";
const ARIA_EXPANDED = "aria-expanded";

afterEach(cleanup);

type CommitProps = ComponentProps<typeof CommitsSection>;

function commit(sha: string, message: string, repo?: string): CommitProps["commits"][number] {
  return {
    commit_sha: sha,
    commit_message: message,
    insertions: 1,
    deletions: 0,
    pushed: false,
    statsAvailable: true,
    detailTarget: {
      source: "local",
      sha,
      ...(repo ? { repo } : {}),
    },
    repository_name: repo,
  };
}

describe("CommitsSection", () => {
  it("renders the commits section header collapsed by default", () => {
    render(<CommitsSection commits={[commit("abc123", "first")]} />);
    // Commits section is collapsed by default; the toggle reflects that and
    // the commit rows are not in the DOM until the user expands the section.
    const toggle = screen.getByTestId(COMMITS_SECTION_TOGGLE_TID);
    expect(toggle.getAttribute(ARIA_EXPANDED)).toBe("false");
    expect(screen.queryByTestId(COMMIT_ROW_TID)).toBeNull();

    fireEvent.click(toggle);
    expect(toggle.getAttribute(ARIA_EXPANDED)).toBe("true");
    expect(screen.getByTestId(COMMIT_ROW_TID)).toBeTruthy();
  });

  it("groups commits per repo when 2+ repos are present", () => {
    render(
      <CommitsSection
        commits={[
          commit("c1", "frontend change", "frontend"),
          commit("c2", "backend change", "backend"),
          commit("c3", "another frontend", "frontend"),
        ]}
      />,
    );
    fireEvent.click(screen.getByTestId(COMMITS_SECTION_TOGGLE_TID));
    const headers = screen.getAllByTestId("commits-repo-header");
    expect(headers).toHaveLength(2);
    expect(headers[0].textContent).toContain("frontend");
    expect(headers[0].textContent).toContain("2");
    expect(headers[1].textContent).toContain("backend");
    expect(headers[1].textContent).toContain("1");
    expect(screen.getAllByTestId(COMMIT_ROW_TID)).toHaveLength(3);
  });

  it("renders commits flat (no commits-repo header) for single-repo workspaces", () => {
    // Single-repo: drop the per-repo "REPOSITORY" sub-header. Push / PR
    // buttons live in the section header instead.
    render(
      <CommitsSection
        commits={[commit("c1", "msg"), commit("c2", "msg")]}
        onRepoPush={() => undefined}
      />,
    );
    fireEvent.click(screen.getByTestId(COMMITS_SECTION_TOGGLE_TID));
    expect(screen.queryAllByTestId("commits-repo-header")).toHaveLength(0);
    expect(screen.getAllByTestId(COMMIT_ROW_TID)).toHaveLength(2);
  });
});

describe("CommitsSection actions", () => {
  it("does not show Push when commits are already on the tracked remote", () => {
    render(
      <CommitsSection
        commits={[commit("c1", "already pushed"), commit("c2", "also pushed")]}
        onRepoPush={() => undefined}
        perRepoStatus={[{ repository_name: "", ahead: 0 }]}
      />,
    );

    expect(screen.queryByTestId("commits-repo-push")).toBeNull();

    fireEvent.click(screen.getByTestId(COMMITS_SECTION_TOGGLE_TID));
    expect(screen.getAllByTestId(COMMIT_ROW_TID)).toHaveLength(2);
    expect(screen.queryByTestId("commits-repo-push")).toBeNull();
  });

  it("labels Push with the actual ahead count instead of the commit list count", () => {
    render(
      <CommitsSection
        commits={[commit("c1", "newest"), commit("c2", "middle"), commit("c3", "oldest")]}
        onRepoPush={() => undefined}
        perRepoStatus={[{ repository_name: "", ahead: 1 }]}
      />,
    );

    const push = screen.getByTestId("commits-repo-push");
    expect(push.textContent).toContain("Push");
    expect(push.textContent).toContain("1");
    expect(push.textContent).not.toContain("3");
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
        onRevertCommit={() => undefined}
      />,
    );
    fireEvent.click(screen.getByTestId(COMMITS_SECTION_TOGGLE_TID));
    const rows = screen.getAllByTestId(COMMIT_ROW_TID);
    const latestByShas = rows
      .filter((r) => r.getAttribute("data-is-latest") === "true")
      .map((r) => r.getAttribute("data-sha"));
    expect(latestByShas.sort()).toEqual(["backend-only", "frontend-new"]);
  });
});

describe("section auto-expand (defaultCollapsed prop)", () => {
  const PR_TOGGLE_TID = "pr-changes-section-collapse-toggle";

  function prFile(path: string) {
    return { path, status: "modified" as const, plus: 1, minus: 0, oldPath: undefined };
  }

  it("PR Changes is collapsed by default (no prop)", () => {
    render(<PRFilesSection files={[prFile("a.ts")]} onOpenDiff={vi.fn()} />);
    expect(screen.getByTestId(PR_TOGGLE_TID).getAttribute(ARIA_EXPANDED)).toBe("false");
    expect(screen.queryByTestId("pr-files-list")).toBeNull();
  });

  it("PR Changes is expanded when defaultCollapsed={false}", () => {
    render(
      <PRFilesSection files={[prFile("a.ts")]} onOpenDiff={vi.fn()} defaultCollapsed={false} />,
    );
    expect(screen.getByTestId(PR_TOGGLE_TID).getAttribute(ARIA_EXPANDED)).toBe("true");
    expect(screen.getByTestId("pr-files-list")).toBeTruthy();
  });

  it("Commits is expanded when defaultCollapsed={false}", () => {
    render(
      <CommitsSection
        commits={[
          {
            commit_sha: "abc123",
            commit_message: "first",
            insertions: 1,
            deletions: 0,
            pushed: false,
            statsAvailable: true,
            detailTarget: { source: "local", sha: "abc123" },
            repository_name: undefined,
          },
        ]}
        defaultCollapsed={false}
      />,
    );
    const toggle = screen.getByTestId(COMMITS_SECTION_TOGGLE_TID);
    expect(toggle.getAttribute(ARIA_EXPANDED)).toBe("true");
    expect(screen.getByTestId(COMMIT_ROW_TID)).toBeTruthy();
  });

  it("keeps a comparison-targeted disclosure in keyboard tab order", () => {
    render(
      <>
        <button type="button" data-testid="before-disclosure">
          Before
        </button>
        <CommitsSection commits={[commit("abc123", "first")]} defaultCollapsed focusOnExpand />
      </>,
    );

    const toggle = screen.getByTestId(COMMITS_SECTION_TOGGLE_TID);
    screen.getByTestId("before-disclosure").focus();
    expect(toggle.tabIndex).toBe(0);
    toggle.focus();
    expect(document.activeElement).toBe(toggle);

    fireEvent.click(toggle);
    expect(toggle.getAttribute(ARIA_EXPANDED)).toBe("true");
  });
});
