import { describe, expect, it } from "vitest";
import { coordinatorProfileStatusMessageKey } from "./profile-status-message";

describe("coordinatorProfileStatusMessageKey", () => {
  it("returns null for an ok status", () => {
    expect(coordinatorProfileStatusMessageKey("agent", "ok")).toBeNull();
    expect(coordinatorProfileStatusMessageKey("executor", "ok")).toBeNull();
  });

  it("returns null for an undefined status (e.g. the list response, or a brand new draft)", () => {
    expect(coordinatorProfileStatusMessageKey("agent", undefined)).toBeNull();
  });

  it("maps agent missing to its own message key", () => {
    expect(coordinatorProfileStatusMessageKey("agent", "missing")).toBe(
      "coordinator:agentProfileMissingWarning",
    );
  });

  it("maps agent passthrough to its own message key, distinct from missing", () => {
    expect(coordinatorProfileStatusMessageKey("agent", "passthrough")).toBe(
      "coordinator:agentProfilePassthroughWarning",
    );
  });

  it("maps executor missing to its own message key", () => {
    expect(coordinatorProfileStatusMessageKey("executor", "missing")).toBe(
      "coordinator:executorProfileMissingWarning",
    );
  });

  it("maps an unknown status value to the field's missing message (design B8)", () => {
    expect(coordinatorProfileStatusMessageKey("agent", "some_future_status")).toBe(
      "coordinator:agentProfileMissingWarning",
    );
    expect(coordinatorProfileStatusMessageKey("executor", "some_future_status")).toBe(
      "coordinator:executorProfileMissingWarning",
    );
  });
});
