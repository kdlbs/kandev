import { describe, it, expect } from "vitest";
import { normalizeAgentProfile, toAgentProfilePayload } from "./agent-profile-normalize";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";

const sampleEnvVar = { key: "ANTHROPIC_BASE_URL", value: "https://api.example" };
const SAMPLE_ID = "p1";
const SAMPLE_PREFIX = "greywall --";
const WORKSPACE_ID = "workspace-1";

const snakeCaseWirePayload = {
  id: SAMPLE_ID,
  agent_id: "claude",
  name: "default",
  agent_display_name: "Claude Code",
  model: "claude-sonnet-4-5",
  mode: "acp",
  allow_indexing: true,
  auto_approve: false,
  cli_flags: [{ flag: "--verbose", description: "v", enabled: true }],
  env_vars: [sampleEnvVar],
  cli_passthrough: false,
  enabled: false,
  workspace_id: WORKSPACE_ID,
  user_modified: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
};

const expectedCamelCaseProfile = {
  id: SAMPLE_ID,
  name: "default",
  agentId: "claude",
  agentDisplayName: "Claude Code",
  model: "claude-sonnet-4-5",
  fallbackModel: "",
  autoFallback: false,
  mode: "acp",
  allowIndexing: true,
  autoApprove: false,
  cliFlags: [{ flag: "--verbose", description: "v", enabled: true }],
  envVars: [sampleEnvVar],
  cliPassthrough: false,
  enabled: false,
  workspaceId: WORKSPACE_ID,
  userModified: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-02T00:00:00Z",
};

describe("normalizeAgentProfile", () => {
  it("converts snake_case wire payload to canonical camelCase", () => {
    const result = normalizeAgentProfile(snakeCaseWirePayload);
    expect(result).toEqual(expectedCamelCaseProfile);
  });

  it("falls back to safe defaults for missing fields", () => {
    const result = normalizeAgentProfile({ id: "x", name: "y" });
    expect(result.cliFlags).toEqual([]);
    expect(result.envVars).toEqual([]);
    expect(result.cliPassthrough).toBe(false);
    expect(result.allowIndexing).toBe(false);
    expect(result.autoApprove).toBe(false);
    expect(result.agentDisplayName).toBe("");
    // Legacy payloads without the flag are treated as enabled.
    expect(result.enabled).toBe(true);
  });

  it("preserves the office workspace scope when it is present", () => {
    expect(normalizeAgentProfile({ id: "x", workspace_id: WORKSPACE_ID }).workspaceId).toBe(
      WORKSPACE_ID,
    );
  });

  it("accepts already-camelCase input", () => {
    const result = normalizeAgentProfile({
      id: SAMPLE_ID,
      name: "default",
      agentId: "codex",
      cliPassthrough: true,
    });
    expect(result.agentId).toBe("codex");
    expect(result.cliPassthrough).toBe(true);
  });

  it("maps command_prefix to commandPrefix", () => {
    const result = normalizeAgentProfile({
      id: SAMPLE_ID,
      name: "default",
      command_prefix: SAMPLE_PREFIX,
    });
    expect(result.commandPrefix).toBe(SAMPLE_PREFIX);
  });

  it("accepts already-camelCase commandPrefix", () => {
    const result = normalizeAgentProfile({
      id: SAMPLE_ID,
      name: "default",
      commandPrefix: SAMPLE_PREFIX,
    });
    expect(result.commandPrefix).toBe(SAMPLE_PREFIX);
  });

  it("leaves commandPrefix undefined when absent", () => {
    const result = normalizeAgentProfile({ id: "x", name: "y" });
    expect(result.commandPrefix).toBeUndefined();
  });

  it("ignores a non-string command_prefix instead of propagating the raw value", () => {
    const nullResult = normalizeAgentProfile({ id: "x", name: "y", command_prefix: null });
    expect(nullResult.commandPrefix).toBeUndefined();

    const numberResult = normalizeAgentProfile({ id: "x", name: "y", command_prefix: 42 });
    expect(numberResult.commandPrefix).toBeUndefined();

    const objectResult = normalizeAgentProfile({
      id: "x",
      name: "y",
      commandPrefix: { flag: "greywall" },
    });
    expect(objectResult.commandPrefix).toBeUndefined();
  });

  it("maps fallback_model/auto_fallback from the wire shape", () => {
    const result = normalizeAgentProfile({
      id: SAMPLE_ID,
      name: "default",
      fallback_model: "gpt-5",
      auto_fallback: true,
    });
    expect(result.fallbackModel).toBe("gpt-5");
    expect(result.autoFallback).toBe(true);
  });

  it("defaults fallbackModel to empty and autoFallback to false when absent", () => {
    const result = normalizeAgentProfile({ id: SAMPLE_ID, name: "default" });
    expect(result.fallbackModel).toBe("");
    expect(result.autoFallback).toBe(false);
  });
});

describe("toAgentProfilePayload", () => {
  it("converts canonical camelCase back to snake_case wire shape", () => {
    const payload = toAgentProfilePayload({
      id: toAgentProfileId(SAMPLE_ID),
      agentId: "claude",
      name: "default",
      cliPassthrough: false,
      cliFlags: [],
      envVars: [sampleEnvVar],
    });
    expect(payload).toEqual({
      id: SAMPLE_ID,
      agent_id: "claude",
      name: "default",
      cli_passthrough: false,
      cli_flags: [],
      env_vars: [sampleEnvVar],
    });
  });

  it("omits undefined fields rather than emitting nullish keys", () => {
    const payload = toAgentProfilePayload({ id: toAgentProfileId(SAMPLE_ID), name: "x" });
    expect(payload).toEqual({ id: SAMPLE_ID, name: "x" });
    expect("agent_id" in payload).toBe(false);
  });

  it("maps commandPrefix to command_prefix", () => {
    const payload = toAgentProfilePayload({
      id: toAgentProfileId(SAMPLE_ID),
      name: "default",
      commandPrefix: SAMPLE_PREFIX,
    });
    expect(payload).toEqual({ id: SAMPLE_ID, name: "default", command_prefix: SAMPLE_PREFIX });
  });

  it("maps fallbackModel/autoFallback to fallback_model/auto_fallback", () => {
    const payload = toAgentProfilePayload({
      id: toAgentProfileId(SAMPLE_ID),
      name: "default",
      fallbackModel: "gpt-5",
      autoFallback: true,
    });
    expect(payload).toEqual({
      id: SAMPLE_ID,
      name: "default",
      fallback_model: "gpt-5",
      auto_fallback: true,
    });
  });

  it("maps enabled and omits it when undefined", () => {
    const disabled = toAgentProfilePayload({ id: toAgentProfileId(SAMPLE_ID), enabled: false });
    expect(disabled.enabled).toBe(false);
    const omitted = toAgentProfilePayload({ id: toAgentProfileId(SAMPLE_ID) });
    expect("enabled" in omitted).toBe(false);
  });
});
