import { act, renderHook, waitFor, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api/domains/tool-payload-retention-api";
import { useToolPayloadRetention } from "./use-tool-payload-retention";
import type {
  ToolPayloadOperation,
  ToolPayloadRetentionStatus,
} from "@/lib/types/tool-payload-retention";
vi.mock("@/lib/api/domains/tool-payload-retention-api");
const status: ToolPayloadRetentionStatus = {
  supported: true,
  policy: { enabled: false, age: { value: 3, unit: "months" }, revision: 0 },
  preparation: { state: "none", choice: "" },
};
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (cause: unknown) => void;
  const promise = new Promise<T>((r, j) => {
    resolve = r;
    reject = j;
  });
  return { promise, resolve, reject };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue(status);
});
afterEach(cleanup);
it("loads the disabled policy without launching an analysis", async () => {
  const { result } = renderHook(useToolPayloadRetention);
  await waitFor(() => expect(result.current.status).toEqual(status));
  expect(api.analyzeToolPayloadRetention).not.toHaveBeenCalled();
});
it("does not let a GET started before saving overwrite the PUT response", async () => {
  const { result } = renderHook(useToolPayloadRetention);
  await waitFor(() => expect(result.current.status).toEqual(status));
  const old = deferred<ToolPayloadRetentionStatus>();
  vi.mocked(api.fetchToolPayloadRetention).mockReturnValueOnce(old.promise);
  let reload!: Promise<void>;
  act(() => {
    reload = result.current.reload();
  });
  const saved = {
    ...status,
    policy: { ...status.policy, revision: 1, age: { value: 4, unit: "months" as const } },
  };
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValue(saved);
  await act(async () => {
    await result.current.save(saved.policy);
  });
  await act(async () => {
    old.resolve(status);
    await reload;
  });
  expect(result.current.status).toEqual(saved);
});

it("serializes same-tick commands and keeps pending until the initiating request settles", async () => {
  const { result } = renderHook(useToolPayloadRetention);
  await waitFor(() => expect(result.current.status).toEqual(status));
  const request = deferred<{ operation_id: string }>();
  vi.mocked(api.analyzeToolPayloadRetention).mockReturnValueOnce(request.promise);
  let first!: Promise<unknown>;
  await act(async () => {
    first = result.current.analyze(status.policy.age);
    await expect(result.current.analyze(status.policy.age)).rejects.toMatchObject({ status: 409 });
    await result.current.reload();
  });
  expect(result.current.pending).toBe(true);
  expect(api.analyzeToolPayloadRetention).toHaveBeenCalledTimes(1);
  await act(async () => {
    request.resolve({ operation_id: "scan" });
    await first;
  });
  expect(result.current.pending).toBe(false);
  expect(result.current.active).toBe(true);
});
it("keeps saved policy on failure and releases the mutation lock for retry", async () => {
  const { result } = renderHook(useToolPayloadRetention);
  await waitFor(() => expect(result.current.status).toEqual(status));
  const error = new Error("offline");
  vi.mocked(api.saveToolPayloadRetention)
    .mockRejectedValueOnce(error)
    .mockResolvedValueOnce(status);
  await act(async () => {
    await expect(result.current.save(status.policy)).rejects.toBe(error);
  });
  expect(result.current.status).toEqual(status);
  expect(result.current.pending).toBe(false);
  expect(result.current.error).toBe(error);
  await act(async () => {
    await result.current.save(status.policy);
  });
  expect(result.current.error).toBeNull();
});

it("clears a recovered status error on background polling", async () => {
  vi.useFakeTimers();
  let unmount: (() => void) | undefined;
  try {
    const readError = new Error("status unavailable");
    vi.mocked(api.fetchToolPayloadRetention)
      .mockResolvedValueOnce(status)
      .mockRejectedValueOnce(readError)
      .mockResolvedValueOnce(status);
    const rendered = renderHook(useToolPayloadRetention);
    unmount = rendered.unmount;
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(rendered.result.current.error).toBe(readError);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(rendered.result.current.error).toBeNull();
  } finally {
    unmount?.();
    vi.useRealTimers();
  }
});

