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

const RESET_CONTEXT_BUTTON_TEST_ID = "reset-context-button";
const RESET_CONTEXT_CONFIRM_TEST_ID = "reset-context-confirm";
const RESET_CONTEXT_WARNING =
  "This will clear the agent's conversation history and start a fresh context. Your workspace, files, and git state will be preserved.";

function renderResetButton(
  presentation: "desktop" | "mobile" = "desktop",
  onConfirmationOpenChange?: (open: boolean) => void,
) {
  return render(
    <TooltipProvider>
      <ResetContextButton
        sessionId="session-1"
        presentation={presentation}
        onConfirmationOpenChange={onConfirmationOpenChange}
      />
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

  it("confirms from an anchored popover and clears the session usage cache once", async () => {
    renderResetButton();

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
    fireEvent.click(await screen.findByTestId(RESET_CONTEXT_CONFIRM_TEST_ID));

    await waitFor(() => {
      expect(mocks.request).toHaveBeenCalledWith(
        "session.reset_context",
        { session_id: "session-1" },
        30000,
      );
      expect(mocks.clearContextWindow).toHaveBeenCalledWith("session-1");
    });
    expect(mocks.request).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("leaves transcript and context untouched when desktop confirmation is cancelled", async () => {
    renderResetButton();

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));

    expect(mocks.request).not.toHaveBeenCalled();
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
    expect(screen.queryByTestId("reset-context-confirm-popover")).toBeNull();
  });

  it("keeps the session usage cache when reset fails", async () => {
    mocks.request.mockRejectedValue(new Error("reset failed"));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    renderResetButton();

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
    await act(async () => {
      fireEvent.click(await screen.findByTestId(RESET_CONTEXT_CONFIRM_TEST_ID));
    });

    await waitFor(() => expect(mocks.request).toHaveBeenCalled());
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
    consoleError.mockRestore();
  });

  it("keeps reset busy while the confirmed request is pending", async () => {
    let resolveRequest!: (value: { success: boolean }) => void;
    mocks.request.mockReturnValue(
      new Promise<{ success: boolean }>((resolve) => {
        resolveRequest = resolve;
      }),
    );

    renderResetButton();
    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
    fireEvent.click(await screen.findByTestId(RESET_CONTEXT_CONFIRM_TEST_ID));

    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID)).toHaveProperty("disabled", true);
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();

    await act(async () => {
      resolveRequest({ success: true });
    });
  });
});

describe("ResetContextButton mobile presentation", () => {
  beforeEach(() => {
    mocks.request.mockResolvedValue({ success: true });
  });

  it("uses inline touch actions without opening a second overlay", async () => {
    renderResetButton("mobile");

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));

    const inlineConfirmation = await screen.findByTestId("reset-context-inline-confirm");
    expect(inlineConfirmation).toBeTruthy();
    expect(screen.queryByTestId("reset-context-confirm-popover")).toBeNull();
    expect(screen.getByTestId(RESET_CONTEXT_CONFIRM_TEST_ID).className).toContain("h-11");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(mocks.request).not.toHaveBeenCalled();
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
  });

  it("shows the destructive warning in the inline confirmation", async () => {
    renderResetButton("mobile");

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));

    expect(await screen.findByText(RESET_CONTEXT_WARNING)).toBeTruthy();
  });

  it("returns focus to the reset trigger after cancelling inline", async () => {
    renderResetButton("mobile");

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));

    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID)),
    );
  });

  it("returns focus to the reset trigger after dismissing inline with Escape", async () => {
    renderResetButton("mobile");

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
    fireEvent.keyDown(await screen.findByRole("group"), { key: "Escape" });

    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID)),
    );
  });

  it("dispatches reset once after inline confirmation", async () => {
    renderResetButton("mobile");

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
    fireEvent.click(await screen.findByTestId(RESET_CONTEXT_CONFIRM_TEST_ID));

    await waitFor(() => {
      expect(mocks.request).toHaveBeenCalledTimes(1);
      expect(mocks.clearContextWindow).toHaveBeenCalledWith("session-1");
    });
    expect(document.activeElement).not.toBe(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
  });

  it("reports one close transition before dispatching the confirmed reset", async () => {
    const onConfirmationOpenChange = vi.fn();
    renderResetButton("mobile", onConfirmationOpenChange);

    fireEvent.click(screen.getByTestId(RESET_CONTEXT_BUTTON_TEST_ID));
    fireEvent.click(await screen.findByTestId(RESET_CONTEXT_CONFIRM_TEST_ID));

    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(1));
    expect(onConfirmationOpenChange.mock.calls.map(([open]) => open)).toEqual([true, false]);
  });
});
