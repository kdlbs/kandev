import { afterEach, describe, expect, it, vi } from "vitest";
import { buildTerminalWsUrl } from "./use-passthrough-terminal";
import { computeCanConnect, computeTerminalPaneState } from "./passthrough-terminal";
import type { TaskSessionState } from "@/app/office/tasks/[id]/types";
import { reconnectDelayMs, startReconnectLoop } from "./ws-reconnect";
import type { Terminal } from "@xterm/xterm";

const WS_BASE_URL = "ws://localhost:38429";

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
  const ENDED: TaskSessionState[] = ["COMPLETED", "FAILED", "CANCELLED"];
  const LIVE: TaskSessionState[] = ["CREATED", "STARTING", "RUNNING", "IDLE", "WAITING_FOR_INPUT"];

  it.each(ENDED)("refuses a shell terminal on a %s session", (state) => {
    expect(computeCanConnect("shell", "env-1", "session-1", state)).toBe(false);
  });

  it.each(LIVE)("still connects a shell terminal on a %s session", (state) => {
    expect(computeCanConnect("shell", "env-1", "session-1", state)).toBe(true);
  });

  it("connects when the session state is not known yet", () => {
    expect(computeCanConnect("shell", "env-1", "session-1", null)).toBe(true);
  });

  // Agent terminals resolve readiness server-side and must keep waiting.
  it.each(ENDED)("leaves agent terminals alone on a %s session", (state) => {
    expect(computeCanConnect("agent", "session-1", "session-1", state)).toBe(true);
  });
});

// The loading overlay is opaque and full-bleed. A dead terminal that leaves
// isConnected false therefore renders a "Connecting terminal…" spinner that is
// actively false, and hides anything written into the pane behind it. "Ended"
// has to outrank "connecting", or the user is told the opposite of the truth.
describe("computeTerminalPaneState", () => {
  it("reports ended instead of connecting when the session is over", () => {
    expect(computeTerminalPaneState("shell", "FAILED", false)).toBe("ended");
  });

  it("still reports ended when a socket happens to be open", () => {
    expect(computeTerminalPaneState("shell", "CANCELLED", true)).toBe("ended");
  });

  it("reports connecting only while a live session is not yet attached", () => {
    expect(computeTerminalPaneState("shell", "RUNNING", false)).toBe("connecting");
    expect(computeTerminalPaneState("shell", "RUNNING", true)).toBe("connected");
  });

  it("never reports ended for agent terminals", () => {
    expect(computeTerminalPaneState("agent", "FAILED", false)).toBe("connecting");
  });
});
