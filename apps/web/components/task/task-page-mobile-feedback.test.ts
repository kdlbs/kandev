import { describe, expect, it } from "vitest";
import { selectTaskStatusSummary } from "@/lib/task-status-summary";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import {
  resolveTaskPageBootstrapRecoveryError,
  shouldReservePageLevelMobileFeedbackOffset,
} from "./task-page-content-helpers";

function pageFeedbackParams(
  overrides: Partial<Parameters<typeof shouldReservePageLevelMobileFeedbackOffset>[0]> = {},
) {
  return {
    isMobile: true,
    hasTaskMoveError: false,
    hasEnsureSessionError: false,
    hasBootstrapRecoveryError: false,
    effectiveSessionId: null,
    isSessionPassthrough: false,
    hasResumptionError: false,
    hasResumptionNotice: false,
    hasStatusUnavailable: false,
    ...overrides,
  };
}

describe("shouldReservePageLevelMobileFeedbackOffset", () => {
  it("reserves space for a phone ensure failure without a task summary error", () => {
    expect(
      shouldReservePageLevelMobileFeedbackOffset(
        pageFeedbackParams({ hasEnsureSessionError: true }),
      ),
    ).toBe(true);
  });

  it("reserves space for page-level status-unavailable feedback", () => {
    expect(
      shouldReservePageLevelMobileFeedbackOffset(
        pageFeedbackParams({ hasStatusUnavailable: true }),
      ),
    ).toBe(true);
  });

  it("reserves space for a task move error", () => {
    expect(
      shouldReservePageLevelMobileFeedbackOffset(pageFeedbackParams({ hasTaskMoveError: true })),
    ).toBe(true);
  });

  it("reserves space for a resumption error or notice", () => {
    expect(
      shouldReservePageLevelMobileFeedbackOffset(pageFeedbackParams({ hasResumptionError: true })),
    ).toBe(true);
    expect(
      shouldReservePageLevelMobileFeedbackOffset(pageFeedbackParams({ hasResumptionNotice: true })),
    ).toBe(true);
  });

  it("reserves space for a visible passthrough bootstrap recovery card", () => {
    expect(
      shouldReservePageLevelMobileFeedbackOffset(
        pageFeedbackParams({
          hasBootstrapRecoveryError: true,
          effectiveSessionId: "session-1",
          isSessionPassthrough: true,
        }),
      ),
    ).toBe(true);
  });

  it("does not reserve page space for a bootstrap error without a passthrough session", () => {
    expect(
      shouldReservePageLevelMobileFeedbackOffset(
        pageFeedbackParams({ hasBootstrapRecoveryError: true }),
      ),
    ).toBe(false);
  });

  it("uses the same live bootstrap status for recovery feedback and phone clearance", () => {
    const activeError: NonNullable<TaskStatusSummary["active_error"]> = {
      scope: "session",
      session_id: "session-1",
      stamp: "bootstrap-failed",
      occurred_at: "2026-09-28T10:00:00Z",
      preview: "The session failed to start",
      phase: "bootstrap",
    };
    const staleDetail: TaskStatusSummary = {
      revision: 1,
      updated_at: "2026-09-28T09:00:00Z",
      active_error: null,
    };
    const liveSummary: TaskStatusSummary = {
      revision: 2,
      updated_at: "2026-09-28T10:00:00Z",
      active_error: activeError,
    };
    const latestSummary = selectTaskStatusSummary(staleDetail, [liveSummary]);
    const bootstrapError = resolveTaskPageBootstrapRecoveryError(latestSummary, "session-1", null);

    expect(bootstrapError).toEqual(activeError);
    expect(
      shouldReservePageLevelMobileFeedbackOffset(
        pageFeedbackParams({
          hasBootstrapRecoveryError: bootstrapError !== null,
          effectiveSessionId: "session-1",
          isSessionPassthrough: true,
        }),
      ),
    ).toBe(true);
  });

  it("leaves shared task-summary errors and ordinary pages to their existing owners", () => {
    expect(shouldReservePageLevelMobileFeedbackOffset(pageFeedbackParams())).toBe(false);
    expect(
      shouldReservePageLevelMobileFeedbackOffset({
        ...pageFeedbackParams({ hasEnsureSessionError: true, hasStatusUnavailable: true }),
        isMobile: false,
      }),
    ).toBe(false);
  });
});
