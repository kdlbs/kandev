import { act, renderHook } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { useInterruptedRecoveryBatch } from "./use-interrupted-recovery-batch";
const mocks = vi.hoisted(() => ({ continue: vi.fn(), read: vi.fn().mockReturnValue(null) }));
vi.mock("@/lib/services/interrupted-session-recovery", () => ({
  continueInterruptedSession: mocks.continue,
  readInterruptedCheckpoint: mocks.read,
  readInterruptedRecoveryResult: mocks.read,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
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
