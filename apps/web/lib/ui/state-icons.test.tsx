import { describe, expect, it } from "vitest";
import { isValidElement, type ReactNode } from "react";
import { IconCheck, IconMessageQuestion } from "@tabler/icons-react";
import { getTaskStateIcon, shouldShowTaskRunningSpinner } from "./state-icons";

function iconType(node: ReactNode) {
  if (!isValidElement(node)) throw new Error("Expected React element");
  return node.type;
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

describe("shouldShowTaskRunningSpinner", () => {
  it("returns false for non-loading task states without an active session", () => {
    expect(shouldShowTaskRunningSpinner("COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("FAILED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("CANCELLED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("REVIEW")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO")).toBe(false);
  });

  it("returns true when any task state has an actively running primary session", () => {
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
