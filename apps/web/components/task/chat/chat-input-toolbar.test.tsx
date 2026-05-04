import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, act, cleanup } from "@testing-library/react";

afterEach(() => {
  cleanup();
});

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/keyboard-shortcut-tooltip", () => ({
  KeyboardShortcutTooltip: ({
    children,
    description,
  }: {
    children: React.ReactNode;
    description?: string;
  }) => (
    <>
      {children}
      {description ? <span>{description}</span> : null}
    </>
  ),
}));

import { ChatInputToolbar } from "./chat-input-toolbar";

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function renderToolbar(onCancel: () => void | Promise<void>) {
  return render(
    <ChatInputToolbar
      planModeEnabled={false}
      onPlanModeChange={() => {}}
      sessionId="s1"
      taskId="t1"
      taskDescription=""
      isAgentBusy
      isDisabled={false}
      isSending={false}
      onCancel={onCancel}
      onSubmit={() => {}}
      minimalToolbar
    />,
  );
}

// The cancel button must disable itself while a cancel request is in flight.
// Without this guard, an impatient user clicking it repeatedly while the agent
// tears down a long-running tool (Claude Monitor, etc.) sends N cancel requests
// to the backend, each producing a duplicate "Turn cancelled by user" message.
describe("ChatInputToolbar cancel button", () => {
  it("disables itself and blocks duplicate clicks while cancel is in flight", async () => {
    const { promise, resolve } = deferred<void>();
    const onCancel = vi.fn(() => promise);

    renderToolbar(onCancel);

    const button = screen.getByTestId("cancel-agent-button") as HTMLButtonElement;
    expect(button.disabled).toBe(false);

    fireEvent.click(button);
    // Click is processed synchronously; React then flushes the setState that
    // marks the button disabled. We assert post-flush.
    await act(async () => {});
    expect(button.disabled).toBe(true);
    expect(onCancel).toHaveBeenCalledTimes(1);

    // Subsequent clicks while the promise is pending must not call onCancel.
    fireEvent.click(button);
    fireEvent.click(button);
    fireEvent.click(button);
    expect(onCancel).toHaveBeenCalledTimes(1);

    // Once the in-flight cancel resolves, the button re-enables for retry
    // (rare, but possible if the first cancel returned an error).
    await act(async () => {
      resolve();
      await promise;
    });
    expect(button.disabled).toBe(false);
  });

  it("re-enables the button if onCancel rejects", async () => {
    const { promise, resolve } = deferred<void>();
    const onCancel = vi.fn(() => promise.then(() => Promise.reject(new Error("network"))));

    renderToolbar(onCancel);
    const button = screen.getByTestId("cancel-agent-button") as HTMLButtonElement;

    fireEvent.click(button);
    await act(async () => {});
    expect(button.disabled).toBe(true);

    await act(async () => {
      resolve();
      // Allow the rejected promise to settle inside the click handler.
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(button.disabled).toBe(false);
  });
});

describe("ChatInputToolbar submit button", () => {
  it("shows the setup-disabled reason while keeping the submit button disabled", () => {
    render(
      <ChatInputToolbar
        planModeEnabled={false}
        onPlanModeChange={() => {}}
        sessionId="s1"
        taskId="t1"
        taskDescription=""
        isAgentBusy={false}
        hasContent
        isDisabled
        submitDisabledReason="The agent is still being set up."
        isSending={false}
        onCancel={() => {}}
        onSubmit={() => {}}
        minimalToolbar
      />,
    );

    expect((screen.getByTestId("submit-message-button") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("The agent is still being set up.")).toBeTruthy();
  });
});
