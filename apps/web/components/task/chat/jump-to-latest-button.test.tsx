import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { JumpToLatestButton } from "./jump-to-latest-button";

afterEach(cleanup);

describe("JumpToLatestButton", () => {
  it("is absent when no transcript content is below the viewport", () => {
    render(
      <TooltipProvider delayDuration={0}>
        <JumpToLatestButton isVisible={false} onClick={vi.fn()} />
      </TooltipProvider>,
    );

    expect(screen.queryByTestId("jump-to-latest-button")).toBeNull();
  });

  it("has a localized accessible name and activates the latest-position action", () => {
    const onClick = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <JumpToLatestButton isVisible onClick={onClick} />
      </TooltipProvider>,
    );

    const button = screen.getByRole("button", { name: /jump to latest/i });
    expect(button.tagName).toBe("BUTTON");
    fireEvent.click(button);
    expect(onClick).toHaveBeenCalledOnce();
  });
});
