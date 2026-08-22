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

// eslint-disable-next-line max-lines-per-function -- keeps the canonical wire-shape matrix together.
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

  it("normalizes a dynamic profile document and preserves candidate order", () => {
    const result = normalizeAgentProfile({
      id: "dynamic-profile",
      kind: "dynamic",
      dynamic: {
        version: 4,
        candidates: [
          {
            position: 0,
            execution_profile_id: "primary",
            enabled: true,
            policies: {
              version: 1,
              transient: {
                retry: { enabled: true, max_retries: 2, initial_interval_seconds: 5 },
                wait_for_reset: { enabled: true, max_wait_seconds: 300 },
                on_exhausted: "skip",
              },
              hard: {
                retry: { enabled: false, max_retries: 0, initial_interval_seconds: 0 },
                wait_for_reset: { enabled: false, max_wait_seconds: 0 },
                on_exhausted: "stop",
              },
            },
          },
        ],
      },
    });
    expect(result.kind).toBe("dynamic");
    expect(result.dynamic).toEqual({
      version: 4,
      candidates: [
        {
          position: 0,
          executionProfileId: "primary",
          enabled: true,
          policies: {
            version: 1,
            transient: {
              retry: { enabled: true, maxRetries: 2, initialIntervalSeconds: 5 },
              waitForReset: { enabled: true, maxWaitSeconds: 300 },
              onExhausted: "skip",
            },
            hard: {
              retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
              waitForReset: { enabled: false, maxWaitSeconds: 0 },
              onExhausted: "stop",
            },
          },
        },
      ],
    });
  });

  it("normalizes legacy dynamic rules into both error classes", () => {
    const result = normalizeAgentProfile({
      id: "dynamic-profile",
      kind: "dynamic",
      dynamic: {
        version: 1,
        candidates: [
          {
            position: 0,
            execution_profile_id: "primary",
            enabled: true,
            rules: { on_provider_error: "retry_same", quota_limited: "stop" },
          },
        ],
      },
    });
    const candidate = result.dynamic?.candidates[0];
    expect(candidate?.policies.transient.retry.maxRetries).toBe(1);
    expect(candidate?.policies.hard.onExhausted).toBe("stop");
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

describe("toAgentProfilePayload dynamic candidates", () => {
  it("serializes dynamic candidates with opaque profile IDs", () => {
    const payload = toAgentProfilePayload({
      id: toAgentProfileId("dynamic-profile"),
      kind: "dynamic",
      dynamic: {
        version: 2,
        candidates: [
          {
            position: 0,
            executionProfileId: toAgentProfileId("primary"),
            enabled: true,
            policies: {
              version: 1,
              transient: {
                retry: { enabled: true, maxRetries: 1, initialIntervalSeconds: 5 },
                waitForReset: { enabled: false, maxWaitSeconds: 0 },
                onExhausted: "stop",
              },
              hard: {
                retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
                waitForReset: { enabled: false, maxWaitSeconds: 0 },
                onExhausted: "skip",
              },
            },
          },
        ],
      },
    });
    expect(payload.dynamic).toEqual({
      version: 2,
      candidates: [
        {
          position: 0,
          execution_profile_id: "primary",
          enabled: true,
          policies: {
            version: 1,
            transient: {
              retry: { enabled: true, max_retries: 1, initial_interval_seconds: 5 },
              wait_for_reset: { enabled: false, max_wait_seconds: 0 },
              on_exhausted: "stop",
            },
            hard: {
              retry: { enabled: false, max_retries: 0, initial_interval_seconds: 0 },
              wait_for_reset: { enabled: false, max_wait_seconds: 0 },
              on_exhausted: "skip",
            },
          },
        },
      ],
    });
  });
});
