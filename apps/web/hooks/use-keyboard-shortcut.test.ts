import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook, cleanup } from "@testing-library/react";
import { useKeyboardShortcut } from "./use-keyboard-shortcut";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";

const SHORTCUT: KeyboardShortcut = { key: "k", modifiers: { ctrlOrCmd: true, shift: true } };

function pressKey(
  key: string,
  init: KeyboardEventInit = {},
  target: EventTarget = window,
): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(event);
  return event;
}

describe("useKeyboardShortcut", () => {
  afterEach(() => cleanup());

  it("invokes the callback when the combo matches", () => {
    const callback = vi.fn();
    renderHook(() => useKeyboardShortcut(SHORTCUT, callback));

    pressKey("k", { ctrlKey: true, shiftKey: true });

    expect(callback).toHaveBeenCalledTimes(1);
  });

  it("does not invoke the callback when the event was already handled (defaultPrevented)", () => {
    // Regression: a bubble-phase useKeyboardShortcut listener must not act on
    // an event a capture-phase dispatcher (e.g. a plugin keybinding handler)
    // already handled and called preventDefault() on — otherwise the same
    // combo fires two actions instead of one.
    const callback = vi.fn();
    renderHook(() => useKeyboardShortcut(SHORTCUT, callback));

    window.addEventListener(
      "keydown",
      (event) => {
        event.preventDefault();
      },
      { capture: true },
    );

    pressKey("k", { ctrlKey: true, shiftKey: true });

    expect(callback).not.toHaveBeenCalled();
  });
});
