import { afterEach, describe, expect, it, vi } from "vitest";
import { buildTerminalWsUrl } from "./use-passthrough-terminal";
import {
  computeCanConnect,
  computeTerminalPaneState,
  isEnvironmentEnded,
  shouldGuardPassthroughEscape,
} from "./passthrough-terminal";
import { reconnectDelayMs, startReconnectLoop } from "./ws-reconnect";
import type { Terminal } from "@xterm/xterm";

const WS_BASE_URL = "ws://localhost:38429";

function keyboardEvent(key: string, target: EventTarget, modifiers: KeyboardEventInit = {}) {
  const event = new KeyboardEvent("keydown", { key, ...modifiers });
  Object.defineProperty(event, "target", { value: target });
  return event;
}

describe("shouldGuardPassthroughEscape", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("@covers AC-UI-QUICK-TERMINAL-001.11 claims unmodified Escape from xterm", () => {
    const textarea = document.createElement("textarea");
    document.body.append(textarea);
    textarea.focus();

    expect(shouldGuardPassthroughEscape(keyboardEvent("Escape", textarea), textarea)).toBe(true);
  });

  it("does not claim modified Escape or Escape from another focused control", () => {
    const textarea = document.createElement("textarea");
    const button = document.createElement("button");
    document.body.append(textarea, button);

    expect(
      shouldGuardPassthroughEscape(keyboardEvent("Escape", textarea, { ctrlKey: true }), textarea),
    ).toBe(false);

    button.focus();
    expect(shouldGuardPassthroughEscape(keyboardEvent("Escape", button), textarea)).toBe(false);
  });

  it("lets the dialog handle Escape after the passthrough connection ends", () => {
    const textarea = document.createElement("textarea");
    document.body.append(textarea);
    textarea.focus();

    expect(shouldGuardPassthroughEscape(keyboardEvent("Escape", textarea), textarea, false)).toBe(
      false,
    );
  });
});

describe("computeCanConnect", () => {
  it("lets the backend wait for an agent passthrough session that is not cached as ready", () => {
    expect(computeCanConnect("agent", "session-1", "session-1")).toBe(true);
  });
});

describe("reconnectDelayMs", () => {
  it("returns 300ms for attempt 0", () => {
    expect(reconnectDelayMs(0)).toBe(300);
  });

  it("doubles delay for each attempt", () => {
    expect(reconnectDelayMs(0)).toBe(300);
    expect(reconnectDelayMs(1)).toBe(600);
    expect(reconnectDelayMs(2)).toBe(1200);
    expect(reconnectDelayMs(3)).toBe(2400);
    expect(reconnectDelayMs(4)).toBe(4800);
  });

  it("caps at 5000ms", () => {
    expect(reconnectDelayMs(5)).toBe(5000);
  });

  it("caps attempt at 5 so high values stay at 5000ms", () => {
    expect(reconnectDelayMs(10)).toBe(5000);
    expect(reconnectDelayMs(100)).toBe(5000);
  });
});

describe("buildTerminalWsUrl", () => {
  it("routes shell terminals by task environment ID", () => {
    expect(
      buildTerminalWsUrl(WS_BASE_URL, {
        mode: "shell",
        environmentId: "env-1",
        terminalId: "terminal with spaces",
      }),
    ).toBe("ws://localhost:38429/terminal/environment/env-1?terminalId=terminal%20with%20spaces");
  });

  it("routes agent terminals by session ID", () => {
    expect(
      buildTerminalWsUrl(WS_BASE_URL, {
        mode: "agent",
        sessionId: "session-1",
      }),
    ).toBe("ws://localhost:38429/terminal/session/session-1?mode=agent");
  });
});

describe("startReconnectLoop", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("notifies disconnects so the terminal can show the reconnecting state", () => {
    vi.useFakeTimers();

    const onDisconnected = vi.fn();
    const connectWebSocket = vi.fn(({ onSocketClose }) => {
      onSocketClose({ code: 1006 } as CloseEvent);
    });

    const stop = startReconnectLoop({
      environmentId: "env-1",
      wsBaseUrl: WS_BASE_URL,
      mode: "shell",
      terminalId: "shell-1",
      label: undefined,
      terminal: { reset: vi.fn() } as unknown as Terminal,
      fitAndResize: vi.fn(),
      wsRef: { current: null },
      attachAddonRef: { current: null },
      onConnected: vi.fn(),
      onDisconnected,
      connectWebSocket,
    });

    vi.advanceTimersByTime(150);

    expect(connectWebSocket).toHaveBeenCalledTimes(1);
    expect(onDisconnected).toHaveBeenCalledTimes(1);
    stop();
  });

  it("resets xterm before each connection so replayed PTY buffers do not append duplicates", () => {
    vi.useFakeTimers();

    const terminal = { reset: vi.fn() } as unknown as Terminal;
    let closes = 0;
    const connectWebSocket = vi.fn(({ onSocketClose }) => {
      if (closes < 1) {
        closes += 1;
        onSocketClose({ code: 1006 } as CloseEvent);
      }
    });

    const stop = startReconnectLoop({
      environmentId: "env-1",
      wsBaseUrl: WS_BASE_URL,
      mode: "shell",
      terminalId: "shell-1",
      label: undefined,
      terminal,
      fitAndResize: vi.fn(),
      wsRef: { current: null },
      attachAddonRef: { current: null },
      onConnected: vi.fn(),
      onDisconnected: vi.fn(),
      connectWebSocket,
    });

    vi.advanceTimersByTime(150);

    expect(connectWebSocket).toHaveBeenCalledTimes(1);
    expect(terminal.reset).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(300);

    expect(connectWebSocket).toHaveBeenCalledTimes(2);
    expect(terminal.reset).toHaveBeenCalledTimes(2);
    stop();
  });
});

