import { describe, expect, it } from "vitest";

import { resolveAgentStatusConfig } from "./agent-status";

describe("resolveAgentStatusConfig", () => {
  it("keeps the status spinner after the foreground settles while background work remains", () => {
    expect(resolveAgentStatusConfig("WAITING_FOR_INPUT", true)).toMatchObject({
      labelKey: "task:backgroundWorkIsRunning",
      icon: "spinner",
    });
  });

  it("does not invent background work for an ordinary settled session", () => {
    expect(resolveAgentStatusConfig("WAITING_FOR_INPUT", false)).toMatchObject({ icon: null });
  });

  it("keeps foreground running on the established status", () => {
    expect(resolveAgentStatusConfig("RUNNING", true)).toMatchObject({
      labelKey: "task:agentIsRunning",
      icon: "spinner",
    });
  });

  it("reads background when a RUNNING foreground turn has yielded to background work", () => {
    expect(resolveAgentStatusConfig("RUNNING", true, true)).toMatchObject({
      labelKey: "task:backgroundWorkIsRunning",
      icon: "spinner",
    });
  });

  it("keeps the generating label for a RUNNING turn without background work", () => {
    expect(resolveAgentStatusConfig("RUNNING", true, false)).toMatchObject({
      labelKey: "task:agentIsRunning",
    });
  });
});
