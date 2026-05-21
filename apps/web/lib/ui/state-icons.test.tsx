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
  it("returns false for terminal task states", () => {
    expect(shouldShowTaskRunningSpinner("COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("FAILED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("CANCELLED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("REVIEW")).toBe(false);
    expect(shouldShowTaskRunningSpinner("TODO")).toBe(false);
  });

  it("returns true for SCHEDULING with no primary session yet", () => {
    expect(shouldShowTaskRunningSpinner("SCHEDULING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", null)).toBe(true);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", undefined)).toBe(true);
  });

  it("returns true for IN_PROGRESS with a non-terminal primary session", () => {
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "RUNNING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "STARTING")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "CREATED")).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", undefined)).toBe(true);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", null)).toBe(true);
  });

  it("suppresses the spinner when the primary session has reached a terminal state", () => {
    // The repro from issue #985: agent finishes (session → COMPLETED) but the
    // workflow leaves the task in IN_PROGRESS for review/manual move. The
    // spinner must not keep spinning forever.
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "COMPLETED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "FAILED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("IN_PROGRESS", "CANCELLED")).toBe(false);
    expect(shouldShowTaskRunningSpinner("SCHEDULING", "COMPLETED")).toBe(false);
  });
});
