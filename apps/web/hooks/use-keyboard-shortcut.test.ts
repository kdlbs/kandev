import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook, render, cleanup } from "@testing-library/react";
import { createElement } from "react";
import {
  useKeyboardShortcut,
  useKeyboardShortcutHandler,
  type KeyboardShortcutOptions,
} from "./use-keyboard-shortcut";
import { SHORTCUTS } from "@/lib/keyboard/constants";
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

    const preventDefaultListener = (event: KeyboardEvent) => {
      event.preventDefault();
    };
    window.addEventListener("keydown", preventDefaultListener, { capture: true });

    try {
      pressKey("k", { ctrlKey: true, shiftKey: true });

      expect(callback).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("keydown", preventDefaultListener, { capture: true });
    }
  });
});

const COMPOSITION_SIGNALS = [
  { name: "isComposing", isComposing: true, keyCode: 13 },
  { name: "legacy 229", isComposing: false, keyCode: 229 },
];
const MODIFIERS = [{ ctrlKey: true }, { metaKey: true }];
const INPUT_TEST_ID = "shortcut-input";

function ReactShortcutInput({
  callback,
  options,
}: {
  callback: () => void;
  options?: KeyboardShortcutOptions;
}) {
  const onKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, callback, options);
  return createElement("input", { onKeyDown, "data-testid": INPUT_TEST_ID });
}

// @covers AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.1, .2, .3
describe("composition native", () => {
  afterEach(() => cleanup());

  for (const signal of COMPOSITION_SIGNALS) {
    it.each(MODIFIERS)(
      `leaves ${signal.name} unconsumed for %j and accepts a later chord`,
      (mod) => {
        const input = document.createElement("input");
        document.body.append(input);
        const callback = vi.fn();
        const propagated = vi.fn();
        document.body.addEventListener("keydown", propagated);
        const { unmount } = renderHook(() =>
          useKeyboardShortcut(SHORTCUTS.SUBMIT, callback, {
            target: input,
            stopPropagation: true,
          }),
        );
        try {
          const event = pressKey("Enter", { ...signal, ...mod }, input);
          expect(callback).not.toHaveBeenCalled();
          expect(event.defaultPrevented).toBe(false);
          expect(propagated).toHaveBeenCalledTimes(1);
          const ordinary = pressKey("Enter", mod, input);
          expect(callback).toHaveBeenCalledTimes(1);
          expect(ordinary.defaultPrevented).toBe(true);
          expect(propagated).toHaveBeenCalledTimes(1);
        } finally {
          unmount();
          document.body.removeEventListener("keydown", propagated);
          input.remove();
        }
      },
    );
  }

  it("retains disabled, target, capture and consumption options", () => {
    const target = document.createElement("div");
    const input = document.createElement("input");
    target.append(input);
    const callback = vi.fn();
    const { rerender, unmount } = renderHook(
      ({ enabled }) =>
        useKeyboardShortcut(SHORTCUTS.SUBMIT, callback, {
          target,
          capture: true,
          enabled,
          preventDefault: false,
        }),
      { initialProps: { enabled: false } },
    );
    pressKey("Enter", { ctrlKey: true }, input);
    expect(callback).not.toHaveBeenCalled();
    rerender({ enabled: true });
    pressKey("Enter", { ctrlKey: true });
    expect(callback).not.toHaveBeenCalled();
    input.addEventListener("keydown", (event) => event.stopPropagation());
    const event = pressKey("Enter", { ctrlKey: true }, input);
    expect(callback).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(false);
    unmount();
    pressKey("Enter", { ctrlKey: true }, input);
    expect(callback).toHaveBeenCalledTimes(1);
  });
});

describe("composition React", () => {
  afterEach(() => cleanup());

  for (const signal of COMPOSITION_SIGNALS) {
    it.each(MODIFIERS)(
      `leaves ${signal.name} unconsumed for %j and accepts a later chord`,
      (mod) => {
        const callback = vi.fn();
        const { getByTestId } = render(
          createElement(ReactShortcutInput, { callback, options: { stopPropagation: true } }),
        );
        const input = getByTestId(INPUT_TEST_ID);
        const propagated = vi.fn();
        // React delegates at the container; document observes propagation after React.
        document.addEventListener("keydown", propagated);
        try {
          const event = pressKey("Enter", { ...signal, ...mod }, input);
          expect(callback).not.toHaveBeenCalled();
          expect(event.defaultPrevented).toBe(false);
          expect(propagated).toHaveBeenCalledTimes(1);
          const ordinary = pressKey("Enter", mod, input);
          expect(callback).toHaveBeenCalledTimes(1);
          expect(ordinary.defaultPrevented).toBe(true);
          expect(propagated).toHaveBeenCalledTimes(1);
        } finally {
          document.removeEventListener("keydown", propagated);
        }
      },
    );
  }

  it("retains disabled, nonmatching and already-prevented ordinary behavior", () => {
    const callback = vi.fn();
    const { getByTestId, rerender } = render(
      createElement(ReactShortcutInput, { callback, options: { enabled: false } }),
    );
    const input = getByTestId(INPUT_TEST_ID);
    pressKey("Enter", { ctrlKey: true }, input);
    expect(callback).not.toHaveBeenCalled();
    rerender(createElement(ReactShortcutInput, { callback, options: { preventDefault: false } }));
    pressKey("Enter", {}, input);
    pressKey("Enter", { ctrlKey: true, shiftKey: true }, input);
    pressKey("Enter", { metaKey: true, altKey: true }, input);
    expect(callback).not.toHaveBeenCalled();
    const ordinary = pressKey("Enter", { ctrlKey: true }, input);
    expect(ordinary.defaultPrevented).toBe(false);
    expect(callback).toHaveBeenCalledTimes(1);
    const event = new KeyboardEvent("keydown", {
      key: "Enter",
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    });
    event.preventDefault();
    input.dispatchEvent(event);
    expect(callback).toHaveBeenCalledTimes(2);
  });

  it("uses the latest callback after rerender", () => {
    const first = vi.fn();
    const latest = vi.fn();
    const { getByTestId, rerender } = render(
      createElement(ReactShortcutInput, { callback: first }),
    );
    rerender(createElement(ReactShortcutInput, { callback: latest }));
    pressKey("Enter", { ctrlKey: true }, getByTestId(INPUT_TEST_ID));
    expect(first).not.toHaveBeenCalled();
    expect(latest).toHaveBeenCalledTimes(1);
  });
});
