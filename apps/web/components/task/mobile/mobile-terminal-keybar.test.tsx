import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { MobileTerminalKeybar } from "./mobile-terminal-keybar";

const sendMock = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ send: sendMock }),
}));

vi.mock("@/hooks/use-visual-viewport-offset", () => ({
  useVisualViewportOffset: () => ({ bottomOffset: 0, keyboardOpen: false }),
}));

const CTRL_KEY = "keybar-key-ctrl";
const LETTER_C = "keybar-key-letter-c";
const ARIA_PRESSED = "aria-pressed";

function tap(testId: string): void {
  fireEvent.click(screen.getByTestId(testId));
}

function lastData(): string | undefined {
  const call = sendMock.mock.calls.at(-1)?.[0] as { payload?: { data?: string } } | undefined;
  return call?.payload?.data;
}

function allData(): string[] {
  return sendMock.mock.calls.map((c) => (c[0] as { payload: { data: string } }).payload.data);
}

beforeEach(() => {
  sendMock.mockReset();
  cleanup();
});

describe("MobileTerminalKeybar visibility", () => {
  it("renders when visible and has a sessionId", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    expect(screen.getByTestId("mobile-terminal-keybar")).toBeDefined();
  });

  it("renders nothing when not visible", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={false} />);
    expect(screen.queryByTestId("mobile-terminal-keybar")).toBeNull();
  });

  it("renders nothing without a sessionId", () => {
    render(<MobileTerminalKeybar sessionId={null} visible={true} />);
    expect(screen.queryByTestId("mobile-terminal-keybar")).toBeNull();
  });
});

describe("MobileTerminalKeybar single-key emission", () => {
  it("sends \\x1b on Esc tap", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    tap("keybar-key-esc");
    expect(lastData()).toBe("\x1b");
  });

  it("sends \\t on Tab tap", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    tap("keybar-key-tab");
    expect(lastData()).toBe("\t");
  });

  it("sends arrow escape sequences", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    tap("keybar-key-up");
    expect(lastData()).toBe("\x1b[A");
    tap("keybar-key-down");
    expect(lastData()).toBe("\x1b[B");
    tap("keybar-key-left");
    expect(lastData()).toBe("\x1b[D");
    tap("keybar-key-right");
    expect(lastData()).toBe("\x1b[C");
  });

  it("sends Home / End / PgUp / PgDn", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    tap("keybar-key-home");
    expect(lastData()).toBe("\x01");
    tap("keybar-key-end");
    expect(lastData()).toBe("\x05");
    tap("keybar-key-pageup");
    expect(lastData()).toBe("\x1b[5~");
    tap("keybar-key-pagedown");
    expect(lastData()).toBe("\x1b[6~");
  });

  it("sends literal symbols", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    tap("keybar-key-pipe");
    expect(lastData()).toBe("|");
    tap("keybar-key-tilde");
    expect(lastData()).toBe("~");
  });
});

describe("MobileTerminalKeybar dedicated Ctrl+C / Ctrl+D", () => {
  it("Ctrl+C button sends \\x03 in one tap", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    tap("keybar-key-ctrl-c");
    expect(lastData()).toBe("\x03");
  });

  it("Ctrl+D button sends \\x04 in one tap", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    tap("keybar-key-ctrl-d");
    expect(lastData()).toBe("\x04");
  });
});

describe("MobileTerminalKeybar sticky Ctrl state machine", () => {
  it("latches Ctrl then chords the next letter and auto-releases", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);

    const ctrl = screen.getByTestId(CTRL_KEY);
    expect(ctrl.getAttribute(ARIA_PRESSED)).toBe("false");

    tap(CTRL_KEY);
    expect(ctrl.getAttribute(ARIA_PRESSED)).toBe("true");

    tap(LETTER_C);
    expect(lastData()).toBe("\x03");
    expect(ctrl.getAttribute(ARIA_PRESSED)).toBe("false");
  });

  it("double-tap makes Ctrl sticky; chord bytes keep flowing", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    const ctrl = screen.getByTestId(CTRL_KEY);

    tap(CTRL_KEY);
    tap(CTRL_KEY);
    expect(ctrl.getAttribute(ARIA_PRESSED)).toBe("true");
    expect(ctrl.getAttribute("data-sticky")).toBe("true");

    tap(LETTER_C);
    tap("keybar-key-letter-d");
    const data = allData();
    expect(data).toContain("\x03");
    expect(data).toContain("\x04");
    expect(ctrl.getAttribute(ARIA_PRESSED)).toBe("true");
  });

  it("third tap releases sticky", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    const ctrl = screen.getByTestId(CTRL_KEY);

    tap(CTRL_KEY);
    tap(CTRL_KEY);
    tap(CTRL_KEY);
    expect(ctrl.getAttribute(ARIA_PRESSED)).toBe("false");
  });

  it("does not expose letter keys until Ctrl is active", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    expect(screen.queryByTestId("keybar-key-letter-c")).toBeNull();
  });
});

describe("MobileTerminalKeybar accessibility", () => {
  it("every key has a non-empty aria-label", () => {
    render(<MobileTerminalKeybar sessionId="s1" visible={true} />);
    const buttons = document.querySelectorAll('[data-testid^="keybar-key-"]');
    expect(buttons.length).toBeGreaterThan(0);
    buttons.forEach((b) => {
      expect(b.getAttribute("aria-label")?.length ?? 0).toBeGreaterThan(0);
    });
  });
});
