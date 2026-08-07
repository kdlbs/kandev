import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  refetch: vi.fn(),
  useCommitDetail: vi.fn(() => ({
    files: null,
    commit: null,
    loading: false,
    error: "Commit detail unavailable",
    refetch: mocks.refetch,
  })),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ tasks: { activeSessionId: null } }),
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

import { CommitDetailPanel, CommitDiffView } from "./commit-detail-panel";

afterEach(() => {
  mocks.refetch.mockReset();
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

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
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
});
