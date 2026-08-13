import { describe, expect, it } from "vitest";
import { isLaunchStateRegression } from "./session-state";

describe("isLaunchStateRegression", () => {
  it("applies the launch response over a never-started (CREATED) session", () => {
    expect(isLaunchStateRegression("CREATED", "STARTING")).toBe(false);
    expect(isLaunchStateRegression("CREATED", "RUNNING")).toBe(false);
  });

  it("applies the launch response over an idle (resume-skipped) session", () => {
    expect(isLaunchStateRegression("IDLE", "STARTING")).toBe(false);
    expect(isLaunchStateRegression("IDLE", "RUNNING")).toBe(false);
  });

  it("does not regress a live RUNNING state with a delayed STARTING response", () => {
    expect(isLaunchStateRegression("RUNNING", "STARTING")).toBe(true);
  });

  it("does not regress a live WAITING_FOR_INPUT state with a delayed STARTING response", () => {
    expect(isLaunchStateRegression("WAITING_FOR_INPUT", "STARTING")).toBe(true);
  });

  it("does not regress a live FAILED state with a delayed STARTING response", () => {
    expect(isLaunchStateRegression("FAILED", "STARTING")).toBe(true);
    expect(isLaunchStateRegression("CANCELLED", "STARTING")).toBe(true);
    expect(isLaunchStateRegression("COMPLETED", "STARTING")).toBe(true);
  });

  it("accepts a STARTING response while the live state is already STARTING", () => {
    expect(isLaunchStateRegression("STARTING", "STARTING")).toBe(false);
  });

  it("applies a RUNNING launch response over a live STARTING state", () => {
    expect(isLaunchStateRegression("STARTING", "RUNNING")).toBe(false);
  });

  it("never regresses when there is no live state", () => {
    expect(isLaunchStateRegression(undefined, "STARTING")).toBe(false);
    expect(isLaunchStateRegression("", "STARTING")).toBe(false);
  });

  it("treats unknown states as non-regressing", () => {
    expect(isLaunchStateRegression("SOMETHING_NEW", "STARTING")).toBe(false);
    expect(isLaunchStateRegression("CREATED", "SOMETHING_NEW")).toBe(false);
  });
});
