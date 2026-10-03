import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import { recoveryCopy } from "./session-recovery-model";
import { selectActiveSessionRecovery } from "@/lib/active-session-recovery";

vi.mock("./messages/action-message-recovery", () => ({ sessionRecoveryAction: vi.fn() }));

const translate = ((key: string) => `localized:${key}`) as TFunction;

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
