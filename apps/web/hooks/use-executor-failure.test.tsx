import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { useExecutorFailure } from "./use-executor-failure";
import type { ExecutorFailureEpisode } from "@/lib/types/executor-failure";
const { recheck } = vi.hoisted(() => ({ recheck: vi.fn() }));
vi.mock("@/lib/api/domains/executor-failure-api", () => ({ recheckExecutorFailure: recheck }));
const first = { id: "first", revision: 1, state: "active" } as ExecutorFailureEpisode;
describe("executor recheck ownership", () => {
  it("does not retain uncertainty when the task changes", async () => {
    recheck.mockRejectedValueOnce(new Error("API unavailable"));
    const { result, rerender } = renderHook(({ id, episode }) => useExecutorFailure(id, episode), {
      initialProps: { id: "one", episode: first },
    });
    await act(async () => {
      await result.current.recheck();
    });
    expect(result.current.unverified).toBe(true);
    rerender({ id: "two", episode: { ...first, id: "second" } });
    expect(result.current.unverified).toBe(false);
  });
  it("ignores delayed recovery of a previous episode", async () => {
    let finish: (value: ExecutorFailureEpisode) => void = () => {};
    recheck.mockReturnValueOnce(
      new Promise<ExecutorFailureEpisode>((resolve) => {
        finish = resolve;
      }),
    );
    const { result, rerender } = renderHook(({ episode }) => useExecutorFailure("task", episode), {
      initialProps: { episode: first },
    });
    act(() => {
      void result.current.recheck();
    });
    rerender({ episode: { ...first, id: "successor", revision: 2 } });
    await act(async () => {
      finish({ ...first, state: "resolved", revision: 2 });
    });
    await waitFor(() => expect(result.current.pending).toBe(false));
    expect(result.current.episode?.id).toBe("successor");
    expect(result.current.episode?.state).toBe("active");
  });
});

it("coalesces repeated clicks before React updates pending state", async () => {
  recheck.mockClear();
  let finish: (value: ExecutorFailureEpisode) => void = () => {};
  recheck.mockImplementation(
    () =>
      new Promise<ExecutorFailureEpisode>((resolve) => {
        finish = resolve;
      }),
  );
  const { result } = renderHook(() => useExecutorFailure("task", first));
  act(() => {
    void result.current.recheck();
    void result.current.recheck();
  });
  expect(recheck).toHaveBeenCalledTimes(1);
  await act(async () => {
    finish({ ...first, state: "resolved", revision: 2 });
  });
});
