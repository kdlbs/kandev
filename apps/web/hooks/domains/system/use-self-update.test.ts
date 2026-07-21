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

describe("useSelfUpdate", () => {
  it("locks and persists the target while installing", async () => {
    mocks.applyUpdate.mockResolvedValue({ job_id: "j1" });
    mocks.fetchSystemInfo.mockResolvedValue({ version: "v1.0.0" }); // not flipped yet

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1" }));

    await act(async () => {
      await result.current.start();
    });
    expect(mocks.applyUpdate).toHaveBeenCalledWith("UPDATE");
    expect(result.current.phase).toBe("installing");
    expect(result.current.isUpdating).toBe(true);
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) as string).target).toBe("v1.0.1");
  });

  it("finishes when the version flips to the target", async () => {
    mocks.applyUpdate.mockResolvedValue({ job_id: "j1" });
    mocks.fetchSystemInfo.mockResolvedValue({ version: "v1.0.1" });
    const onComplete = vi.fn();

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1", onComplete }));

    await act(async () => {
      await result.current.start();
    });
    await waitFor(() => expect(result.current.phase).toBe("done"));
    expect(result.current.isUpdating).toBe(false);
    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it("reports 'restarting' while the backend is unreachable", async () => {
    mocks.applyUpdate.mockResolvedValue({ job_id: "j1" });
    mocks.fetchSystemInfo.mockRejectedValue(new Error("connection refused"));

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1" }));

    await act(async () => {
      await result.current.start();
    });
    await waitFor(() => expect(result.current.phase).toBe("restarting"));
    expect(result.current.isUpdating).toBe(true);
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

    const { result } = renderHook(() => useSelfUpdate({ latestVersion: "v1.0.1" }));

    await act(async () => {
      await result.current.start();
    });
    await waitFor(() => expect(result.current.phase).toBe("error"));
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
});
