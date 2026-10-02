import { describe, expect, it } from "vitest";
import {
  continuationPhase,
  interruptionRecoveryKey,
  retryNoticeVisible,
} from "./interruption-recovery-feedback";

describe("provider interruption feedback", () => {
  it("keeps an owned continuation notice reachable while running", () => {
    expect(
      retryNoticeVisible("RUNNING", {
        retrying: true,
        recovery_mode: "continue",
        recovery_phase: "continuing",
      }),
    ).toBe(true);
    expect(retryNoticeVisible("RUNNING", { retrying: true })).toBe(false);
    expect(
      retryNoticeVisible("COMPLETED", {
        retrying: true,
        recovery_mode: "continue",
        recovery_phase: "continuing",
      }),
    ).toBe(false);
  });
  it("requires an attested mode and phase", () => {
    expect(
      continuationPhase({
        retrying: true,
        recovery_mode: "continue",
        recovery_phase: "reconnecting",
      }),
    ).toBe("reconnecting");
    expect(continuationPhase({ retrying: true, recovery_phase: "continuing" })).toBeUndefined();
    expect(
      continuationPhase({ retrying: true, recovery_mode: "continue", recovery_phase: "unknown" }),
    ).toBeUndefined();
  });
  it("reports exhaustion only with actual started attempts", () => {
    expect(
      interruptionRecoveryKey({ recovery_disposition: "exhausted", attempts_started: 0 }),
    ).toBe("chat:providerManualRecoveryBody");
    expect(
      interruptionRecoveryKey({ recovery_disposition: "exhausted", attempts_started: 5 }),
    ).toBe("chat:providerRecoveryExhaustedBody");
    expect(
      interruptionRecoveryKey({ recovery_disposition: "cancelled", attempts_started: 1 }),
    ).toBe("chat:providerRecoveryCancelledBody");
  });
});
