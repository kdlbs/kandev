import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CommitRow } from "./commit-row";
import { mergeCommits, separateCommitHistories } from "./changes-panel-helpers";
import { CommitsRepoGroup } from "./changes-panel-repo-groups";
import { ChangesTimelineHistoryRow } from "./changes-timeline-history-row";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));
vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ContextMenuItem: ({
    children,
    onSelect,
  }: {
    children: React.ReactNode;
    onSelect: () => void;
  }) => (
    <button type="button" onClick={onSelect}>
      {children}
    </button>
  ),
  ContextMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: () => null,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

afterEach(cleanup);

const REVERT = "Revert commit";

function local(parent: string) {
  return {
    commit_sha: "1234567890abcdef",
    commit_message: "Local commit",
    parent_sha: parent,
    repository_name: "service",
    insertions: 1,
    deletions: 0,
  };
}

describe("root commit revert availability", () => {
  // @covers AC-WORKSPACES-ROOT-REVERT-001.1
  it("omits both revert controls for a known root while retaining other actions", () => {
    const commit = mergeCommits([local("")], [])[0];
    render(
      <CommitRow
        commit={commit}
        isLatest
        onRevertCommit={vi.fn()}
        onAmendCommit={vi.fn()}
        onResetToCommit={vi.fn()}
      />,
    );
    expect(screen.queryAllByRole("button", { name: REVERT })).toHaveLength(0);
    expect(screen.getAllByRole("button", { name: /Amend/ }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: /Reset/ }).length).toBeGreaterThan(0);
  });

  // @covers AC-WORKSPACES-ROOT-REVERT-001.2
  it("dispatches the parented commit and its repository", () => {
    const revert = vi.fn();
    const commit = mergeCommits([local("parent")], [])[0];
    render(<CommitRow commit={commit} isLatest onRevertCommit={revert} />);
    const actions = screen.getAllByRole("button", { name: REVERT });
    expect(actions).toHaveLength(2);
    fireEvent.click(actions[0]);
    expect(revert).toHaveBeenCalledWith("1234567890abcdef", "service");
  });

  it("retains legacy unknown parent eligibility and excludes older commits", () => {
    const { parent_sha: _parent, ...legacy } = local("parent");
    const commit = mergeCommits([legacy], [])[0];
    const { rerender } = render(<CommitRow commit={commit} isLatest onRevertCommit={vi.fn()} />);
    expect(screen.getAllByRole("button", { name: REVERT })).toHaveLength(2);
    rerender(<CommitRow commit={commit} isLatest={false} onRevertCommit={vi.fn()} />);
    expect(screen.queryByRole("button", { name: REVERT })).toBeNull();
  });
});

describe("history source and action adapters", () => {
  it("applies root eligibility through both rendered history adapters", () => {
    const root = separateCommitHistories([local("")], []).localCommits[0];
    const revert = vi.fn();
    const { rerender } = render(
      <CommitsRepoGroup
        repositoryName="service"
        groupCommits={[root]}
        showHeader={false}
        onRevertCommit={revert}
        onResetToCommit={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: REVERT })).toBeNull();
    expect(screen.getAllByRole("button", { name: /Reset/ }).length).toBeGreaterThan(0);
    rerender(
      <ChangesTimelineHistoryRow
        row={{
          kind: "commit",
          key: "commit",
          sectionKey: "commits",
          groups: [],
          commit: root,
          target: root.detailTarget,
          targetKey: "target",
          expanded: false,
          isLatest: true,
          detail: { expanded: false, status: "idle", files: [] },
        }}
        actions={{
          onOpenDiff: vi.fn(),
          onRevertCommit: revert,
          onResetToCommit: vi.fn(),
          pushDisabled: false,
        }}
        onToggleSection={vi.fn()}
        onToggleRepository={vi.fn()}
        onToggleCommit={vi.fn()}
        onRetryCommit={vi.fn()}
        onToggleInlineDirectory={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: REVERT })).toBeNull();
    expect(screen.getAllByRole("button", { name: /Reset/ }).length).toBeGreaterThan(0);
  });
});

describe("history provenance and pushed restrictions", () => {
  it("keeps local ancestry in unified and separate history without enabling provider mutations", () => {
    const root = local("");
    const provider = {
      sha: root.commit_sha,
      message: "Published",
      additions: 0,
      deletions: 0,
      workspace_id: "workspace",
      owner: "owner",
      repo: "service",
      repository_name: "service",
    };
    const shared = mergeCommits([root], [provider])[0];
    expect(shared).toMatchObject({
      parent_sha: "",
      detailTarget: { source: "local" },
      pushed: true,
    });
    const separate = separateCommitHistories([root], [provider]);
    expect(separate.localCommits[0]).toMatchObject({
      parent_sha: "",
      detailTarget: { source: "local" },
    });
    render(
      <CommitRow
        commit={separate.providerCommits[0]}
        isLatest
        onRevertCommit={vi.fn()}
        onAmendCommit={vi.fn()}
        onResetToCommit={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: REVERT })).toBeNull();
    expect(screen.queryByRole("button", { name: /Amend|Reset/ })).toBeNull();
  });

  it("does not render an empty action menu for a revert-only root", () => {
    const commit = mergeCommits([local("")], [])[0];
    const { container } = render(<CommitRow commit={commit} isLatest onRevertCommit={vi.fn()} />);
    expect(screen.queryByRole("button", { name: REVERT })).toBeNull();
    expect(container.querySelector(".group-hover\\:flex")).toBeNull();
  });

  it("keeps pushed restrictions in both actual history adapters", () => {
    const commit = mergeCommits([{ ...local("parent"), pushed: true }], [])[0];
    const revert = vi.fn();
    const { rerender } = render(
      <CommitsRepoGroup
        repositoryName="service"
        groupCommits={[commit]}
        showHeader={false}
        onRevertCommit={revert}
        onResetToCommit={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: /Revert|Reset/ })).toBeNull();
    rerender(
      <ChangesTimelineHistoryRow
        row={{
          kind: "commit",
          key: "commit",
          sectionKey: "commits",
          groups: [],
          commit,
          target: commit.detailTarget,
          targetKey: "target",
          expanded: false,
          isLatest: true,
          detail: { expanded: false, status: "idle", files: [] },
        }}
        actions={{
          onOpenDiff: vi.fn(),
          onRevertCommit: revert,
          onResetToCommit: vi.fn(),
          pushDisabled: false,
        }}
        onToggleSection={vi.fn()}
        onToggleRepository={vi.fn()}
        onToggleCommit={vi.fn()}
        onRetryCommit={vi.fn()}
        onToggleInlineDirectory={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: /Revert|Reset/ })).toBeNull();
  });
});
