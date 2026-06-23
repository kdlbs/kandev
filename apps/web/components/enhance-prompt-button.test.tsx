import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
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
  cleanup();
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

  it("makes the disabled tooltip wrapper keyboard focusable", () => {
    render(<EnhancePromptButton onClick={vi.fn()} isLoading={false} isConfigured={false} />);

    const tooltipTrigger = screen.getByLabelText(
      "Configure a utility agent in settings to enable AI enhancement",
    );
    expect(tooltipTrigger.getAttribute("tabindex")).toBe("0");
  });

  it("leaves focus on the button when enabled", () => {
    render(<EnhancePromptButton onClick={vi.fn()} isLoading={false} />);

    const button = screen.getByTestId("enhance-prompt-button");
    expect(button.parentElement?.getAttribute("tabindex")).toBe("-1");
  });
});
