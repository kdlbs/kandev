import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { withSiblings } from "@/lib/ws/handlers/task-primary-focus.test-helpers";
import { clearSessionTabUserActivationIntentsForTest } from "./session-tab-activation-intent";
import { useSessionTabUserActivationIntent } from "./use-session-tab-user-activation";

const harness = vi.hoisted(() => ({ store: null as StoreApi<AppState> | null }));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => harness.store }));

function Tab() {
  const handlers = useSessionTabUserActivationIntent("next", { isActive: true });
  return (
    <div
      data-testid="activation-tab"
      onPointerDownCapture={handlers.handlePointerDownCapture}
      onKeyDownCapture={handlers.handleKeyDownCapture}
    >
      <span>Session</span>
      <button data-testid="nested-close">Close</button>
    </div>
  );
}

beforeEach(() => {
  harness.store = withSiblings("FAILED");
  harness.store.getState().clearActiveSession();
});
afterEach(() => {
  cleanup();
  clearSessionTabUserActivationIntentsForTest();
  harness.store = null;
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([
  "pointer",
  "Enter",
  " ",
  "right-button",
  "Escape",
  "nested-pointer",
  "nested-key",
  "current",
] as const)("claims an already-visible implicit tab only for valid %s input", (input) => {
  if (input === "current") harness.store!.getState().setActiveSessionAuto("t1", "next");
  render(<Tab />);
  const target = screen.getByTestId(
    input.startsWith("nested-") ? "nested-close" : "activation-tab",
  );
  if (
    input === "pointer" ||
    input === "right-button" ||
    input === "nested-pointer" ||
    input === "current"
  ) {
    fireEvent.pointerDown(target, { button: input === "right-button" ? 2 : 0 });
  } else {
    fireEvent.keyDown(target, { key: input === "nested-key" ? "Enter" : input });
  }
  const claimed = input === "pointer" || input === "Enter" || input === " ";
  expect(harness.store!.getState().tasks.activeSessionId).toBe(
    claimed || input === "current" ? "next" : null,
  );
  expect(harness.store!.getState().tasks.pinnedSessionId).toBe(claimed ? "next" : null);
});
