import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { useOrchestrationData } from "./use-orchestration-data";

it("clears prior workspace data synchronously and rejects delayed reads after a switch", async () => {
  const first = vi.fn().mockResolvedValue("workspace one");
  let resolve!: (value: string) => void;
  const delayed = vi.fn(
    () =>
      new Promise<string>((done) => {
        resolve = done;
      }),
  );
  const last = vi.fn().mockResolvedValue("workspace three");
  const { result, rerender } = renderHook(({ load }) => useOrchestrationData(load), {
    initialProps: { load: first },
  });
  await waitFor(() => expect(result.current.data).toBe("workspace one"));
  rerender({ load: delayed });
  expect(result.current.data).toBeUndefined();
  rerender({ load: last });
  await waitFor(() => expect(result.current.data).toBe("workspace three"));
  await act(async () => {
    resolve("workspace two");
  });
  expect(result.current.data).toBe("workspace three");
});

it("isolates results and errors when the owner changes without changing the loader", async () => {
  let finish!: (value: string) => void;
  const load = vi
    .fn()
    .mockResolvedValueOnce("first owner")
    .mockImplementationOnce(
      () =>
        new Promise<string>((resolve) => {
          finish = resolve;
        }),
    )
    .mockRejectedValueOnce(new Error("old scope"))
    .mockResolvedValue("fourth owner");
  const { result, rerender } = renderHook(({ owner }) => useOrchestrationData(load, owner), {
    initialProps: { owner: "one" },
  });
  await waitFor(() => expect(result.current.data).toBe("first owner"));
  rerender({ owner: "two" });
  expect(result.current.data).toBeUndefined();
  rerender({ owner: "three" });
  await waitFor(() => expect(result.current.error).toBe("old scope"));
  await act(async () => {
    finish("second owner");
  });
  expect(result.current.data).toBeUndefined();
  rerender({ owner: "four" });
  expect(result.current.error).toBeUndefined();
});
