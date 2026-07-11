import { describe, expect, it } from "vitest";
import { isValidElement, type ReactNode } from "react";
import {
  IconCheck,
  IconCircleCheck,
  IconCircleFilled,
  IconLoader2,
  IconMessageQuestion,
} from "@tabler/icons-react";
import { getSessionStateIcon, getTaskStateIcon, shouldShowTaskRunningSpinner } from "./state-icons";

function iconType(node: ReactNode) {
  if (!isValidElement(node)) throw new Error("Expected React element");
  return node.type;
}

function iconClassName(node: ReactNode): string {
  if (!isValidElement(node)) throw new Error("Expected React element");
  return (node.props as { className?: string }).className ?? "";
}

describe("getTaskStateIcon", () => {
  it("uses the question icon for waiting-for-input task state", () => {
    expect(iconType(getTaskStateIcon("WAITING_FOR_INPUT"))).toBe(IconMessageQuestion);
  });

  it("uses the question icon when there is a pending clarification", () => {
    expect(iconType(getTaskStateIcon("REVIEW", undefined, true))).toBe(IconMessageQuestion);
  });

  it("keeps review task state as the review check without pending clarification", () => {
    expect(iconType(getTaskStateIcon("REVIEW", undefined, false))).toBe(IconCheck);
  });
});

describe("getSessionStateIcon — fine-grained busy tri-state", () => {
  // ADR-0035. Three distinguishable conditions:
  //  (a) RUNNING + generating  → the established static "running" dot (unchanged)
  //  (b) RUNNING + background   → working-in-background spinner, NOT the done check
  //  (c) COMPLETED              → done checkmark
  it("(a) keeps the established static running dot while the foreground is generating", () => {
    // The fine-grained signal only ADDS a background indicator; the foreground
    // running affordance is deliberately left as it always was (static dot).
    const a = getSessionStateIcon("RUNNING", undefined, "generating");
    expect(iconType(a)).toBe(IconCircleFilled);
    expect(iconClassName(a)).not.toContain("animate-spin");
  });

  it("(a) defaults to the running dot when the substate is unknown", () => {
    // Absent/null substate must preserve the historical RUNNING affordance.
    expect(iconType(getSessionStateIcon("RUNNING"))).toBe(IconCircleFilled);
    expect(iconType(getSessionStateIcon("RUNNING", undefined, null))).toBe(IconCircleFilled);
  });

  it("(b) shows a working spinner — never the done checkmark — while background work runs", () => {
    const b = getSessionStateIcon("RUNNING", undefined, "background");
    expect(iconType(b)).toBe(IconLoader2);
    expect(iconType(b)).not.toBe(IconCircleCheck);
    expect(iconClassName(b)).toContain("animate-spin");
  });

  it("(b) is visually distinct from (a) so the operator can tell them apart", () => {
    const a = iconClassName(getSessionStateIcon("RUNNING", undefined, "generating"));
    const b = iconClassName(getSessionStateIcon("RUNNING", undefined, "background"));
    expect(a).not.toBe(b);
  });

  it("(c) flips to the done checkmark only once the session leaves RUNNING", () => {
    // The (b)→(c) flip: the coarse state stays RUNNING while background work is
    // outstanding, so the checkmark appears only after the last task finishes
    // and the session settles to COMPLETED.
    expect(iconType(getSessionStateIcon("COMPLETED"))).toBe(IconCircleCheck);
    // A stale "background" substate must not resurrect a spinner on a terminal
    // session — the coarse state governs (c).
    expect(iconType(getSessionStateIcon("COMPLETED", undefined, "background"))).toBe(
      IconCircleCheck,
    );
  });
});

describe("shouldShowTaskRunningSpinner", () => {
  it("returns false for non-loading task states without an active session", () => {
    expect(shouldShowTaskRunningSpinner("COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("FAILED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("CANCELLED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("REVIEW")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO")).toBe(false);
  });

  it("returns true for non-TODO task states with an actively running primary session", () => {
    expect(shouldShowTaskRunningSpinner("REVIEW", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("COMPLETED", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("FAILED", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("CANCELLED", "RUNNING")).toBe(true);
  });

  it("returns true for SCHEDULING with no primary session yet", () => {
    expect(shouldShowTaskRunningSpinner("SCHEDULING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", null)).toBe(true);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", undefined)).toBe(true);
  });

  it("returns true for IN_PROGRESS when the primary session is actively running", () => {
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "STARTING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "CREATED")).toBe(true);
  });

  it("returns true for IN_PROGRESS when no primary session is attached yet", () => {
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", undefined)).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", null)).toBe(true);
  });

  it("suppresses the spinner when the primary session is terminal", () => {
    // Repro from issue #985: agent finishes (session → COMPLETED) but the
    // workflow leaves the task in IN_PROGRESS for review/manual move. The
    // spinner must not keep spinning forever.
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "FAILED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "CANCELLED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", "COMPLETED")).toBe(false);
  });

  it("suppresses the spinner when the primary session is paused (waiting/idle)", () => {
    // Same desync class, paused branch: agent stopped to wait for input or
    // was torn down (office IDLE). The spinner is misleading.
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "WAITING_FOR_INPUT")).toBe(false);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "IDLE")).toBe(false);
  });

  it("suppresses the spinner for a CREATED primary session on an inactive task", () => {
    // Repro from the stuck kanban cards (PR #11571 / #11502): task CREATED,
    // sitting in a Waiting column, with a primary session that was persisted in
    // CREATED and never advanced (no executor, no turns). CREATED means "agent
    // not started", so it must defer to the task state instead of spinning.
    expect(shouldShowTaskRunningSpinner("CREATED", "CREATED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("REVIEW", "CREATED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("COMPLETED", "CREATED")).toBe(false);
  });

  it("still spins for a CREATED primary session during a genuine launch", () => {
    // During an actual launch the task state is SCHEDULING/IN_PROGRESS while the
    // session momentarily sits in CREATED. Deferring to the task state keeps the
    // spinner on for that startup window.
    expect(shouldShowTaskRunningSpinner("SCHEDULING", "CREATED")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "CREATED")).toBe(true);
  });

  it("suppresses the spinner for TODO regardless of primary session state", () => {
    // TODO is the queued/not-started column. A stale primary session state
    // (e.g. task moved back from IN_PROGRESS with the session still alive)
    // must not paint the running spinner on the kanban card.
    expect(shouldShowTaskRunningSpinner("TODO", "RUNNING")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "STARTING")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "CREATED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "WAITING_FOR_INPUT")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO", "IDLE")).toBe(false);
  });
});
