import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  clearContextWindow: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { clearContextWindow: typeof mocks.clearContextWindow }) => unknown,
  ) => selector({ clearContextWindow: mocks.clearContextWindow }),
}));

import { ResetContextButton } from "./reset-context-button";

function renderResetButton() {
  return render(
    <TooltipProvider>
      <ResetContextButton sessionId="session-1" />
    </TooltipProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ResetContextButton context-window invalidation", () => {
  beforeEach(() => {
    mocks.request.mockResolvedValue({ success: true });
  });

  it("clears the session usage cache after a successful reset", async () => {
    renderResetButton();

    fireEvent.click(screen.getByTestId("reset-context-button"));
    fireEvent.click(await screen.findByTestId("reset-context-confirm"));

    await waitFor(() => {
      expect(mocks.request).toHaveBeenCalledWith(
        "session.reset_context",
        { session_id: "session-1" },
        30000,
      );
      expect(mocks.clearContextWindow).toHaveBeenCalledWith("session-1");
    });
  });

  it("keeps the session usage cache when reset fails", async () => {
    mocks.request.mockRejectedValue(new Error("reset failed"));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    renderResetButton();

    fireEvent.click(screen.getByTestId("reset-context-button"));
    await act(async () => {
      fireEvent.click(await screen.findByTestId("reset-context-confirm"));
    });

    await waitFor(() => expect(mocks.request).toHaveBeenCalled());
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
    consoleError.mockRestore();
  });
});
