import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { CommandPanelDialog } from "./command-panel-dialog";
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true }),
}));

it("unmounts the closed phone palette before another drawer opens", () => {
  const { rerender } = render(
    <CommandPanelDialog open onOpenChange={vi.fn()}>
      <button>Search fixture</button>
    </CommandPanelDialog>,
  );
  expect(screen.getByRole("button", { name: "Search fixture" })).toBeTruthy();
  rerender(
    <CommandPanelDialog open={false} onOpenChange={vi.fn()}>
      <button>Search fixture</button>
    </CommandPanelDialog>,
  );
  expect(screen.queryByText("Search fixture")).toBeNull();
});
