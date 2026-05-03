import { afterEach, describe, expect, it, vi } from "vitest";
import { buildTerminalWsUrl } from "./use-passthrough-terminal";
import { reconnectDelayMs, startReconnectLoop } from "./ws-reconnect";
import type { Terminal } from "@xterm/xterm";

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
      buildTerminalWsUrl("ws://localhost:38429", {
        mode: "shell",
        environmentId: "env-1",
        terminalId: "terminal with spaces",
      }),
    ).toBe("ws://localhost:38429/terminal/environment/env-1?terminalId=terminal%20with%20spaces");
  });

  it("routes agent terminals by session ID", () => {
    expect(
      buildTerminalWsUrl("ws://localhost:38429", {
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
      wsBaseUrl: "ws://localhost:38429",
      mode: "shell",
      terminalId: "shell-1",
      label: undefined,
      terminal: {} as Terminal,
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
});
