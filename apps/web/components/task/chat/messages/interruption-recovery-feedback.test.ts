import { describe, expect, it } from "vitest";
import {
  continuationPhase,
  interruptionRecoveryKey,
  retryNoticeVisible,
  retainedTurnAttemptsKey,
  retainedTurnRecoveryKey,
} from "./interruption-recovery-feedback";

describe("provider interruption feedback", () => {
  it.each([
    ["disabled", "chat:providerRecoveryDisabledBody"],
    ["unsafe_work", "chat:providerRecoveryUnsafeBody"],
    ["unsupported_restore", "chat:providerRecoveryUnsupportedBody"],
    ["missing_evidence", "chat:providerRecoveryEvidenceBody"],
  ])("explains the attested manual reason %s", (recovery_reason, key) => {
    expect(interruptionRecoveryKey({ recovery_reason })).toBe(key);
  });
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

// @covers AC-PLATFORM-TURN-CONTINUITY-003.5
describe("capacity recovery deadline feedback", () => {
  it("distinguishes window expiry from exhausted attempts and refusal", () => {
    expect(
      retainedTurnRecoveryKey({
        runtime_retained: true,
        recovery_disposition: "window_expired",
        attempts_started: 3,
      }),
    ).toBe("chat:retainedTurnRecoveryWindowExpiredBody");
    expect(
      retainedTurnRecoveryKey({
        runtime_retained: true,
        recovery_disposition: "restart_interrupted",
        attempts_started: 6,
      }),
    ).toBe("chat:retainedTurnRecoveryRestartBody");
    expect(
      retainedTurnRecoveryKey({
        runtime_retained: true,
        recovery_disposition: "refused",
        attempts_started: 0,
      }),
    ).toBe("chat:retainedTurnRecoveryUnknownBody");
  });
});

describe("retained interruption refusal feedback", () => {
  it.each([
    ["unsafe_work", "chat:retainedTurnRecoveryUnsafeBody"],
    ["unsupported_restore", "chat:retainedTurnRecoveryUnsupportedBody"],
    ["missing_evidence", "chat:retainedTurnRecoveryEvidenceBody"],
  ])("explains the attested refusal reason %s without claiming an attempt ran", (reason, key) => {
    expect(
      retainedTurnRecoveryKey({
        runtime_retained: true,
        recovery_disposition: "refused",
        recovery_reason: reason,
        attempts_started: 0,
      }),
    ).toBe(key);
  });

  it("uses neutral copy for historical and unknown refusal reasons", () => {
    expect(
      retainedTurnRecoveryKey({ runtime_retained: true, recovery_disposition: "refused" }),
    ).toBe("chat:retainedTurnRecoveryUnknownBody");
    expect(
      retainedTurnRecoveryKey({
        runtime_retained: true,
        recovery_disposition: "refused",
        recovery_reason: "future_reason",
        attempts_started: 0,
      }),
    ).toBe("chat:retainedTurnRecoveryUnknownBody");
  });

  it("reports actual attempts only when a refused recovery started them", () => {
    expect(
      retainedTurnAttemptsKey({ recovery_disposition: "refused", attempts_started: 0 }),
    ).toBeUndefined();
    expect(retainedTurnAttemptsKey({ recovery_disposition: "refused", attempts_started: 2 })).toBe(
      "chat:retainedTurnRecoveryAttemptsStarted",
    );
    expect(
      retainedTurnAttemptsKey({ recovery_disposition: "exhausted", attempts_started: 2 }),
    ).toBeUndefined();
    expect(
      retainedTurnRecoveryKey({
        runtime_retained: true,
        recovery_disposition: "refused",
        recovery_reason: "unsupported_restore",
        attempts_started: 2,
      }),
    ).toBe("chat:retainedTurnRecoveryUnsupportedBody");
    expect(
      retainedTurnRecoveryKey({
        runtime_retained: true,
        recovery_disposition: "exhausted",
        attempts_started: 5,
      }),
    ).toBe("chat:retainedTurnRecoveryExhaustedBody");
  });
});
