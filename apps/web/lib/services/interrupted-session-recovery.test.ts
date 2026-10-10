import { beforeEach, expect, it, vi } from "vitest";
import {
  continueInterruptedSession,
  readInterruptedCheckpoint,
  readInterruptedRecoveryResult,
} from "./interrupted-session-recovery";
const SESSION_RECOVER = "session.recover";
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
    if (action === SESSION_RECOVER) return observed;
    expect(readInterruptedCheckpoint("task", "session")?.request.instruction).toBe(
      options.instruction,
    );
    expect(payload.items[0].idempotency_key).toBe("interrupted:old:4");
    throw new Error("connection lost");
  });
  await expect(continueInterruptedSession(options)).rejects.toThrow("connection lost");
  mocks.request.mockImplementation(async (action) =>
    action === SESSION_RECOVER
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
  expect(mocks.request.mock.calls.every(([action]) => action === SESSION_RECOVER)).toBe(true);
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

it("isolates saved requests and results from a later interruption of the same session", async () => {
  mocks.request.mockImplementation(async (action) =>
    action === SESSION_RECOVER
      ? observed
      : { completed: 1, results: [{ ...observed, outcome: "continued" }] },
  );
  await continueInterruptedSession(options);
  const later = {
    ...observed,
    recovery_revision: 6,
    recovery_identity: {
      ...observed.recovery_identity,
      submission_id: "next",
      harness_generation: 2,
    },
  };
  expect(readInterruptedCheckpoint("task", "session", later)).toBeNull();
  expect(readInterruptedRecoveryResult("task", "session", later)).toBeNull();
  expect(
    readInterruptedRecoveryResult("task", "session", { ...observed, recovery_revision: 5 })
      ?.outcome,
  ).toBe("continued");
});

it("does not let an old response overwrite a newer interruption checkpoint", async () => {
  let finishOld!: (value: unknown) => void;
  const later = {
    ...observed,
    recovery_revision: 6,
    recovery_identity: {
      ...observed.recovery_identity,
      submission_id: "next",
      harness_generation: 2,
    },
  };
  mocks.request.mockImplementation(async (action, payload) => {
    if (action === SESSION_RECOVER) return mocks.request.mock.calls.length === 1 ? observed : later;
    if (payload.items[0].recovery_identity.submission_id === "old")
      return new Promise((resolve) => {
        finishOld = resolve;
      });
    return { completed: 1, results: [{ ...later, outcome: "continued" }] };
  });
  const old = continueInterruptedSession(options);
  await vi.waitFor(() => expect(finishOld).toBeTypeOf("function"));
  const nextOperation = continueInterruptedSession({
    ...options,
    observed: later,
    instruction: "New instruction",
  });
  try {
    expect(nextOperation).not.toBe(old);
    const next = await nextOperation;
    expect(next.request.instruction).toBe("New instruction");
  } finally {
    finishOld({ completed: 1, results: [{ ...observed, outcome: "continued" }] });
    await old;
  }
  expect(readInterruptedCheckpoint("task", "session", later)?.request.instruction).toBe(
    "New instruction",
  );
});

it("does not dispatch or save a stale preflight after a newer interruption completes", async () => {
  let finishOld!: (value: unknown) => void;
  const later = {
    ...observed,
    recovery_revision: 6,
    recovery_identity: {
      ...observed.recovery_identity,
      submission_id: "next",
      harness_generation: 2,
    },
  };
  mocks.request.mockImplementation(async (action) => {
    if (action === SESSION_RECOVER && !finishOld)
      return new Promise((resolve) => {
        finishOld = resolve;
      });
    if (action === SESSION_RECOVER) return later;
    return { completed: 1, results: [{ ...later, outcome: "continued" }] };
  });
  const old = continueInterruptedSession(options);
  const rejected = expect(old).rejects.toThrow("failed");
  await continueInterruptedSession({
    ...options,
    observed: later,
    instruction: "Current instruction",
  });
  finishOld(observed);
  await rejected;
  expect(readInterruptedCheckpoint("task", "session", later)?.request.instruction).toBe(
    "Current instruction",
  );
  expect(mocks.request.mock.calls.filter(([action]) => action !== SESSION_RECOVER)).toHaveLength(1);
});

it("recovers a lost reply after the same interruption advances its revision", async () => {
  mocks.request.mockImplementation(async (action) => {
    if (action === SESSION_RECOVER) return observed;
    throw new Error("lost accepted response");
  });
  await expect(continueInterruptedSession(options)).rejects.toThrow("lost accepted response");
  const committed = { ...observed, recovery_revision: 5 };
  expect(readInterruptedCheckpoint("task", "session", committed)?.request.idempotency_key).toBe(
    "interrupted:old:4",
  );
  mocks.request.mockClear();
  mocks.request.mockResolvedValue({
    completed: 1,
    results: [{ ...committed, outcome: "continued" }],
  });
  const result = await continueInterruptedSession({ ...options, observed: committed });
  expect(result.result?.outcome).toBe("continued");
  expect(mocks.request).toHaveBeenCalledTimes(1);
  expect(mocks.request).toHaveBeenCalledWith(
    "session.recover_batch",
    {
      items: [
        expect.objectContaining({ idempotency_key: "interrupted:old:4", recovery_revision: 4 }),
      ],
    },
    150_000,
  );
  expect(
    readInterruptedCheckpoint("task", "session", {
      ...committed,
      recovery_identity: {
        ...committed.recovery_identity,
        submission_id: "different",
      },
    }),
  ).toBeNull();
});
