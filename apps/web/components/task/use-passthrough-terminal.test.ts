import { describe, expect, it } from "vitest";
import { buildTerminalWsUrl } from "./use-passthrough-terminal";
import { reconnectDelayMs } from "./ws-reconnect";

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
