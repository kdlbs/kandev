import { describe, expect, it } from "vitest";
import { branchRecoveryDetails, sessionRecoveryGuardDetails } from "./session-recovery-service";
import { WebSocketRequestError } from "@/lib/ws/client";

describe("sessionRecoveryGuardDetails", () => {
  it("returns the details for a retryable in-progress recovery refusal", () => {
    const error = new WebSocketRequestError("blocked", "CONFLICT", {
      kind: "session_recovery_in_progress",
      retryable: true,
      session_id: "session-1",
    });

    expect(sessionRecoveryGuardDetails(error)).toEqual({
      kind: "session_recovery_in_progress",
      retryable: true,
      session_id: "session-1",
    });
  });

  it("returns the details for a non-retryable unstoppable-agent refusal", () => {
    const error = new WebSocketRequestError("blocked", "UNAVAILABLE", {
      kind: "session_recovery_unstoppable",
      retryable: false,
      session_id: "session-2",
    });

    expect(sessionRecoveryGuardDetails(error)).toEqual({
      kind: "session_recovery_unstoppable",
      retryable: false,
      session_id: "session-2",
    });
  });

  it("returns null for an unrelated WebSocketRequestError", () => {
    const error = new WebSocketRequestError("nope", "CONFLICT", {
      kind: "branch_unrecoverable",
      recovery_action: "resume_new_branch",
    });

    expect(sessionRecoveryGuardDetails(error)).toBeNull();
  });

  it("returns null for a non-WebSocketRequestError", () => {
    expect(sessionRecoveryGuardDetails(new Error("plain"))).toBeNull();
  });

  it("does not match a branch recovery error", () => {
    const error = new WebSocketRequestError("branch gone", "CONFLICT", {
      kind: "branch_unrecoverable",
      recovery_action: "resume_new_branch",
      original_branch: "feature/lost",
    });

    expect(branchRecoveryDetails(error)).not.toBeNull();
    expect(sessionRecoveryGuardDetails(error)).toBeNull();
  });
});
