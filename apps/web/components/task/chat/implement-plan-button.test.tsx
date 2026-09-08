import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ImplementPlanButton } from "./implement-plan-button";

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuItem: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

afterEach(cleanup);

describe("ImplementPlanButton presentation", () => {
  it("gives both split-button controls touch-safe geometry on mobile", () => {
    render(<ImplementPlanButton onClick={vi.fn()} presentation="mobile" />);

    for (const testId of ["implement-plan-button", "implement-plan-menu-trigger"]) {
      const control = screen.getByTestId(testId);
      expect(control.className).toContain("min-h-11");
      expect(control.className).toContain("min-w-11");
    }
  });

  it("keeps both split-button controls compact by default", () => {
    render(<ImplementPlanButton onClick={vi.fn()} />);

    for (const testId of ["implement-plan-button", "implement-plan-menu-trigger"]) {
      const control = screen.getByTestId(testId);
      expect(control.className).toContain("h-7");
      expect(control.className).not.toContain("min-h-11");
      expect(control.className).not.toContain("min-w-11");
    }
  });
});
