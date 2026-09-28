import { describe, expect, it } from "vitest";
import { shouldReservePageLevelMobileFeedbackOffset } from "./task-page-content-helpers";

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
