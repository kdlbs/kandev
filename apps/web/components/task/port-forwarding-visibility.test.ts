import { describe, expect, it } from "vitest";
import { canUsePortForwarding, isPortForwardingEnabled } from "./port-forwarding-visibility";

describe("port-forwarding visibility", () => {
  it.each([
    [{ port_forwarding_enabled: true }, true],
    [{ port_forwarding_enabled: false }, false],
    [{ port_forwarding_enabled: "true" }, false],
    [null, false],
    [undefined, false],
  ])("reads only an explicit true preference", (metadata, expected) => {
    expect(isPortForwardingEnabled(metadata)).toBe(expected);
  });

  it.each([
    ["session-1", true, true],
    ["session-1", false, false],
    [null, true, false],
    [undefined, true, false],
  ])("requires a session and ready agentctl", (sessionId, ready, expected) => {
    expect(canUsePortForwarding({ sessionId, isAgentctlReady: ready })).toBe(expected);
  });
});
