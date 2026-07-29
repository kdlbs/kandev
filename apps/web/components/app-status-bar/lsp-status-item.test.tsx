import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LspStatusItem } from "./lsp-status-item";

const lsp = vi.hoisted(() => ({
  status: { state: "starting" } as const,
  progress: {
    initializingSince: 2_000,
    active: [],
    completed: null,
    hasReportedProgress: false,
  },
  toggle: vi.fn(),
}));

vi.mock("@/hooks/use-lsp", () => ({
  useLspStatus: () => ({
    status: lsp.status,
    progress: lsp.progress,
    toggle: lsp.toggle,
  }),
}));

describe("LspStatusItem", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(67_000);
    lsp.toggle.mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("shows the active language and live initialize summary", () => {
    render(
      <TooltipProvider>
        <LspStatusItem sessionId="session-1" monacoLanguage="kotlin" />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("app-status-lsp").textContent).toContain("Kotlin");
    expect(screen.getByTestId("app-status-lsp").textContent).toContain(
      "Server process started · 1m 05s",
    );
  });

  it("opens the shared details and lifecycle action", () => {
    render(
      <TooltipProvider>
        <LspStatusItem sessionId="session-1" monacoLanguage="kotlin" />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId("app-status-lsp"));
    expect(screen.getByTestId("lsp-progress-details")).toBeTruthy();
    fireEvent.click(screen.getByTestId("lsp-lifecycle-action"));
    expect(lsp.toggle).toHaveBeenCalledOnce();
  });
});
