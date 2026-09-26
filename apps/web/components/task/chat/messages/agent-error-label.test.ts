import { describe, it, expect } from "vitest";
import { isEnvironmentSetupError, resolveAgentErrorLabelKey } from "./agent-error-label";

const FALLBACK = "task:agentHasEncounteredAnError";

describe("isEnvironmentSetupError", () => {
  it("matches genuine environment/workspace setup failures", () => {
    const setupFailures = [
      'environment preparation failed: branch "feature/foo" not found locally or on remote',
      "failed to launch container",
      "session already has an agent running somewhere",
      "race resolved during register",
      "failed to prepare fresh branch",
    ];
    for (const msg of setupFailures) {
      expect(isEnvironmentSetupError(msg)).toBe(true);
    }
  });

  it("does not match downstream agent / API errors", () => {
    const agentErrors = [
      "API Error: 400 messages.0.content.1: `thinking` or `redacted_thinking` blocks in the latest assistant message cannot be modified.",
      "API Error: 401 authentication_error: OAuth token has expired",
      "rate_limit_exceeded",
      "the agent crashed unexpectedly",
      "",
    ];
    for (const msg of agentErrors) {
      expect(isEnvironmentSetupError(msg)).toBe(false);
    }
  });

  it("is case-insensitive", () => {
    expect(isEnvironmentSetupError("Environment Preparation Failed: x")).toBe(true);
  });
});

describe("resolveAgentErrorLabelKey", () => {
  it("returns the setup label key only for genuine setup failures", () => {
    expect(resolveAgentErrorLabelKey("failed to launch container", FALLBACK)).toBe(
      "task:environmentSetupFailed",
    );
  });

  it("returns the fallback label key for downstream agent/API errors", () => {
    expect(
      resolveAgentErrorLabelKey("API Error: 400 thinking blocks cannot be modified", FALLBACK),
    ).toBe(FALLBACK);
  });

  it("returns the fallback label key when there is no error message", () => {
    expect(resolveAgentErrorLabelKey(undefined, FALLBACK)).toBe(FALLBACK);
    expect(resolveAgentErrorLabelKey("", FALLBACK)).toBe(FALLBACK);
  });
});
