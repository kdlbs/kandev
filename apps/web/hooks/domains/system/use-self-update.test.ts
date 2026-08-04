import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  applyUpdate: vi.fn(),
  fetchSystemInfo: vi.fn(),
  fetchSystemJob: vi.fn(),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  applyUpdate: mocks.applyUpdate,
  fetchSystemInfo: mocks.fetchSystemInfo,
  fetchSystemJob: mocks.fetchSystemJob,
}));

import { useSelfUpdate } from "./use-self-update";

const STORAGE_KEY = "kandev.selfUpdate";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

beforeEach(() => {
  mocks.applyUpdate.mockReset();
  mocks.fetchSystemInfo.mockReset();
  mocks.fetchSystemJob.mockReset();
  mocks.fetchSystemJob.mockResolvedValue({ state: "succeeded" });
  localStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

function registerUpdateLifecycleTests() {
  it("locks and persists the target while installing", async () => {
    mocks.applyUpdate.mockResolvedValue({ job_id: "j1" });
    mocks.fetchSystemInfo.mockResolvedValue({ version: "v1.0.0" }); // not flipped yet

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1" }));

    await act(async () => {
      await result.current.start();
    });
    expect(mocks.applyUpdate).toHaveBeenCalledWith("UPDATE", "v1.0.1");
    expect(result.current.phase).toBe("installing");
    expect(result.current.isUpdating).toBe(true);
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) as string).target).toBe("v1.0.1");
  });

  it("finishes when the version flips to the target", async () => {
    mocks.applyUpdate.mockResolvedValue({ job_id: "j1" });
    mocks.fetchSystemInfo.mockResolvedValue({ version: "v1.0.1" });
    const persistedAtCompletion: Array<string | null> = [];
    const onComplete = vi.fn(() => {
      persistedAtCompletion.push(localStorage.getItem(STORAGE_KEY));
    });

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1", onComplete }));

    await act(async () => {
      await result.current.start();
    });
    await waitFor(() => expect(result.current.phase).toBe("done"));
    expect(result.current.isUpdating).toBe(false);
    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(persistedAtCompletion).toEqual([null]);
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it("reports 'restarting' while the backend is unreachable", async () => {
    mocks.applyUpdate.mockResolvedValue({ job_id: "j1" });
    mocks.fetchSystemInfo.mockRejectedValue(new Error("connection refused"));
    const onComplete = vi.fn();

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1", onComplete }));

    await act(async () => {
      await result.current.start();
    });
    await waitFor(() => expect(result.current.phase).toBe("restarting"));
    expect(result.current.isUpdating).toBe(true);
    expect(onComplete).not.toHaveBeenCalled();
  });

  it("surfaces an error when apply fails to start", async () => {
    mocks.applyUpdate.mockRejectedValue(new Error("rate limited"));

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1" }));

    await act(async () => {
      await result.current.start();
    });
    expect(result.current.phase).toBe("error");
    expect(result.current.errorMessage).toBe("rate limited");
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it("errors when the launch job reports failure", async () => {
    mocks.applyUpdate.mockResolvedValue({ job_id: "j1" });
    mocks.fetchSystemJob.mockResolvedValue({ state: "failed", message: "bootstrap failed" });
    mocks.fetchSystemInfo.mockResolvedValue({ version: "v1.0.0" }); // never flips
    const onComplete = vi.fn();

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1", onComplete }));

    await act(async () => {
      await result.current.start();
    });
    await waitFor(() => expect(result.current.phase).toBe("error"));
    expect(onComplete).not.toHaveBeenCalled();
  });
}

function registerUpdateRecoveryTests() {
  it("completes once when older and overlapping target-version polls settle", async () => {
    vi.useFakeTimers();
    const firstTargetPoll = deferred<{ version: string }>();
    const secondTargetPoll = deferred<{ version: string }>();
    mocks.applyUpdate.mockResolvedValue({ job_id: "j1" });
    mocks.fetchSystemInfo
      .mockResolvedValueOnce({ version: "v1.0.0" })
      .mockReturnValueOnce(firstTargetPoll.promise)
      .mockReturnValueOnce(secondTargetPoll.promise);
    const onComplete = vi.fn();

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1", onComplete }));

    await act(async () => {
      await result.current.start();
      await Promise.resolve();
    });
    expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1);
    expect(onComplete).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(3);

    await act(async () => {
      firstTargetPoll.resolve({ version: "v1.0.1" });
      secondTargetPoll.resolve({ version: "v1.0.1" });
      await Promise.all([firstTargetPoll.promise, secondTargetPoll.promise]);
    });

    expect(result.current.phase).toBe("done");
    expect(onComplete).toHaveBeenCalledTimes(1);
  });

  it("resumes an in-progress update persisted before a page reload", async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ target: "v1.0.1", startedAt: Date.now() }));
    mocks.fetchSystemInfo.mockResolvedValue({ version: "v1.0.1" });

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1" }));

    expect(result.current.isUpdating).toBe(true);
    await waitFor(() => expect(result.current.phase).toBe("done"));
  });

  it("ignores a stale persisted update past the safety window", () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ target: "v1.0.1", startedAt: Date.now() - 10 * 60 * 1000 }),
    );

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1" }));

    expect(result.current.phase).toBe("idle");
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
  });
}

describe("useSelfUpdate", () => {
  registerUpdateLifecycleTests();
  registerUpdateRecoveryTests();
});