// The env handler refuses a shell on an ended session, identically every time.
// Opening the socket anyway only restarts the 5s retry timer.
describe("computeCanConnect on ended sessions", () => {
  it("refuses a shell terminal for an ended environment", () => {
    expect(computeCanConnect("shell", "env-1", "session-1", true)).toBe(false);
  });

  it("still connects a shell terminal for a live environment", () => {
    expect(computeCanConnect("shell", "env-1", "session-1", false)).toBe(true);
  });

  it("connects when nothing is known about the environment", () => {
    expect(computeCanConnect("shell", "env-1", "session-1")).toBe(true);
  });

  // Agent terminals resolve readiness server-side and must keep waiting.
  it("leaves agent terminals alone even for a dead environment", () => {
    expect(computeCanConnect("agent", "session-1", "session-1", true)).toBe(true);
  });
});

// Shell terminals are environment-scoped, but useEnvironmentSessionId caches
// the first session seen for an environment. Gating on that cached session's
// state would refuse a shell on a live environment as soon as a second session
// reused it — worse than the loop this exists to stop.
describe("isEnvironmentEnded", () => {
  it("is not ended while any session sharing the environment is live", () => {
    expect(
      isEnvironmentEnded(
        "env-1",
        { "session-a": "COMPLETED", "session-b": "RUNNING" },
        { "session-a": "env-1", "session-b": "env-1" },
      ),
    ).toBe(false);
  });

  it("is ended only when every session for the environment is terminal", () => {
    expect(
      isEnvironmentEnded(
        "env-1",
        { "session-a": "FAILED", "session-b": "CANCELLED" },
        { "session-a": "env-1", "session-b": "env-1" },
      ),
    ).toBe(true);
  });

  it("ignores sessions belonging to other environments", () => {
    expect(
      isEnvironmentEnded(
        "env-1",
        { "session-a": "FAILED", "session-b": "RUNNING" },
        { "session-a": "env-1", "session-b": "env-2" },
      ),
    ).toBe(true);
  });

  it("never refuses without positive knowledge", () => {
    expect(isEnvironmentEnded("env-1", {}, {})).toBe(false);
    expect(isEnvironmentEnded(null, { "session-a": "FAILED" }, { "session-a": "env-1" })).toBe(
      false,
    );
    expect(isEnvironmentEnded("env-1", {}, { "session-a": "env-1" })).toBe(false);
  });
});

// The loading overlay is opaque and full-bleed. A dead terminal that leaves
// isConnected false therefore renders a "Connecting terminal…" spinner that is
// actively false, and hides anything written into the pane behind it. "Ended"
// has to outrank "connecting", or the user is told the opposite of the truth.
describe("computeTerminalPaneState", () => {
  it("reports ended instead of connecting when the session is over", () => {
    expect(computeTerminalPaneState("shell", true, false)).toBe("ended");
  });

  it("still reports ended when a socket happens to be open", () => {
    expect(computeTerminalPaneState("shell", true, true)).toBe("ended");
  });

  it("reports connecting only while a live session is not yet attached", () => {
    expect(computeTerminalPaneState("shell", false, false)).toBe("connecting");
    expect(computeTerminalPaneState("shell", false, true)).toBe("connected");
  });

  it("never reports ended for agent terminals", () => {
    expect(computeTerminalPaneState("agent", true, false)).toBe("connecting");
  });

  // Before the session lands, state is null. Treating that as ended would put
  // a permanent "session has ended" over a terminal that is merely starting.
  it("treats an unknown session state as live", () => {
    expect(computeTerminalPaneState("shell", false, false)).toBe("connecting");
    expect(computeTerminalPaneState("shell", false, true)).toBe("connected");
  });
});
