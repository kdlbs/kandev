import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { MobileTerminalKeybar } from "./mobile-terminal-keybar";
import { useShellModifiersStore } from "@/lib/terminal/shell-modifiers";

const sendMock = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ send: sendMock }),
}));

vi.mock("@/hooks/use-visual-viewport-offset", () => ({
  useVisualViewportOffset: () => ({ bottomOffset: 0, keyboardOpen: false, viewportBottom: 0 }),
}));

const KEYBAR_ROOT = "mobile-terminal-keybar";
const CTRL_KEY = "keybar-key-ctrl";
const SHIFT_KEY = "keybar-key-shift";
const ESC_KEY = "keybar-key-esc";
const ARIA_PRESSED = "aria-pressed";
const SESSION_ID = "s1";

function tap(testId: string): void {
  fireEvent.click(screen.getByTestId(testId));
}

function lastData(): string | undefined {
  const call = sendMock.mock.calls.at(-1)?.[0] as { payload?: { data?: string } } | undefined;
  return call?.payload?.data;
}

beforeEach(() => {
  sendMock.mockReset();
  useShellModifiersStore.getState().reset();
  cleanup();
});

describe("MobileTerminalKeybar visibility", () => {
  it("renders when visible and has a sessionId", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    expect(screen.getByTestId(KEYBAR_ROOT)).toBeDefined();
  });

  it("renders nothing when not visible", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={false} />);
    expect(screen.queryByTestId(KEYBAR_ROOT)).toBeNull();
  });

  it("renders nothing without a sessionId", () => {
    render(<MobileTerminalKeybar sessionId={null} visible={true} />);
    expect(screen.queryByTestId(KEYBAR_ROOT)).toBeNull();
  });
});

describe("MobileTerminalKeybar single-key emission", () => {
  it("sends \\x1b on Esc tap", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap(ESC_KEY);
    expect(lastData()).toBe("\x1b");
  });

  it("sends \\t on Tab tap", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap("keybar-key-tab");
    expect(lastData()).toBe("\t");
  });

  it("sends arrow escape sequences", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap("keybar-key-up");
    expect(lastData()).toBe("\x1b[A");
    tap("keybar-key-down");
    expect(lastData()).toBe("\x1b[B");
    tap("keybar-key-left");
    expect(lastData()).toBe("\x1b[D");
    tap("keybar-key-right");
    expect(lastData()).toBe("\x1b[C");
  });

  it("sends literal symbols", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap("keybar-key-pipe");
    expect(lastData()).toBe("|");
    tap("keybar-key-tilde");
    expect(lastData()).toBe("~");
  });
});

describe("MobileTerminalKeybar dedicated Ctrl+C / Ctrl+D", () => {
  it("Ctrl+C button sends \\x03 in one tap", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap("keybar-key-ctrl-c");
    expect(lastData()).toBe("\x03");
  });

  it("Ctrl+D button sends \\x04 in one tap", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap("keybar-key-ctrl-d");
    expect(lastData()).toBe("\x04");
  });
});

describe("MobileTerminalKeybar Ctrl modifier", () => {
  it("tapping Ctrl latches the modifier in the shared store", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    const ctrl = screen.getByTestId(CTRL_KEY);
    expect(ctrl.getAttribute(ARIA_PRESSED)).toBe("false");

    tap(CTRL_KEY);
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: true, sticky: false });
    expect(ctrl.getAttribute(ARIA_PRESSED)).toBe("true");
  });

  it("double-tap makes Ctrl sticky", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    const ctrl = screen.getByTestId(CTRL_KEY);
    tap(CTRL_KEY);
    tap(CTRL_KEY);
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: true, sticky: true });
    expect(ctrl.getAttribute("data-sticky")).toBe("true");
  });

  it("third tap clears Ctrl entirely", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap(CTRL_KEY);
    tap(CTRL_KEY);
    tap(CTRL_KEY);
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: false, sticky: false });
  });

  it("tapping a bar key with Ctrl latched chords it and auto-releases", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap(CTRL_KEY);
    // Tab becomes Ctrl+I (\x09) when ctrl is active — but the chord transform
    // only runs on single-char data. Use a symbol key which is single-char.
    tap("keybar-key-pipe");
    expect(lastData()).toBe("|"); // "|" transforms to itself (non-letter)
    // Ctrl was latched (non-sticky), so it clears after consumption.
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: false, sticky: false });
  });

  it("does not render in-bar letter buttons — OS keyboard drives letters", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    expect(screen.queryByTestId("keybar-key-letter-c")).toBeNull();
    expect(screen.queryByTestId("keybar-key-letter-a")).toBeNull();
  });
});

