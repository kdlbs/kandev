import { fireEvent, render, screen } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  refetch: vi.fn(),
  mounts: vi.fn(),
  unmounts: vi.fn(),
  activeSessionId: null as string | null,
  useCommitDetail: vi.fn(() => ({
    files: null as Record<string, never> | null,
    commit: null,
    loading: false,
    error: "Commit detail unavailable" as string | null,
    refetch: mocks.refetch,
  })),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ tasks: { activeSessionId: mocks.activeSessionId } }),
}));

vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({ commits: [] }),
}));

vi.mock("@/hooks/domains/session/use-commit-detail", () => ({
  useCommitDetail: mocks.useCommitDetail,
}));

vi.mock("@/hooks/use-panel-actions", () => ({
  usePanelActions: () => ({ openFile: vi.fn() }),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  setPanelTitle: vi.fn(),
}));

vi.mock("./panel-primitives", () => ({
  PanelRoot: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  PanelBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock("./commit-detail-content", () => ({
  CommitDetailContent: ({ target }: { target: { sha: string } }) => {
    useEffect(() => {
      mocks.mounts();
      return mocks.unmounts;
    }, []);
    return <div data-testid="commit-detail-content" data-commit-sha={target.sha} />;
  },
}));

import { CommitDetailPanel, CommitDiffView } from "./commit-detail-panel";

afterEach(() => {
  mocks.refetch.mockReset();
  mocks.mounts.mockReset();
  mocks.unmounts.mockReset();
  mocks.activeSessionId = null;
  mocks.useCommitDetail.mockClear();
});

describe("CommitDiffView error state", () => {
  it("does not present a protocol failure as an empty commit and offers retry", () => {
    render(
      <CommitDiffView
        target={{
          source: "github",
          sha: "remote123",
          workspaceId: "workspace-1",
          owner: "acme",
          repo: "widget",
        }}
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain("Commit detail unavailable");
    expect(screen.queryByText("No files in this commit")).toBeNull();

    const retryButton = screen.getByRole("button", { name: "Retry" });
    expect(retryButton.getAttribute("data-size")).toBe("default");
    expect(retryButton.className).toContain("h-7");
    expect(retryButton.className).toContain("pointer:coarse");
    fireEvent.click(retryButton);
    expect(mocks.refetch).toHaveBeenCalledOnce();
  });
});

describe("CommitDetailPanel target validation", () => {
  it("does not accept a partial GitHub target from serialized params", () => {
    render(
      <CommitDetailPanel
        panelId="commit-detail"
        params={{ target: { source: "github", sha: "partial" } }}
      />,
    );

    expect(mocks.useCommitDetail).toHaveBeenCalledWith({ source: "local", sha: "" });
  });

  it("remounts commit content when the active session changes", () => {
    mocks.activeSessionId = "session-a";
    mocks.useCommitDetail.mockReturnValue({
      files: {},
      commit: null,
      loading: false,
      error: null,
      refetch: mocks.refetch,
    });
    const target = {
      source: "github" as const,
      sha: "remote123",
      workspaceId: "workspace-1",
      owner: "acme",
      repo: "widget",
    };
    const { rerender } = render(<CommitDetailPanel panelId="commit-detail" params={{ target }} />);
    expect(mocks.mounts).toHaveBeenCalledOnce();

    mocks.activeSessionId = "session-b";
    rerender(<CommitDetailPanel panelId="commit-detail" params={{ target }} />);

    expect(mocks.mounts).toHaveBeenCalledTimes(2);
    expect(mocks.unmounts).toHaveBeenCalledOnce();
  });
});
