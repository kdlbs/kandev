import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { useEffect } from "react";
import { EnhancePromptButton } from "./enhance-prompt-button";

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({
    open,
    onOpenChange,
    children,
  }: {
    open?: boolean;
    onOpenChange?: (open: boolean) => void;
    children: ReactNode;
  }) => {
    useEffect(() => {
      onOpenChange?.(true);
    }, [onOpenChange]);
    return (
      <div data-testid="tooltip-root" data-open={String(open)}>
        {children}
      </div>
    );
  },
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => (
    <div data-testid="tooltip-content">{children}</div>
  ),
}));

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("EnhancePromptButton", () => {
  it("ignores tooltip open requests during the initial mount frame", async () => {
    vi.stubGlobal(
      "requestAnimationFrame",
      vi.fn(() => 1),
    );
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    render(<EnhancePromptButton onClick={vi.fn()} isLoading={false} />);

    await waitFor(() => {
      expect(screen.getByTestId("tooltip-root").getAttribute("data-open")).toBe("false");
    });
  });
});