describe("MobileTerminalKeybar Shift modifier", () => {
  it("exposes a Shift toggle that latches in the shared store", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    const shift = screen.getByTestId(SHIFT_KEY);
    expect(shift.getAttribute(ARIA_PRESSED)).toBe("false");

    tap(SHIFT_KEY);
    expect(useShellModifiersStore.getState().shift).toEqual({ latched: true, sticky: false });
    expect(shift.getAttribute(ARIA_PRESSED)).toBe("true");
  });

  it("Shift+Tab from the bar emits CSI Z (reverse tab)", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    tap(SHIFT_KEY);
    tap("keybar-key-tab");
    expect(lastData()).toBe("\x1b[Z");
    expect(useShellModifiersStore.getState().shift).toEqual({ latched: false, sticky: false });
  });
});

describe("MobileTerminalKeybar iOS focus retention", () => {
  function mountXtermTextarea(): HTMLTextAreaElement {
    const ta = document.createElement("textarea");
    ta.className = "xterm-helper-textarea";
    document.body.appendChild(ta);
    return ta;
  }

  it("refocuses the xterm textarea on every key tap so the OS keyboard stays up", () => {
    const ta = mountXtermTextarea();
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);

    tap(ESC_KEY);
    expect(document.activeElement).toBe(ta);

    (document.activeElement as HTMLElement).blur();
    tap(CTRL_KEY);
    expect(document.activeElement).toBe(ta);

    (document.activeElement as HTMLElement).blur();
    tap(SHIFT_KEY);
    expect(document.activeElement).toBe(ta);

    (document.activeElement as HTMLElement).blur();
    tap("keybar-key-ctrl-c");
    expect(document.activeElement).toBe(ta);

    ta.remove();
  });
});

describe("MobileTerminalKeybar iOS keyboard retention", () => {
  it("calls preventDefault on pointerdown so the OS keyboard stays open", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    const esc = screen.getByTestId(ESC_KEY);

    const pointerEvent = new Event("pointerdown", { bubbles: true, cancelable: true });
    const mouseEvent = new Event("mousedown", { bubbles: true, cancelable: true });
    esc.dispatchEvent(pointerEvent);
    esc.dispatchEvent(mouseEvent);

    expect(pointerEvent.defaultPrevented).toBe(true);
    expect(mouseEvent.defaultPrevented).toBe(true);
  });

  it("still fires onClick after preventing focus transfer", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    const esc = screen.getByTestId(ESC_KEY);
    esc.dispatchEvent(new Event("pointerdown", { bubbles: true, cancelable: true }));
    fireEvent.click(esc);
    expect(lastData()).toBe("\x1b");
  });
});

describe("MobileTerminalKeybar positioning", () => {
  it("anchors via top (not bottom) when the keyboard is open — iOS fixed-positioning fix", async () => {
    // Re-mock the hook to report a keyboard-open state.
    vi.doMock("@/hooks/use-visual-viewport-offset", () => ({
      useVisualViewportOffset: () => ({
        bottomOffset: 300,
        keyboardOpen: true,
        viewportBottom: 500,
      }),
    }));
    vi.resetModules();
    const { MobileTerminalKeybar: Reloaded } = await import("./mobile-terminal-keybar");
    render(<Reloaded sessionId={SESSION_ID} visible={true} />);
    const bar = screen.getByTestId(KEYBAR_ROOT);
    expect(bar.style.top).toBeTruthy();
    expect(bar.style.bottom).toBe("auto");
  });
});

describe("MobileTerminalKeybar accessibility", () => {
  it("every key has a non-empty aria-label", () => {
    render(<MobileTerminalKeybar sessionId={SESSION_ID} visible={true} />);
    const buttons = document.querySelectorAll('[data-testid^="keybar-key-"]');
    expect(buttons.length).toBeGreaterThan(0);
    buttons.forEach((b) => {
      expect(b.getAttribute("aria-label")?.length ?? 0).toBeGreaterThan(0);
    });
  });
});
