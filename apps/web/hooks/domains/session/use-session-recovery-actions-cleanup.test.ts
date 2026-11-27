import { act, renderHook } from "@testing-library/react";
import { vi, it, expect } from "vitest";
import { createElement, type PropsWithChildren } from "react";
import { StateProvider } from "@/components/state-provider";
import { WebSocketRequestError } from "@/lib/ws/client";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";

const mocks = vi.hoisted(() => ({ requestSessionRecover: vi.fn() }));
vi.mock("@/lib/services/session-recovery-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/services/session-recovery-service")>()),
  requestSessionRecover: mocks.requestSessionRecover,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
const TASK_ID = "cleanup-task";
const SESSION_ID = "cleanup-session";
const wrapper = ({ children }: PropsWithChildren) => createElement(StateProvider, null, children);

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.13
it("associates a cleanup response with its newer durable error stamp", async () => {
  const deferred = Promise.withResolvers<void>();
  mocks.requestSessionRecover.mockReturnValueOnce(deferred.promise);
  const { result, rerender } = renderHook(
    ({ errorStamp }) =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID, errorStamp }),
    { initialProps: { errorStamp: "previous-error" }, wrapper },
  );
  let request: Promise<boolean>;
  act(() => {
    request = result.current.handleRecover("resume");
  });
  rerender({ errorStamp: "cleanup-error" });
  await act(async () => {
    deferred.reject(
      new WebSocketRequestError("safe cleanup failure", "INTERNAL_ERROR", {
        kind: "runtime_cleanup_failed",
        session_id: SESSION_ID,
        error_stamp: "cleanup-error",
        causes: [{ operation: "resume", code: "runtime_cleanup_failed", prompt_not_sent: true }],
      }),
    );
    await request!;
  });
  expect(result.current.manualRecoveryFailure).toMatchObject({
    sessionId: SESSION_ID,
    errorStamp: "cleanup-error",
    causes: [{ operation: "resume", code: "runtime_cleanup_failed", prompt_not_sent: true }],
  });
  expect(result.current.recoveryError?.message).toBe("task:sessionRuntimeCleanupFailedBody");
});
