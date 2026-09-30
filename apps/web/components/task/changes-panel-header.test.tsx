import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PullDropdown } from "./changes-panel-header";
import { ChangesPanelHeaderOverflowActions } from "./changes-panel-header-actions";

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuItem: ({ children, ...props }: { children: React.ReactNode }) => (
    <div role="menuitem" {...props}>
      {children}
    </div>
  ),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("./panel-primitives", () => ({
  PanelHeaderBarSplit: ({
    left,
    right,
    overflow,
  }: {
    left?: ReactNode;
    right?: ReactNode;
    overflow?: ReactNode;
  }) => (
    <div>
      {left}
      {right}
      {overflow}
    </div>
  ),
  PanelHeaderOverflowMenu: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("./changes-panel-per-repo-menu", () => ({
  PerRepoPullMenu: () => null,
}));

afterEach(cleanup);

describe("PullDropdown remote safety", () => {
  it("keeps the configured-upstream reason reachable while Pull is disabled", () => {
    render(
      <PullDropdown
        behindCount={0}
        pullDisabled
        pullDisabledReason="Pull requires a configured upstream for this checkout."
        isLoading={false}
        loadingOperation={null}
        repoNames={[""]}
        perRepoStatus={[]}
        onRepoPull={vi.fn()}
        onRepoRebase={vi.fn()}
        onRepoMerge={vi.fn()}
      />,
    );

    const pullButton = screen.getByRole("button", { name: /Pull/ });
    expect(pullButton).toHaveProperty("disabled", true);
    expect(pullButton.parentElement?.getAttribute("tabindex")).toBe("0");
    expect(screen.getByText("Pull requires a configured upstream for this checkout.")).toBeTruthy();
    expect(
      screen.queryByText(
        "Current PR history is unavailable. The checkout history is shown without assuming a rewrite.",
      ),
    ).toBeNull();
  });
});

describe("ChangesPanelHeaderOverflowActions", () => {
  it("does not expose Diff or Review when the Changes panel has no reviewable content", () => {
    render(
      <ChangesPanelHeaderOverflowActions
        showDiffReview={false}
        onOpenDiffAll={vi.fn()}
        onOpenReview={vi.fn()}
      />,
    );

    expect(screen.queryByText("Diff")).toBeNull();
    expect(screen.queryByText("Review")).toBeNull();
  });

  it("keeps Diff and Review available when reviewable content exists", () => {
    render(
      <ChangesPanelHeaderOverflowActions
        showDiffReview
        onOpenDiffAll={vi.fn()}
        onOpenReview={vi.fn()}
      />,
    );

    expect(screen.getByText("Diff")).toBeTruthy();
    expect(screen.getByText("Review")).toBeTruthy();
  });
});
