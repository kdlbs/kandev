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
});
