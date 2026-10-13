import { describe, expect, it } from "vitest";
import {
  canAddCoordinator,
  COORDINATOR_NAME_MAX_LENGTH,
  isValidCoordinatorName,
} from "./validate-form";

describe("isValidCoordinatorName", () => {
  it("rejects an empty name", () => {
    expect(isValidCoordinatorName("")).toBe(false);
  });

  it("rejects a name that is only whitespace", () => {
    expect(isValidCoordinatorName("   ")).toBe(false);
  });

  it("accepts a name at exactly the 60 code point limit", () => {
    expect(isValidCoordinatorName("a".repeat(COORDINATOR_NAME_MAX_LENGTH))).toBe(true);
  });

  it("rejects a name one code point past the limit", () => {
    expect(isValidCoordinatorName("a".repeat(COORDINATOR_NAME_MAX_LENGTH + 1))).toBe(false);
  });

  it("counts astral code points rather than UTF-16 code units", () => {
    // Each emoji below is a single Unicode code point encoded as a UTF-16
    // surrogate pair; a naive `.length` check would count 120 and reject it.
    const emojiName = "\u{1F600}".repeat(COORDINATOR_NAME_MAX_LENGTH);
    expect(isValidCoordinatorName(emojiName)).toBe(true);
  });

  it("trims surrounding whitespace before measuring length", () => {
    expect(isValidCoordinatorName("  Coordinator  ")).toBe(true);
  });
});

describe("canAddCoordinator", () => {
  const validForm = {
    name: "Coordinator",
    agentProfileId: "agent-1",
    executorProfileId: "exec-1",
    taskAgentProfileId: "agent-2",
    taskExecutorProfileId: "exec-2",
  };

  it("is true when the name is valid and both profiles are chosen", () => {
    expect(canAddCoordinator(validForm)).toBe(true);
  });

  it("is false when the name is invalid", () => {
    expect(canAddCoordinator({ ...validForm, name: "" })).toBe(false);
  });

  it("is false when no agent profile is chosen", () => {
    expect(canAddCoordinator({ ...validForm, agentProfileId: "" })).toBe(false);
  });

  it("is false when no executor profile is chosen", () => {
    expect(canAddCoordinator({ ...validForm, executorProfileId: "" })).toBe(false);
  });

  it("is false when the agent for created tasks is not chosen", () => {
    expect(canAddCoordinator({ ...validForm, taskAgentProfileId: "" })).toBe(false);
    expect(canAddCoordinator({ ...validForm, taskExecutorProfileId: "" })).toBe(false);
  });
});