it("keeps action failures when a status read succeeds", async () => {
  const { result } = renderHook(useToolPayloadRetention);
  await waitFor(() => expect(result.current.status).toEqual(status));
  const actionError = new Error("analysis failed");
  vi.mocked(api.analyzeToolPayloadRetention).mockRejectedValueOnce(actionError);
  await act(async () => {
    await expect(result.current.analyze(status.policy.age)).rejects.toBe(actionError);
  });
  expect(result.current.error).toBe(actionError);
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValueOnce(status);
  await act(async () => {
    await result.current.reload();
  });
  expect(result.current.error).toBe(actionError);
});

it("does not let a stale status failure replace a mutation result", async () => {
  const { result } = renderHook(useToolPayloadRetention);
  await waitFor(() => expect(result.current.status).toEqual(status));
  const old = deferred<ToolPayloadRetentionStatus>();
  vi.mocked(api.fetchToolPayloadRetention).mockReturnValueOnce(old.promise);
  let reload!: Promise<void>;
  act(() => {
    reload = result.current.reload();
  });
  const saved = {
    ...status,
    policy: { ...status.policy, revision: 1, age: { value: 4, unit: "months" as const } },
  };
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValue(saved);
  await act(async () => {
    await result.current.save(saved.policy);
  });
  const staleError = new Error("stale read failed");
  await act(async () => {
    old.reject(staleError);
    await reload;
  });
  expect(result.current.status).toEqual(saved);
  expect(result.current.error).toBeNull();
});
it("cancels the accepted operation and applies the returned status", async () => {
  const { result } = renderHook(useToolPayloadRetention);
  await waitFor(() => expect(result.current.status).toEqual(status));
  vi.mocked(api.runToolPayloadRetention).mockResolvedValue({ operation_id: "cleanup" });
  await act(async () => {
    await result.current.run(0);
  });
  vi.mocked(api.cancelToolPayloadRetention).mockResolvedValue(status);
  await act(async () => {
    await result.current.cancel("cleanup");
  });
  expect(result.current.active).toBe(false);
  expect(result.current.acceptedId).toBeNull();
});

it("polls an accepted command and clears pending even when bounded history replaced its id", async () => {
  vi.useFakeTimers();
  try {
    const { result, unmount } = renderHook(useToolPayloadRetention);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    const operation: ToolPayloadOperation = {
      id: "already-finished",
      kind: "analysis",
      state: "running",
      age: status.policy.age,
      scanned: 0,
      eligible_tasks: 0,
      eligible_messages: 0,
      removed_messages: 0,
      payload_bytes: 0,
      skipped: {},
      cutoff: "2026-01-01T00:00:00.000Z",
      started_at: "2026-01-01T00:00:00.000Z",
    };
    const replacement: ToolPayloadOperation = {
      ...operation,
      id: "replacement",
      state: "succeeded",
      finished_at: "2026-01-01T00:01:00.000Z",
    };
    vi.mocked(api.fetchToolPayloadRetention)
      .mockResolvedValueOnce({ ...status, operation })
      .mockResolvedValue({ ...status, last_analysis: replacement });
    vi.mocked(api.analyzeToolPayloadRetention).mockResolvedValue({
      operation_id: "already-finished",
    });
    await act(async () => {
      await result.current.analyze(status.policy.age);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.active).toBe(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(result.current.active).toBe(false);
    expect(result.current.acceptedId).toBeNull();
    const calls = vi.mocked(api.fetchToolPayloadRetention).mock.calls.length;
    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });
    expect(api.fetchToolPayloadRetention).toHaveBeenCalledTimes(calls);
  } finally {
    vi.useRealTimers();
  }
});
