import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ScrollToLastPromptButton, ScrollToStartButton } from "./scroll-to-last-prompt-button";

afterEach(cleanup);

describe("ScrollToLastPromptButton", () => {
  it("calls onClick when pressed", () => {
    const onClick = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <ScrollToLastPromptButton onClick={onClick} />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /scroll to last prompt/i }));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("is keyboard accessible via a labeled button", () => {
    render(
      <TooltipProvider delayDuration={0}>
        <ScrollToLastPromptButton onClick={vi.fn()} />
      </TooltipProvider>,
    );

    const button = screen.getByRole("button", { name: /scroll to last prompt/i });
    expect(button.tagName).toBe("BUTTON");
  });

  it("uses an upward arrow for the last prompt and a bar-to-up arrow for transcript start", () => {
    render(
      <TooltipProvider delayDuration={0}>
        <ScrollToLastPromptButton onClick={vi.fn()} />
        <ScrollToStartButton onClick={vi.fn()} />
      </TooltipProvider>,
    );

    expect(
      screen
        .getByTestId("scroll-to-last-prompt-button")
        .querySelector("svg")
        ?.getAttribute("class"),
    ).toContain("tabler-icon-arrow-up");
    expect(
      screen.getByTestId("scroll-to-start-button").querySelector("svg")?.getAttribute("class"),
    ).toContain("tabler-icon-arrow-bar-to-up");
  });

  it("flips to a downward arrow when the last prompt sits below the viewport", () => {
    render(
      <TooltipProvider delayDuration={0}>
        <ScrollToLastPromptButton onClick={vi.fn()} direction="down" />
      </TooltipProvider>,
    );

    expect(
      screen
        .getByTestId("scroll-to-last-prompt-button")
        .querySelector("svg")
        ?.getAttribute("class"),
    ).toContain("tabler-icon-arrow-down");
  });

  it("still scrolls to the last prompt when pointing down", () => {
    const onClick = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <ScrollToLastPromptButton onClick={onClick} direction="down" />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /scroll to last prompt/i }));
    expect(onClick).toHaveBeenCalledOnce();
  });
});

describe("ScrollToStartButton", () => {
  it("calls onClick when pressed", () => {
    const onClick = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <ScrollToStartButton onClick={onClick} />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /scroll to start of transcript/i }));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("is keyboard accessible via a labeled button distinct from the last-prompt button", () => {
    render(
      <TooltipProvider delayDuration={0}>
        <ScrollToLastPromptButton onClick={vi.fn()} />
        <ScrollToStartButton onClick={vi.fn()} />
      </TooltipProvider>,
    );

    expect(screen.getByRole("button", { name: /scroll to last prompt/i }).tagName).toBe("BUTTON");
    expect(screen.getByRole("button", { name: /scroll to start of transcript/i }).tagName).toBe(
      "BUTTON",
    );
  });
});
