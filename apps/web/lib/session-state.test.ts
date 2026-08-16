import { describe, expect, it } from "vitest";
import { isLaunchStateRegression, shouldShowComposerAgentStartHint } from "./session-state";

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

describe("shouldShowComposerAgentStartHint", () => {
  const base = {
    resumeSkipped: true,
    hasRecoveryActions: false,
    sessionId: "s1",
  } as const;

  it("shows for a resume-skipped WAITING_FOR_INPUT session without recovery actions", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: "WAITING_FOR_INPUT" })).toBe(
      true,
    );
  });

  it("shows for a resume-skipped IDLE session (office idle shape)", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: "IDLE" })).toBe(true);
  });

  it("hides when the session state is unknown/absent (a send might be rejected)", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: undefined })).toBe(false);
  });

  it("hides when the session is not resume-skipped", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, resumeSkipped: false })).toBe(false);
  });

  it("hides for a FAILED session (recovery owns the surface)", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: "FAILED" })).toBe(false);
  });

  it("hides for terminal CANCELLED/COMPLETED sessions (the composer rejects sends)", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: "CANCELLED" })).toBe(false);
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: "COMPLETED" })).toBe(false);
  });

  it("hides while the session is STARTING", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: "STARTING" })).toBe(false);
  });

  it("hides while the session is RUNNING", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: "RUNNING" })).toBe(false);
  });

  it("hides for a never-started CREATED session (the description button owns that surface)", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionState: "CREATED" })).toBe(false);
  });

  it("hides when recovery actions are already visible", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, hasRecoveryActions: true })).toBe(false);
  });

  it("hides when no session is bound", () => {
    expect(shouldShowComposerAgentStartHint({ ...base, sessionId: null })).toBe(false);
  });
});
