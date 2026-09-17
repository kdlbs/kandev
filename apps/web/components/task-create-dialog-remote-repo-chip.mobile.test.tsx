import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { RemoteRepoChip } from "./task-create-dialog-remote-repo-chip";
import {
  githubSite,
  makeAccessible,
  noopBranch,
  noopRemove,
  renderInProvider,
  row,
} from "./task-create-dialog-remote-repo-chip-test-support";

const touchDrawer = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchDrawer.enabled,
}));

vi.mock("@/components/task/mobile/mobile-picker-sheet", () => ({
  MobilePickerSheet: ({
    open,
    title,
    contentTestId,
    children,
  }: {
    open: boolean;
    title: string;
    contentTestId?: string;
    children: ReactNode;
  }) =>
    open ? (
      <div data-testid="mobile-picker-sheet">
        <h2>{title}</h2>
        <div data-testid={contentTestId}>{children}</div>
      </div>
    ) : null,
}));

afterEach(() => {
  cleanup();
  touchDrawer.enabled = false;
});

describe("RemoteRepoChip mobile picker", () => {
  it("uses sheet navigation and touch-sized repository and branch controls", () => {
    touchDrawer.enabled = true;
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: "https://github.com/acme/site", branch: "main" })}
        branches={[{ name: "main", type: "remote" }]}
        branchesLoading={false}
        accessibleRepos={makeAccessible({ repos: [githubSite()] })}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );

    const repositoryTrigger = screen.getByTestId("remote-repo-chip-trigger");
    const branchTrigger = screen.getByTestId("remote-branch-chip-trigger");
    expect(repositoryTrigger.className).toContain("min-h-11");
    expect(branchTrigger.className).toContain("min-h-11");

    fireEvent.click(repositoryTrigger);
    expect(screen.getByTestId("mobile-picker-sheet")).toBeTruthy();
    expect(screen.getByTestId("remote-repo-popover-content")).toBeTruthy();
  });
});
