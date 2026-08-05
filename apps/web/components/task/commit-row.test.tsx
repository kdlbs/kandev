import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CommitRow, type CommitItem } from "./commit-row";

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
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

afterEach(cleanup);

function remoteCommit(): CommitItem {
  return {
    commit_sha: "remote123456",
    commit_message: "Remote contribution",
    insertions: 0,
    deletions: 0,
    pushed: true,
    statsAvailable: false,
    detailTarget: {
      source: "github",
      sha: "remote123456",
      workspaceId: "workspace-1",
      owner: "acme",
      repo: "widget",
    },
  };
}

describe("CommitRow", () => {
  it("hides unknown statistics and local mutation actions for remote commits", () => {
    render(
      <CommitRow
        commit={remoteCommit()}
        isLatest
        onAmendCommit={vi.fn()}
        onRevertCommit={vi.fn()}
        onResetToCommit={vi.fn()}
      />,
    );

    expect(screen.getByText("Remote contribution")).toBeTruthy();
    expect(screen.queryByText("+0")).toBeNull();
    expect(screen.queryByText("-0")).toBeNull();
    expect(screen.queryByText("Amend message")).toBeNull();
    expect(screen.queryByText("Revert commit")).toBeNull();
    expect(screen.queryByText("Reset to this commit")).toBeNull();
  });

  it("passes the full source target when a remote row is opened", () => {
    const onOpenCommitDetail = vi.fn();
    const commit = remoteCommit();
    render(<CommitRow commit={commit} isLatest onOpenCommitDetail={onOpenCommitDetail} />);

    fireEvent.click(screen.getByTestId("commit-row-remote1"));
    expect(onOpenCommitDetail).toHaveBeenCalledWith(commit.detailTarget);
  });
});
