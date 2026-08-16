import { describe, expect, it } from "vitest";
import { utilityProfileEligibility } from "./utility-agent-profile-picker";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

const profile = (overrides: Partial<AgentProfileOption> = {}): AgentProfileOption => ({
  id: "profile-1",
  label: "Mock • mock-default",
  agent_id: "mock",
  agent_name: "Mock",
  cli_passthrough: false,
  enabled: true,
  ...overrides,
});

describe("utilityProfileEligibility", () => {
  it.each([
    ["enabled global profile", profile(), true],
    ["omitted enabled flag", profile({ enabled: undefined }), true],
    ["disabled profile", profile({ enabled: false }), false],
    ["CLI passthrough profile", profile({ cli_passthrough: true }), false],
    ["workspace profile", profile({ workspace_id: "workspace-1" }), false],
  ])("returns %s = %s", (_label, candidate, expected) => {
    expect(utilityProfileEligibility(candidate)).toBe(expected);
  });

  it("allows workspace profiles when the picker explicitly includes them", () => {
    expect(utilityProfileEligibility(profile({ workspace_id: "workspace-1" }), true, true)).toBe(
      true,
    );
  });
});
