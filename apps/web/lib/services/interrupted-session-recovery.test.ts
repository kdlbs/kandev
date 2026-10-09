import { beforeEach, expect, it, vi } from "vitest";
import {
  continueInterruptedSession,
  readInterruptedCheckpoint,
} from "./interrupted-session-recovery";
const mocks = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request: mocks.request }) }));
const observed = {
  task_id: "task",
  session_id: "session",
  outcome: "uncertain" as const,
  recovery_revision: 4,
  allowed_actions: ["resume_interrupted"],
  recovery_identity: {
    submission_id: "old",
    stream_id: "stream",
    incarnation_id: "inc",
    harness_generation: 1,
    prompt_generation: 2,
  },
};
const options = {
  taskId: "task",
  sessionId: "session",
  observed,
  instruction: "Inspect saved changes",
  acknowledged: true,
  failureMessage: "failed",
};
beforeEach(() => {
  window.sessionStorage.clear();
  vi.clearAllMocks();
});
it("persists the instruction before dispatch and retries the same checkpoint after a lost response", async () => {
  mocks.request.mockImplementation(async (action, payload) => {
    if (action === "session.recover") return observed;
    expect(readInterruptedCheckpoint("task", "session")?.request.instruction).toBe(
      options.instruction,
    );
    expect(payload.items[0].idempotency_key).toBe("interrupted:old:4");
    throw new Error("connection lost");
  });
  await expect(continueInterruptedSession(options)).rejects.toThrow("connection lost");
  mocks.request.mockImplementation(async (action) =>
    action === "session.recover"
      ? observed
      : {
          completed: 1,
          results: [
            { task_id: "task", session_id: "session", recovery_revision: 4, outcome: "continued" },
          ],
        },
  );
  const result = await continueInterruptedSession(options);
  expect(result.result?.outcome).toBe("continued");
  expect(readInterruptedCheckpoint("task", "session")?.result?.outcome).toBe("continued");
});
it("rejects a changed revision and missing acknowledgment before dispatch", async () => {
  mocks.request.mockResolvedValue({ ...observed, recovery_revision: 5 });
  await expect(continueInterruptedSession(options)).rejects.toThrow("failed");
  await expect(continueInterruptedSession({ ...options, acknowledged: false })).rejects.toThrow(
    "failed",
  );
  expect(readInterruptedCheckpoint("task", "session")).toBeNull();
  expect(mocks.request.mock.calls.every(([action]) => action === "session.recover")).toBe(true);
});

it("preserves the authoritative blocked reason without sending a continuation", async () => {
  mocks.request.mockResolvedValue({
    ...observed,
    outcome: "blocked",
    allowed_actions: [],
    reason: "missing_canonical_submission",
  });
  await expect(continueInterruptedSession(options)).rejects.toMatchObject({
    deliveryRecovery: { reason: "missing_canonical_submission" },
  });
  expect(mocks.request).toHaveBeenCalledTimes(1);
});
