import { describe, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/client";
import { contextContinuationDetails, requestSessionRecover } from "./session-recovery-service";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

describe("session recovery service", () => {
  it("recognizes typed native-state loss without authorizing generic failures", () => {
    const error = new WebSocketRequestError(
      "native state is unavailable",
      "SESSION_RESTORE_REQUIRED",
      {
        kind: "session_restore_required",
        recovery_action: "continue_from_history",
        reason: "native_state_missing",
        generation: 3,
      },
    );
    expect(contextContinuationDetails(error)).toMatchObject({
      kind: "session_restore_required",
      recovery_action: "continue_from_history",
      reason: "native_state_missing",
    });
    expect(
      contextContinuationDetails(new WebSocketRequestError("unknown", "INTERNAL_ERROR")),
    ).toBeNull();
  });

  it("accepts the explicit continuation action as a distinct protocol action", async () => {
    mocks.request.mockResolvedValue({ success: true });
    await expect(
      requestSessionRecover("task-1", "session-1", "continue_from_history", "failed"),
    ).resolves.toBeUndefined();
    expect(mocks.request).toHaveBeenCalledWith(
      "session.recover",
      { task_id: "task-1", session_id: "session-1", action: "continue_from_history" },
      30_000,
    );
  });
});
