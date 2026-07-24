import { describe, expect, it } from "vitest";

import { resolveAgentStatusConfig } from "./agent-status";

describe("resolveAgentStatusConfig", () => {
  it("keeps the status spinner after the foreground settles while background work remains", () => {
    expect(resolveAgentStatusConfig("WAITING_FOR_INPUT", true)).toMatchObject({
      label: "Background work is running",
      icon: "spinner",
    });
  });

  it("does not invent background work for an ordinary settled session", () => {
    expect(resolveAgentStatusConfig("WAITING_FOR_INPUT", false)).toMatchObject({ icon: null });
  });

  it("keeps foreground running on the established status", () => {
    expect(resolveAgentStatusConfig("RUNNING", true)).toMatchObject({
      label: "Agent is running",
      icon: "spinner",
    });
  });
});
