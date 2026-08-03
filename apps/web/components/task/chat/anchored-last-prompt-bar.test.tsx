import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { AnchoredLastPromptBar } from "./anchored-last-prompt-bar";

const BAR_TESTID = "anchored-last-prompt-bar";
const EXPAND_TESTID = "anchored-last-prompt-expand";
const TEXT_TESTID = "anchored-last-prompt-text";
const CONTENT_TESTID = "anchored-last-prompt-content";
const SHORT_TEXT = "fix the bug";
const LONG_TEXT =
  "Please refactor the authentication module to support OAuth as well as the existing session cookie flow, and add tests.";
const originalClientHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientHeight");
const originalScrollHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollHeight");
const originalOffsetHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "offsetHeight");

function renderBar(
  overrides: Partial<Parameters<typeof AnchoredLastPromptBar>[0]> = {},
  container?: HTMLElement,
) {
  return render(
    <TooltipProvider delayDuration={0}>
      <AnchoredLastPromptBar
        promptText={SHORT_TEXT}
        isVisible={true}
        onScrollUp={vi.fn()}
        {...overrides}
      />
    </TooltipProvider>,
    container ? { container: container.appendChild(document.createElement("div")) } : undefined,
  );
}

afterEach(() => {
  cleanup();
  restorePromptMeasurements();
});

function setPromptMeasurements(clientHeight: number, scrollHeight: number) {
  Object.defineProperties(HTMLElement.prototype, {
    clientHeight: { configurable: true, get: () => clientHeight },
    scrollHeight: { configurable: true, get: () => scrollHeight },
  });
}

function setContentHeight(offsetHeight: number) {
  Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
    configurable: true,
    get: () => offsetHeight,
  });
}

function restorePromptMeasurements() {
  restoreMeasurement("clientHeight", originalClientHeight);
  restoreMeasurement("scrollHeight", originalScrollHeight);
  restoreMeasurement("offsetHeight", originalOffsetHeight);
}

function restoreMeasurement(
  property: "clientHeight" | "scrollHeight" | "offsetHeight",
  descriptor: PropertyDescriptor | undefined,
) {
  if (descriptor) {
    Object.defineProperty(HTMLElement.prototype, property, descriptor);
    return;
  }
  Reflect.deleteProperty(HTMLElement.prototype, property);
}

