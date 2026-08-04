import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  refetch: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ tasks: { activeSessionId: null } }),
}));

vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({ commits: [] }),
}));

vi.mock("@/hooks/domains/session/use-commit-detail", () => ({
  useCommitDetail: () => ({
    files: null,
    commit: null,
    loading: false,
    error: "Commit detail unavailable",
    refetch: mocks.refetch,
  }),
}));

vi.mock("@/hooks/use-panel-actions", () => ({
  usePanelActions: () => ({ openFile: vi.fn() }),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  setPanelTitle: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("./panel-primitives", () => ({
  PanelRoot: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  PanelBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

import { CommitDiffView } from "./commit-detail-panel";

afterEach(() => {
  mocks.refetch.mockReset();
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

    fireEvent.click(screen.getByRole("button", { name: "common:retry" }));
    expect(mocks.refetch).toHaveBeenCalledOnce();
  });
});
