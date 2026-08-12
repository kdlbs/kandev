import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { GitActionsDropdown } from "./session-mobile-top-bar-git-controls";

vi.mock("@/hooks/use-git-operations", () => ({
  useChangeRequestTerminology: () => ({ shortName: "PR" }),
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuItem: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuSeparator: () => null,
  DropdownMenuSub: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuSubTrigger: ({
    children,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement>) => <button {...props}>{children}</button>,
  DropdownMenuSubContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

afterEach(cleanup);

const baseProps = {
  sessionId: "session-1",
  isGitLoading: false,
  uncommittedCount: 2,
  baseBranch: "main",
  onCommitClick: vi.fn(),
  onPRClick: vi.fn(),
  onPull: vi.fn(),
  onPush: vi.fn(),
  onRebase: vi.fn(),
  onMerge: vi.fn(),
};

describe("GitActionsDropdown remote safety", () => {
  it("keeps Commit available while disabling Pull and both Push variants", () => {
    render(<GitActionsDropdown {...baseProps} pushDisabled pullDisabled />);

    expect(screen.getByRole("button", { name: /Commit/ })).not.toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: /Pull/ })).toHaveProperty("disabled", true);
    expect(
      screen
        .getAllByRole("button", { name: /^Push$/i })
        .every((button) => button.hasAttribute("disabled")),
    ).toBe(true);
    expect(screen.getByRole("button", { name: /Force Push/ })).toHaveProperty("disabled", true);
  });

  it("keeps Pull available for a provider-ahead checkout when only Push is blocked", () => {
    render(<GitActionsDropdown {...baseProps} pushDisabled />);

    expect(screen.getByRole("button", { name: /Pull/ })).not.toHaveProperty("disabled", true);
    expect(
      screen
        .getAllByRole("button", { name: /^Push$/i })
        .every((button) => button.hasAttribute("disabled")),
    ).toBe(true);
  });

  it("offers scoped contribution version choices without the generic push submenu", () => {
    const onReplaceContribution = vi.fn();
    const onUseContribution = vi.fn();
    const onViewPRVersion = vi.fn();
    render(
      <GitActionsDropdown
        {...baseProps}
        showContributionResolution
        onReplaceContribution={onReplaceContribution}
        onUseContribution={onUseContribution}
        onViewPRVersion={onViewPRVersion}
      />,
    );

    expect(screen.getByRole("button", { name: /Commit/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Replace PR branch/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Use PR version/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /View PR version/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Force Push/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Pull$/i })).toBeNull();

    screen.getByRole("button", { name: /Replace PR branch/ }).click();
    screen.getByRole("button", { name: /Use PR version/ }).click();
    screen.getByRole("button", { name: /View PR version/ }).click();
    expect(onReplaceContribution).toHaveBeenCalledOnce();
    expect(onUseContribution).toHaveBeenCalledOnce();
    expect(onViewPRVersion).toHaveBeenCalledOnce();
  });
});
