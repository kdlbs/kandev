import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RemoteContributionResolutionDialog } from "./remote-contribution-resolution-dialog";

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => <section>{children}</section>,
  DialogDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children: React.ReactNode }) => <footer>{children}</footer>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <header>{children}</header>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

describe("RemoteContributionResolutionDialog", () => {
  it("names the repository and exact provider head before replacement", () => {
    const onConfirm = vi.fn();
    render(
      <RemoteContributionResolutionDialog
        open
        action="replace"
        repositoryName="frontend"
        expectedRemoteHead="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        isLoading={false}
        onOpenChange={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByRole("heading", { name: "Replace PR branch" })).toBeTruthy();
    expect(screen.getByText(/frontend/)).toBeTruthy();
    expect(screen.getByText(/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/)).toBeTruthy();
    expect(screen.getByText(/provider branch/)).toBeTruthy();
    fireEvent.click(screen.getByTestId("remote-contribution-confirm"));
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("states the clean-tree and recovery-branch guard for adoption", () => {
    render(
      <RemoteContributionResolutionDialog
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
