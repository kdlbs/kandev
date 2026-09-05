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
  inference_capable: true,
  ...overrides,
});

describe("utilityProfileEligibility", () => {
  it.each([
    ["enabled global profile", profile(), true],
    ["omitted enabled flag", profile({ enabled: undefined }), true],
    ["disabled profile", profile({ enabled: false }), false],
    ["CLI passthrough profile", profile({ cli_passthrough: true }), false],
    ["workspace profile", profile({ workspace_id: "workspace-1" }), false],
    ["non-inference profile", profile({ inference_capable: false }), false],
    ["profile with unknown inference support", profile({ inference_capable: undefined }), false],
  ])("returns %s = %s", (_label, candidate, expected) => {
    expect(utilityProfileEligibility(candidate)).toBe(expected);
  });

  it("allows workspace profiles when the picker explicitly includes them", () => {
    expect(utilityProfileEligibility(profile({ workspace_id: "workspace-1" }), true, true)).toBe(
      true,
    );
  });

  describe("with includeWorkspaceProfiles=true (Config Chat context)", () => {
    it.each([
      [
        "non-inference workspace profile",
        profile({ workspace_id: "ws-1", inference_capable: false }),
        true,
      ],
      ["non-inference global profile", profile({ inference_capable: false }), true],
      [
        "unknown inference workspace profile",
        profile({ workspace_id: "ws-1", inference_capable: undefined }),
        true,
      ],
    ])("returns %s = %s", (_label, candidate, expected) => {
      expect(utilityProfileEligibility(candidate, true, true)).toBe(expected);
    });

    it("allows CLI passthrough profiles", () => {
      expect(
        utilityProfileEligibility(
          profile({ workspace_id: "ws-1", cli_passthrough: true }),
          true,
          true,
        ),
      ).toBe(true);
    });

    it.each([["disabled", false]])("still rejects %s profile", (_label, enabled) => {
      expect(
        utilityProfileEligibility(
          profile({ workspace_id: "ws-1", cli_passthrough: true, enabled }),
          true,
          true,
        ),
      ).toBe(false);
    });
  });
});
