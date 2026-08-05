import { describe, expect, it } from "vitest";
import { agentProfileId } from "@/lib/types/ids";
import { isSelectableAgentProfile, toAgentProfileOption } from "./types";

describe("isSelectableAgentProfile", () => {
  it.each([
    ["enabled", true, true],
    ["disabled", false, false],
    ["legacy", undefined, true],
  ])("treats %s profiles correctly", (_label, enabled, expected) => {
    expect(isSelectableAgentProfile({ enabled })).toBe(expected);
  });
});

describe("toAgentProfileOption", () => {
  const agent = {
    id: "a1",
    name: "claude-acp",
    capability_status: undefined,
    capability_error: undefined,
  };

  it("maps enabled from the profile and defaults to true when absent", () => {
    const enabled = toAgentProfileOption(agent, {
      id: agentProfileId("p1"),
      agentDisplayName: "Claude Code",
      name: "default",
      enabled: false,
    });
    expect(enabled.enabled).toBe(false);

    const legacy = toAgentProfileOption(agent, {
      id: agentProfileId("p2"),
      agentDisplayName: "Claude Code",
      name: "legacy",
    });
    expect(legacy.enabled).toBe(true);
  });
});
