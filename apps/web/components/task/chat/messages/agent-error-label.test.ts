import { describe, it, expect } from "vitest";
import { isEnvironmentSetupError, resolveAgentErrorLabel } from "./agent-error-label";

const FALLBACK = "Agent has encountered an error";

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

describe("resolveAgentErrorLabel", () => {
  it("returns the setup label only for genuine setup failures", () => {
    expect(resolveAgentErrorLabel("failed to launch container", FALLBACK)).toBe(
      "Environment setup failed",
    );
  });

  it("returns the fallback label for downstream agent/API errors", () => {
    expect(
      resolveAgentErrorLabel("API Error: 400 thinking blocks cannot be modified", FALLBACK),
    ).toBe(FALLBACK);
  });

  it("returns the fallback label when there is no error message", () => {
    expect(resolveAgentErrorLabel(undefined, FALLBACK)).toBe(FALLBACK);
    expect(resolveAgentErrorLabel("", FALLBACK)).toBe(FALLBACK);
  });
});
