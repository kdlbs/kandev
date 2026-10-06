import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PullDropdown } from "./changes-panel-header";
import {
  ChangesPanelHeaderLeft,
  ChangesPanelHeaderOverflowActions,
} from "./changes-panel-header-actions";

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
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="tooltip-content">{children}</div>
  ),
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
    render(<ChangesPanelHeaderOverflowActions showDiffReview={false} onOpenDiffAll={vi.fn()} />);

    expect(screen.queryByText("Diff")).toBeNull();
    expect(screen.queryByText("Review")).toBeNull();
  });

  it("offers only the hidden Diff action, without duplicating visible toolbar actions", () => {
    render(<ChangesPanelHeaderOverflowActions showDiffReview onOpenDiffAll={vi.fn()} />);

    expect(screen.getByText("Diff")).toBeTruthy();
    expect(screen.queryByText("Review")).toBeNull();
    expect(screen.queryByText("Walk me through these changes")).toBeNull();
  });
});

describe("ChangesPanelHeaderLeft feedback", () => {
  it("keeps initial loading visible when review actions are unavailable", () => {
    const { container } = render(
      <ChangesPanelHeaderLeft
        showDiffReview={false}
        refreshStatus="loading"
        hasPriorData={false}
        failedRepositories={[]}
      />,
    );

    expect(screen.getByRole("status").textContent).toContain("Loading changes...");
    expect(container.querySelector("[data-testid='changes-refresh-status']")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Review" })).toBeNull();
  });

  it("places refresh status after Review and shows a localized Review tooltip", () => {
    const onOpenReview = vi.fn();
    render(
      <ChangesPanelHeaderLeft
        showDiffReview
        primaryOnly
        onOpenReview={onOpenReview}
        refreshStatus="loading"
        hasPriorData
        failedRepositories={[]}
      />,
    );

    const review = screen.getByRole("button", { name: "Review" });
    const status = screen.getByTestId("changes-refresh-status");
    expect(review.compareDocumentPosition(status) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(
      screen.getAllByTestId("tooltip-content").some((item) => item.textContent === "Review"),
    ).toBe(true);
    fireEvent.click(review);
    expect(onOpenReview).toHaveBeenCalledTimes(1);
  });
});
