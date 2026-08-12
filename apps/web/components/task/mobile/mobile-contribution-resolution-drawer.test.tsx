import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MobileContributionResolutionDrawer } from "./mobile-contribution-resolution-drawer";

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div>{children}</div> : null,
  DrawerContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <section {...props}>{children}</section>
  ),
  DrawerDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  DrawerFooter: ({ children }: { children: React.ReactNode }) => <footer>{children}</footer>,
  DrawerHeader: ({ children }: { children: React.ReactNode }) => <header>{children}</header>,
  DrawerTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

describe("MobileContributionResolutionDrawer", () => {
  it("uses an inset safe-area drawer and keeps the replacement action touch-sized", () => {
    const onConfirm = vi.fn();
    render(
      <MobileContributionResolutionDrawer
        open
        action="replace"
        repositoryName="frontend"
        expectedRemoteHead="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        isLoading={false}
        onOpenChange={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    const drawer = screen.getByTestId("mobile-remote-contribution-drawer");
    expect(drawer.className).toContain("bottom-3");
    expect(drawer.className).toContain("safe-area-inset-bottom");
    expect(screen.getByRole("heading", { name: "Replace PR branch" })).toBeTruthy();
    const confirm = screen.getByTestId("mobile-remote-contribution-confirm");
    expect(confirm.className).toContain("h-11");
    fireEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("states recovery and clean-tree requirements for the PR version", () => {
    render(
      <MobileContributionResolutionDrawer
        open
        action="use"
        repositoryName="frontend"
        expectedRemoteHead="cccccccccccccccccccccccccccccccccccccccc"
        isLoading={false}
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Use PR version" })).toBeTruthy();
    expect(screen.getByText(/recovery branch/)).toBeTruthy();
    expect(screen.getByText(/working tree must be clean/)).toBeTruthy();
  });
});
