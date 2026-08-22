import { describe, expect, it } from "vitest";
import { agentProfileId } from "@/lib/types/ids";
import { isSelectableAgentProfile, toAgentProfileOption, toProfileCapability } from "./types";

describe("isSelectableAgentProfile", () => {
  it.each([
    ["enabled", true, true],
    ["disabled", false, false],
    ["legacy", undefined, true],
  ])("treats %s profiles correctly", (_label, enabled, expected) => {
    expect(isSelectableAgentProfile({ enabled })).toBe(expected);
  });

  it("excludes dynamic profiles when dynamic routing is disabled", () => {
    expect(isSelectableAgentProfile({ enabled: true, kind: "dynamic" }, false)).toBe(false);
    expect(isSelectableAgentProfile({ enabled: true, kind: "dynamic" }, true)).toBe(true);
    expect(isSelectableAgentProfile({ enabled: true, kind: "concrete" }, false)).toBe(true);
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

  it("preserves profile kind for picker filtering", () => {
    const option = toAgentProfileOption(agent, {
      id: agentProfileId("dynamic"),
      agentDisplayName: "Dynamic",
      name: "Frontier",
      kind: "dynamic",
    });
    expect(option.kind).toBe("dynamic");
  });

  it("preserves the owning agent capability fields", () => {
    const option = toAgentProfileOption(
      {
        ...agent,
        capability_status: "not_installed",
        capability_error: "CLI not found",
      },
      {
        id: agentProfileId("p3"),
        agentDisplayName: "Claude Code",
        name: "default",
      },
    );

    expect(option).toMatchObject({
      capability_status: "not_installed",
      capability_error: "CLI not found",
    });
  });
});

describe("toProfileCapability", () => {
  it("maps an unavailable agent to not_installed even when its cached status is ok", () => {
    expect(
      toProfileCapability({
        name: "claude-acp",
        display_name: "Claude",
        supports_mcp: false,
        installation_paths: [],
        available: false,
        capabilities: {
          supports_session_resume: false,
          supports_shell: false,
          supports_workspace_only: false,
        },
        model_config: {
          default_model: "default",
          available_models: [],
          supports_dynamic_models: false,
          status: "ok",
        },
        updated_at: "2026-07-26T10:00:00Z",
      }),
    ).toMatchObject({ capability_status: "not_installed" });
  });
});
