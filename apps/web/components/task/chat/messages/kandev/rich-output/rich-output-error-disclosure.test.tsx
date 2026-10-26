import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { RichOutputErrorDisclosure } from "./rich-output-error-disclosure";

type MediaListener = (event: MediaQueryListEvent) => void;

const listeners = new Set<MediaListener>();
const originalMatchMedia = window.matchMedia;
const originalInnerWidth = Object.getOwnPropertyDescriptor(window, "innerWidth");

function setViewport(width: number) {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    value: width,
  });
  window.matchMedia = ((query: string) => {
    const minWidth = query.match(/min-width:\s*(\d+)px/);
    const maxWidth = query.match(/max-width:\s*(\d+)px/);
    const matches = query.includes("pointer: fine")
      ? true
      : (!minWidth || width >= Number(minWidth[1])) && (!maxWidth || width <= Number(maxWidth[1]));
    return {
      media: query,
      matches,
      onchange: null,
      addEventListener: (_type: string, listener: MediaListener) => listeners.add(listener),
      removeEventListener: (_type: string, listener: MediaListener) => listeners.delete(listener),
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => true,
    } as MediaQueryList;
  }) as typeof window.matchMedia;
}

function resizeViewport(width: number) {
  setViewport(width);
  act(() => {
    for (const listener of listeners) listener({ matches: true } as MediaQueryListEvent);
  });
}

describe("RichOutputErrorDisclosure responsive state", () => {
  beforeEach(() => {
    listeners.clear();
    setViewport(768);
  });

  afterEach(() => {
    cleanup();
    listeners.clear();
    window.matchMedia = originalMatchMedia;
    if (originalInnerWidth) {
      Object.defineProperty(window, "innerWidth", originalInnerWidth);
    }
  });

  it("keeps an open disclosure mounted and open across the phone breakpoint", () => {
    render(
      <TooltipProvider>
        <RichOutputErrorDisclosure diagnostic="Retained validation detail." />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Show visual output error" }));
    expect(screen.getByRole("dialog", { name: "Visual output error" })).toBeTruthy();

    resizeViewport(767);
    expect(screen.getByRole("dialog", { name: "Visual output error" })).toBeTruthy();
    expect(screen.getByTestId("rich-output-error-drawer").textContent).toContain(
      "Retained validation detail.",
    );

    resizeViewport(768);
    expect(screen.getByRole("dialog", { name: "Visual output error" })).toBeTruthy();
    expect(screen.getByTestId("rich-output-error-popover").textContent).toContain(
      "Retained validation detail.",
    );
  });
});
