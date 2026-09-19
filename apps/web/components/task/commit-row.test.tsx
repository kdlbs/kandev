import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CommitRow, type CommitItem } from "./commit-row";

const mocks = vi.hoisted(() => ({
  isMobile: false,
  isFinePointer: true,
  files: null as Record<string, { path: string; status: "modified" }> | null,
}));

const COMMIT_TOGGLE_TEST_ID = "commit-toggle";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.isMobile,
    isFinePointer: mocks.isFinePointer,
  }),
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
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/hooks/domains/session/use-commit-detail", () => ({
  useCommitDetail: () => ({
    files: mocks.files,
    commit: null,
    loading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ userSettings: { changesPanelLayout: "flat" } }),
}));

afterEach(() => {
  cleanup();
  mocks.isMobile = false;
  mocks.isFinePointer = true;
  mocks.files = null;
});

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

describe("CommitRow file navigation", () => {
  it("allocates a new file-navigation token after the row remounts", () => {
    mocks.files = { "src/app.ts": { path: "src/app.ts", status: "modified" } };
    const onOpenCommitDetail = vi.fn();
    const commit: CommitItem = {
      ...remoteCommit(),
      commit_sha: "local123456",
      detailTarget: { source: "local", sha: "local123456" },
    };
    const { rerender } = render(
      <CommitRow commit={commit} isLatest onOpenCommitDetail={onOpenCommitDetail} />,
    );

    fireEvent.click(screen.getByTestId(COMMIT_TOGGLE_TEST_ID));
    fireEvent.click(screen.getByTestId("commit-file-src-app.ts"));
    rerender(<div />);
    rerender(<CommitRow commit={commit} isLatest onOpenCommitDetail={onOpenCommitDetail} />);
    fireEvent.click(screen.getByTestId(COMMIT_TOGGLE_TEST_ID));
    fireEvent.click(screen.getByTestId("commit-file-src-app.ts"));

    const requests = onOpenCommitDetail.mock.calls.map(([, request]) => request);
    expect(requests).toHaveLength(2);
    expect(requests[0].token).not.toBe(requests[1].token);
  });
});

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

  it("passes the full source target when the separate open action is used", () => {
    const onOpenCommitDetail = vi.fn();
    const commit = remoteCommit();
    render(<CommitRow commit={commit} isLatest onOpenCommitDetail={onOpenCommitDetail} />);

    fireEvent.click(screen.getByTestId("commit-open-remote1"));
    expect(onOpenCommitDetail).toHaveBeenCalledWith(commit.detailTarget);
  });

  it("uses the main row button for inline expansion without opening a detail panel", () => {
    const onOpenCommitDetail = vi.fn();
    const commit = remoteCommit();
    render(<CommitRow commit={commit} isLatest onOpenCommitDetail={onOpenCommitDetail} />);

    const row = screen.getByTestId(COMMIT_TOGGLE_TEST_ID);
    expect(row.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(row);
    expect(row.getAttribute("aria-expanded")).toBe("true");
    expect(onOpenCommitDetail).not.toHaveBeenCalled();
  });

  it("announces diverged row provenance without showing an unpushed marker", () => {
    const localCheckoutCommit: CommitItem = {
      ...remoteCommit(),
      commit_sha: "local123456",
      commit_message: "Superseded checkout commit",
      pushed: false,
      presentation: "local_checkout",
      detailTarget: { source: "local", sha: "local123456" },
    };

    render(<CommitRow commit={localCheckoutCommit} isLatest />);

    expect(screen.getByTitle("Local checkout commit")).toBeTruthy();
    expect(screen.getByText("Local checkout commit")).toBeTruthy();
    expect(screen.queryByTitle("Local commit (not yet pushed)")).toBeNull();
  });

  it("exposes current PR provenance through a stable marker and violet styling", () => {
    render(<CommitRow commit={{ ...remoteCommit(), presentation: "current_pr" }} isLatest />);

    const marker = screen.getByTestId("commit-provenance");
    expect(marker.getAttribute("data-commit-provenance")).toBe("current_pr");
    expect(marker.getAttribute("title")).toBe("Current PR commit");
    expect(marker.querySelector("svg")?.getAttribute("class")).toContain("text-violet-500");
    expect(screen.getByText("Current PR commit")).toBeTruthy();
  });

  it("keeps phone commit identity above its statistics and sizes both controls for touch", () => {
    mocks.isMobile = true;
    mocks.isFinePointer = false;
    const commit: CommitItem = {
      ...remoteCommit(),
      commit_sha: "mobile123456",
      commit_message: "A commit with a long mobile title that must remain readable",
      insertions: 12,
      deletions: 4,
      statsAvailable: true,
    };
    render(<CommitRow commit={commit} isLatest />);

    const toggle = screen.getByTestId(COMMIT_TOGGLE_TEST_ID);
    const stats = screen.getByTestId("commit-stats");
    expect(toggle.contains(stats)).toBe(true);
    const identity = toggle.querySelector("code");
    expect(identity).toBeTruthy();
    expect(
      identity!.compareDocumentPosition(stats) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(toggle.className).toContain("min-h-11");
    expect(screen.getByTestId("commit-open-mobile1").className).toContain("min-h-11");
  });

  it("keeps commit controls at ordinary desktop size for a fine pointer", () => {
    mocks.isMobile = false;
    mocks.isFinePointer = true;
    render(<CommitRow commit={remoteCommit()} isLatest />);

    expect(screen.getByTestId(COMMIT_TOGGLE_TEST_ID).className).toContain("min-h-7");
    expect(screen.getByTestId("commit-open-remote1").className).toContain("min-h-7");
  });

  it("uses touch-sized commit controls for a coarse-pointer tablet", () => {
    mocks.isMobile = false;
    mocks.isFinePointer = false;
    render(<CommitRow commit={remoteCommit()} isLatest />);

    expect(screen.getByTestId(COMMIT_TOGGLE_TEST_ID).className).toContain("min-h-11");
    expect(screen.getByTestId("commit-open-remote1").className).toContain("min-h-11");
  });
});
