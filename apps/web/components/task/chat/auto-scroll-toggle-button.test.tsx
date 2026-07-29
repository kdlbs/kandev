import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

const setTranscriptAutoScrollEnabledMock = vi.fn();
let enabledBySessionId: Record<string, boolean> = {};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      transcriptAutoScroll: { enabledBySessionId },
      setTranscriptAutoScrollEnabled: setTranscriptAutoScrollEnabledMock,
    }),
}));

import { AutoScrollToggleButton } from "./auto-scroll-toggle-button";

const TOGGLE_TESTID = "auto-scroll-toggle-button";

function renderButton(sessionId: string) {
  return render(
    <TooltipProvider>
      <AutoScrollToggleButton sessionId={sessionId} />
    </TooltipProvider>,
  );
}

function getButton() {
  return screen.getByTestId(TOGGLE_TESTID);
}

describe("AutoScrollToggleButton", () => {
  beforeEach(() => {
    enabledBySessionId = {};
    setTranscriptAutoScrollEnabledMock.mockReset();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    cleanup();
  });
  it("renders enabled by default and offers to turn auto-scroll off", () => {
    renderButton("session-a");
    const button = getButton();
    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(button.getAttribute("aria-label")).toMatch(/turn off/i);
  });

  it("clicking while enabled disables auto-scroll for the session", () => {
    renderButton("session-a");
    fireEvent.click(getButton());
    expect(setTranscriptAutoScrollEnabledMock).toHaveBeenCalledWith("session-a", false);
  });

  it("renders disabled state and offers to turn auto-scroll on", () => {
    enabledBySessionId = { "session-a": false };
    renderButton("session-a");
    const button = getButton();
    expect(button.getAttribute("aria-pressed")).toBe("false");
    expect(button.getAttribute("aria-label")).toMatch(/turn on/i);
  });

  it("clicking while disabled re-enables auto-scroll for the session", () => {
    enabledBySessionId = { "session-a": false };
    renderButton("session-a");
    fireEvent.click(getButton());
    expect(setTranscriptAutoScrollEnabledMock).toHaveBeenCalledWith("session-a", true);
  });

  it("only toggles the session it was rendered for", () => {
    enabledBySessionId = { "session-a": false, "session-b": true };
    renderButton("session-b");
    fireEvent.click(getButton());
    expect(setTranscriptAutoScrollEnabledMock).toHaveBeenCalledWith("session-b", false);
  });

  it("falls back to the sessionStorage-persisted preference when the store hasn't hydrated this session yet", () => {
    window.sessionStorage.setItem("kandev.transcript-auto-scroll-enabled.session-a", "false");
    renderButton("session-a");
    const button = getButton();
    expect(button.getAttribute("aria-pressed")).toBe("false");
  });
});
