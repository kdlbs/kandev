import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSessionRecoveryFeedback } from "./use-session-recovery-feedback";

describe("useSessionRecoveryFeedback", () => {
  it.each(["RUNNING", "WAITING_FOR_INPUT"])(
    "clears inspection contention after the session becomes ready (%s)",
    (readyState) => {
      const setNotice = vi.fn();
      const setNoticeKind = vi.fn();
      const setters = {
        setError: vi.fn(),
        setNotice,
        setNoticeKind,
        setRecoveryFailure: vi.fn(),
        setResumptionState: vi.fn(),
      };
      const { rerender } = renderHook(
        ({ sessionState }) =>
          useSessionRecoveryFeedback({
            sessionId: "session-1",
            sessionState,
            error: null,
            notice: "workspace is still being checked",
            noticeKind: "inspection_busy",
            setters,
          }),
        { initialProps: { sessionState: "CANCELLED" as string | undefined } },
      );

      act(() => rerender({ sessionState: "STARTING" }));
      expect(setNotice).not.toHaveBeenCalled();
      expect(setNoticeKind).not.toHaveBeenCalled();

      act(() => rerender({ sessionState: readyState }));
      expect(setNotice).toHaveBeenLastCalledWith(null);
      expect(setNoticeKind).toHaveBeenLastCalledWith(null);
    },
  );
});
