import { describe, expect, it } from "vitest";
import { deriveStatus } from "./prepare-progress";

describe("prepare progress status", () => {
  it("does not infer completion from agentctl readiness before an attempt snapshot completes", () => {
    expect(
      deriveStatus({
        prepareStatus: "preparing",
        sessionState: "STARTING",
        agentctlStatus: "ready",
        hasFailedStep: false,
        hasWarnings: false,
        hasRunningStep: false,
        hasPreparationAttempt: true,
      }),
    ).toBe("preparing");
  });

  it("derives completed_with_warnings when completed with warnings and no fatal failed steps", () => {
    expect(
      deriveStatus({
        prepareStatus: "completed",
        sessionState: "RUNNING",
        agentctlStatus: "ready",
        hasFailedStep: false,
        hasWarnings: true,
        hasRunningStep: false,
        hasPreparationAttempt: true,
      }),
    ).toBe("completed_with_warnings");
  });

  it("prioritizes completed_with_error when completed with both failed steps and warnings", () => {
    expect(
      deriveStatus({
        prepareStatus: "completed",
        sessionState: "RUNNING",
        agentctlStatus: "ready",
        hasFailedStep: true,
        hasWarnings: true,
        hasRunningStep: false,
        hasPreparationAttempt: true,
      }),
    ).toBe("completed_with_error");
  });

  it("derives failed when prepareStatus is failed regardless of warnings", () => {
    expect(
      deriveStatus({
        prepareStatus: "failed",
        sessionState: "FAILED",
        agentctlStatus: undefined,
        hasFailedStep: false,
        hasWarnings: true,
        hasRunningStep: false,
        hasPreparationAttempt: true,
      }),
    ).toBe("failed");
  });
});
