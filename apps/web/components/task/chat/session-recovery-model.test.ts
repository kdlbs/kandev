import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import { renderHook } from "@testing-library/react";
import { recoveryCopy, useRecoveryPresentation } from "./session-recovery-model";
import { selectActiveSessionRecovery } from "@/lib/active-session-recovery";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";

vi.mock("./messages/action-message-recovery", () => ({ sessionRecoveryAction: vi.fn() }));

const translate = ((key: string) => `localized:${key}`) as TFunction;

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.12
it.each([
  ["manual", "inspection_busy"],
  ["automatic", "inspection_busy"],
  ["manual", null],
  ["automatic", null],
] as const)("keeps cleanup evidence with a %s notice of kind %s", (owner, noticeKind) => {
  const notice = "Another recovery inspection is in progress.";
  const model = {
    sessionId: "session",
    stamp: "cleanup-error",
    kind: "generic_launch_failure",
    error: {
      message: "Cleanup failed",
      phase: "bootstrap",
      causes: [{ operation: "resume", code: "runtime_cleanup_failed", prompt_not_sent: true }],
    },
  };
  const actions = {
    busyAction: null,
    recoveryError: null,
    recoveryNotice: owner === "manual" ? notice : null,
    recoveryNoticeKind: owner === "manual" ? noticeKind : null,
  } as SessionRecoveryActions;
  const automaticRecovery = {
    requestIdentity: { taskId: "task", sessionId: "session", generation: 1, attemptId: 1 },
    resumptionState: "error" as const,
    error: null,
    notice,
    noticeKind,
    recoveryFailure: null,
    resumeSession: vi.fn(),
  };
  const { result } = renderHook(() =>
    useRecoveryPresentation(model, actions, {
      taskId: "task",
      workspaceId: "workspace",
      automaticRecovery: owner === "automatic" ? automaticRecovery : null,
    }),
  );
  expect(result.current.copy.summary).toBe(
    noticeKind === "inspection_busy"
      ? notice
      : "The previous runtime could not be stopped. Check executor connectivity, then retry Resume.",
  );
  expect(result.current.detailFields).toContainEqual(
    expect.objectContaining({ value: "runtime_cleanup_failed" }),
  );
});

describe("manual provider recovery copy", () => {
  // @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
  it("uses localized neutral copy instead of a retained exhaustion claim", () => {
    const copy = recoveryCopy(
      {
        sessionId: "session",
        kind: "provider_interrupted",
        summary: "The agent connection kept dropping after several retries.",
      },
      translate,
    );
    expect(copy.summary).toBe("localized:chat:providerManualRecoveryBody");
    expect(copy.title).toBe("localized:task:sessionRecoveryFailed");
  });

  it("preserves ordinary readable failure summaries", () => {
    expect(
      recoveryCopy(
        { sessionId: "session", kind: "generic", summary: "Provider stopped." },
        translate,
      ).summary,
    ).toBe("Provider stopped.");
  });

  it("localizes the matching persisted recovery while retaining its technical detail", () => {
    const model = selectActiveSessionRecovery(
      {
        id: "session",
        state: "WAITING_FOR_INPUT",
        error_message: "Error: RetriableError: Connection stalled",
        metadata: {
          last_agent_error: {
            stamp: "failure",
            message: "Error: RetriableError: Connection stalled",
          },
        },
      },
      [
        {
          id: "recovery",
          session_id: "session",
          created_at: "2026-10-02T09:29:36Z",
          metadata: {
            recovery_actions: true,
            error_stamp: "failure",
            failure_kind: "provider_interrupted",
            error_output: "HTTP/2 stream closed with error code CANCEL (0x8)",
          },
        },
      ],
    );
    expect(model).not.toBeNull();
    expect(recoveryCopy(model!, translate).summary).toBe(
      "localized:chat:providerManualRecoveryBody",
    );
    expect(model?.details).toBe("HTTP/2 stream closed with error code CANCEL (0x8)");
  });

  it("retains quota reset guidance when the reset timestamp is invalid", () => {
    const copy = recoveryCopy(
      { sessionId: "session", kind: "provider_quota_limited", metadata: { reset_at: "invalid" } },
      translate,
    );
    expect(copy.summary).toBe("localized:chat:providerQuotaResetUnknown");
  });
});