describe("AnchoredLastPromptBar", () => {
  it("reflects visibility via data-state for the fluid show/hide transform", () => {
    const { rerender } = renderBar({ isVisible: false });
    expect(screen.getByTestId(BAR_TESTID).getAttribute("data-state")).toBe("closed");

    rerender(
      <TooltipProvider delayDuration={0}>
        <AnchoredLastPromptBar promptText={SHORT_TEXT} isVisible={true} onScrollUp={vi.fn()} />
      </TooltipProvider>,
    );
    expect(screen.getByTestId(BAR_TESTID).getAttribute("data-state")).toBe("open");
  });

  it("stacks above transcript code-block actions while open", () => {
    renderBar({ isVisible: true });

    expect(screen.getByTestId(BAR_TESTID).className).toContain("z-20");
  });

  it("aligns its outer controls with the transcript inset without entering scroll flow", () => {
    renderBar({ isVisible: true });

    const bar = screen.getByTestId(BAR_TESTID);
    const overlay = bar.firstElementChild;
    const content = overlay?.firstElementChild?.firstElementChild;

    expect(bar.className).toContain("h-0");
    expect(overlay?.className).toContain("absolute");
    expect(content?.className).not.toMatch(/\b(?:pl|pr)-/);
    expect(content?.className).toContain("px-4");
    expect(content?.className).not.toContain("px-2");
  });

  it("removes hidden controls from the focus order while closed", () => {
    renderBar({ isVisible: false, promptText: LONG_TEXT });

    const bar = screen.getByTestId(BAR_TESTID);
    expect(bar.getAttribute("aria-hidden")).toBe("true");
    expect(bar.hasAttribute("inert")).toBe(true);
  });

  it("renders the shortened prompt text", () => {
    renderBar();
    screen.getByText(SHORT_TEXT);
  });

  it("renders the pinned copy with the user-message Markdown treatment", () => {
    renderBar({
      promptText: "Use `terraform apply`.\n\n## Steps\n\n- Validate the plan",
    });

    const preview = screen.getByTestId(TEXT_TESTID);
    expect(preview.querySelector("code")?.textContent).toBe("terraform apply");
    expect(preview.querySelector("h2")?.textContent).toBe("Steps");
    expect(preview.querySelector("li")?.textContent).toBe("Validate the plan");
  });

  it("does not offer expand when a long prompt fits within two rendered lines", () => {
    setPromptMeasurements(40, 40);
    renderBar({ promptText: LONG_TEXT });

    expect(screen.queryByTestId(EXPAND_TESTID)).toBeNull();
  });

  it("toggles a scrollable, max-height-bounded expanded view exactly like the message queue", () => {
    setPromptMeasurements(40, 80);
    renderBar({ promptText: LONG_TEXT });
    const expandButton = screen.getByTestId(EXPAND_TESTID);
    const textEl = screen.getByTestId(TEXT_TESTID);

    expect(textEl.getAttribute("data-expanded")).toBe("false");
    expect(expandButton.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(expandButton);

    expect(textEl.getAttribute("data-expanded")).toBe("true");
    expect(expandButton.getAttribute("aria-expanded")).toBe("true");
    expect(textEl.className).toContain("overflow-y-auto");
  });

  it("caps the expanded view at 40% of the transcript container's height", () => {
    setPromptMeasurements(40, 80);
    const container = document.createElement("div");
    container.className = "chat-message-list";
    Object.defineProperty(container, "clientHeight", { configurable: true, value: 1000 });
    document.body.appendChild(container);
    renderBar({ promptText: LONG_TEXT }, container);

    fireEvent.click(screen.getByTestId(EXPAND_TESTID));
    const textEl = screen.getByTestId(TEXT_TESTID);

    expect(textEl.style.maxHeight).toBe("400px");
    container.remove();
  });

  it("scales the expanded cap proportionally at a different container height", () => {
    setPromptMeasurements(40, 80);
    const container = document.createElement("div");
    container.className = "chat-message-list";
    Object.defineProperty(container, "clientHeight", { configurable: true, value: 500 });
    document.body.appendChild(container);
    renderBar({ promptText: LONG_TEXT }, container);

    fireEvent.click(screen.getByTestId(EXPAND_TESTID));
    const textEl = screen.getByTestId(TEXT_TESTID);

    expect(textEl.style.maxHeight).toBe("200px");
    container.remove();
  });

  it("falls back to a viewport-relative expanded cap when no transcript container ancestor is found", () => {
    setPromptMeasurements(40, 80);
    renderBar({ promptText: LONG_TEXT });

    fireEvent.click(screen.getByTestId(EXPAND_TESTID));
    const textEl = screen.getByTestId(TEXT_TESTID);

    expect(textEl.style.maxHeight).toBe("40vh");
  });
});

describe("AnchoredLastPromptBar controls", () => {
  it("calls onScrollUp when the scroll-up button is pressed", () => {
    const onScrollUp = vi.fn();
    renderBar({ onScrollUp });

    fireEvent.click(screen.getByRole("button", { name: /scroll to last prompt/i }));
    expect(onScrollUp).toHaveBeenCalledOnce();
  });

  it("omits the anchored scroll control when transcript navigation is disabled", () => {
    const props = {
      promptText: SHORT_TEXT,
      isVisible: true,
      onScrollUp: vi.fn(),
      showScrollToLastPrompt: false,
    } as Parameters<typeof AnchoredLastPromptBar>[0];
    render(
      <TooltipProvider delayDuration={0}>
        <AnchoredLastPromptBar {...props} />
      </TooltipProvider>,
    );

    expect(screen.queryByRole("button", { name: /scroll to last prompt/i })).toBeNull();
  });
});

describe("AnchoredLastPromptBar height reporting", () => {
  it("reports the pinned content's rendered height on mount, independent of open/closed state", () => {
    setContentHeight(76);
    const onHeightChange = vi.fn();

    renderBar({ isVisible: false, onHeightChange });

    expect(onHeightChange).toHaveBeenCalledWith(76);
    screen.getByTestId(CONTENT_TESTID);
  });

  it("reports zero height on unmount so a removed bar stops reserving scroll space", () => {
    setContentHeight(76);
    const onHeightChange = vi.fn();
    const { unmount } = renderBar({ onHeightChange });
    onHeightChange.mockClear();

    unmount();

    expect(onHeightChange).toHaveBeenCalledWith(0);
  });
});
