import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { interruptedRecoveryKey } from "@/lib/services/interrupted-session-recovery";
import { useInterruptedRecoveryBatch } from "./use-interrupted-recovery-batch";
const mocks = vi.hoisted(() => ({ continue: vi.fn(), read: vi.fn().mockReturnValue(null) }));
vi.mock("@/lib/services/interrupted-session-recovery", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/services/interrupted-session-recovery")>()),
  continueInterruptedSession: mocks.continue,
  readInterruptedCheckpoint: mocks.read,
  readInterruptedRecoveryResult: mocks.read,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
beforeEach(() => {
  vi.clearAllMocks();
  mocks.read.mockReturnValue(null);
});
it("keeps independent results after a partial batch failure and prevents repeated taps", async () => {
  const observed = {
    task_id: "task",
    session_id: "one",
    outcome: "uncertain" as const,
    recovery_revision: 1,
    recovery_identity: {
      submission_id: "old",
      stream_id: "stream",
      incarnation_id: "inc",
      harness_generation: 1,
      prompt_generation: 1,
    },
  };
  const candidates = [
    { taskId: "task", sessionId: "one", label: "One", observed },
    {
      taskId: "task",
      sessionId: "two",
      label: "Two",
      observed: { ...observed, session_id: "two" },
    },
  ];
  mocks.continue
    .mockResolvedValueOnce({ result: { ...observed, outcome: "continued" } })
    .mockRejectedValueOnce(new Error("unavailable"));
  const { result } = renderHook(() => useInterruptedRecoveryBatch(candidates));
  act(() => {
    result.current.toggle("one");
    result.current.toggle("two");
  });
  await act(async () => {
    await Promise.all([
      result.current.run("new instruction", true),
      result.current.run("new instruction", true),
    ]);
  });
  expect(result.current.results.one.outcome).toBe("continued");
  expect(result.current.failures).toEqual(["two"]);
  expect(result.current.completed).toBe(2);
  expect(result.current.busy).toBe(false);
  expect(mocks.continue).toHaveBeenCalledTimes(2);
});

it("hides old and late batch results when the recovery identity changes", async () => {
  const observed = {
    task_id: "task",
    session_id: "one",
    outcome: "uncertain" as const,
    recovery_revision: 1,
    recovery_identity: {
      submission_id: "old",
      stream_id: "stream",
      incarnation_id: "inc",
      harness_generation: 1,
      prompt_generation: 1,
    },
  };
  const item = { taskId: "task", sessionId: "one", label: "One", observed };
  let complete!: (value: unknown) => void;
  mocks.continue.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const { result, rerender } = renderHook(
    ({ candidate }) => useInterruptedRecoveryBatch([candidate]),
    { initialProps: { candidate: item } },
  );
  act(() => result.current.toggle("one"));
  let operation!: Promise<void>;
  act(() => {
    operation = result.current.run("new instruction", true);
  });
  const next = {
    ...item,
    observed: {
      ...observed,
      recovery_revision: 3,
      recovery_identity: {
        ...observed.recovery_identity,
        submission_id: "next",
        harness_generation: 2,
      },
    },
  };
  expect(interruptedRecoveryKey(next.observed)).not.toBe(interruptedRecoveryKey(observed));
  rerender({ candidate: next });
  await act(async () => {
    complete({ result: { ...observed, outcome: "continued" } });
    await operation;
  });
  expect(result.current.results.one).toBeUndefined();
  mocks.continue.mockResolvedValueOnce({ result: { ...next.observed, outcome: "continued" } });
  await act(async () => {
    await result.current.run("current instruction", true);
  });
  expect(mocks.continue).toHaveBeenLastCalledWith(
    expect.objectContaining({ observed: next.observed, instruction: "current instruction" }),
  );
  expect(result.current.results.one.recovery_identity).toEqual(next.observed.recovery_identity);
});
