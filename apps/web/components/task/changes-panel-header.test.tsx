import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PullDropdown } from "./changes-panel-header";

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
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
