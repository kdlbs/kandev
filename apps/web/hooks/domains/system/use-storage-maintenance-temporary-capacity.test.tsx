import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  cleanupJob,
  cleanupJobId,
  disk,
  overview,
  settings,
  wrapper,
} from "./use-storage-maintenance.test-fixtures";

const mocks = vi.hoisted(() => ({
  adopt: vi.fn(),
  analyze: vi.fn(),
  deleteEntry: vi.fn(),
  purge: vi.fn(),
  fetchJob: vi.fn(),
  fetchOverview: vi.fn(),
  fetchDisk: vi.fn(),
  fetchPolicy: vi.fn(),
  fetchQuarantine: vi.fn(),
  fetchRuns: vi.fn(),
  restore: vi.fn(),
  run: vi.fn(),
  save: vi.fn(),
  toast: vi.fn(),
}));

afterEach(cleanup);

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  adoptStorageGoCache: mocks.adopt,
  analyzeStorage: mocks.analyze,
  deleteStorageQuarantine: mocks.deleteEntry,
  purgeStorageQuarantine: mocks.purge,
  fetchSystemJob: mocks.fetchJob,
  fetchStorageOverview: mocks.fetchOverview,
  fetchStorageDisk: mocks.fetchDisk,
  fetchStoragePolicy: mocks.fetchPolicy,
  fetchStorageQuarantine: mocks.fetchQuarantine,
  fetchStorageRuns: mocks.fetchRuns,
  restoreStorageQuarantine: mocks.restore,
  runStorageMaintenance: mocks.run,
  saveStorageSettings: mocks.save,
}));

import { mergeStorageDiskCapacity, useStorageMaintenance } from "./use-storage-maintenance";

const successfulObservedAt = "2026-09-28T10:00:00.000Z";

beforeEach(() => {
  vi.clearAllMocks();
  mocks.fetchOverview.mockResolvedValue(overview);
  mocks.fetchDisk.mockResolvedValue(disk);
  mocks.fetchPolicy.mockResolvedValue({
    settings: overview.settings,
    capabilities: overview.capabilities,
  });
  mocks.fetchRuns.mockResolvedValue([]);
  mocks.fetchQuarantine.mockResolvedValue([]);
  mocks.fetchJob.mockResolvedValue(cleanupJob);
  mocks.save.mockResolvedValue({ settings });
  mocks.run.mockResolvedValue({ job_id: cleanupJobId });
  mocks.purge.mockResolvedValue({ job_id: "purge-job" });
});

it("polls disk capacity only while visible and the Host tab is active", async () => {
  vi.useFakeTimers();
  const visibilityDescriptor = Object.getOwnPropertyDescriptor(document, "visibilityState");
  const setVisibility = (value: "visible" | "hidden") => {
    Object.defineProperty(document, "visibilityState", { configurable: true, value });
    document.dispatchEvent(new Event("visibilitychange"));
  };
  try {
    setVisibility("visible");
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useStorageMaintenance(active),
      { wrapper, initialProps: { active: true } },
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(result.current.disk).toEqual(disk);
    mocks.fetchDisk.mockClear();

    await act(async () => vi.advanceTimersByTimeAsync(29_999));
    expect(mocks.fetchDisk).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
      await Promise.resolve();
    });
    expect(mocks.fetchDisk).toHaveBeenCalledTimes(1);
    mocks.fetchDisk.mockClear();

    setVisibility("hidden");
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(mocks.fetchDisk).not.toHaveBeenCalled();

    setVisibility("visible");
    await act(async () => Promise.resolve());
    expect(mocks.fetchDisk).toHaveBeenCalledTimes(1);
    mocks.fetchDisk.mockClear();

    rerender({ active: false });
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(mocks.fetchDisk).not.toHaveBeenCalled();

    rerender({ active: true });
    await act(async () => Promise.resolve());
    expect(mocks.fetchDisk).toHaveBeenCalledTimes(1);
  } finally {
    if (visibilityDescriptor) {
      Object.defineProperty(document, "visibilityState", visibilityDescriptor);
    } else {
      Reflect.deleteProperty(document, "visibilityState");
    }
    vi.useRealTimers();
  }
});

it("keeps the last successful root capacity when a later read fails", () => {
  const previous = {
    ...disk,
    temporary_roots: [
      {
        requested_path: "/tmp",
        path: "/tmp",
        total_bytes: 100,
        used_bytes: 90,
        available_bytes: 10,
        used_percent: 90,
        available: true,
        observed_at: successfulObservedAt,
        shared_with_home: false,
      },
      {
        requested_path: "/old-temp",
        path: "/old-temp",
        total_bytes: 100,
        used_bytes: 40,
        available_bytes: 60,
        used_percent: 40,
        available: true,
        observed_at: successfulObservedAt,
        shared_with_home: null,
      },
    ],
  };
  const incoming = {
    ...disk,
    observed_at: "2026-09-28T10:01:00.000Z",
    temporary_roots: [
      {
        requested_path: "/tmp",
        path: "/tmp",
        total_bytes: 0,
        used_bytes: 0,
        available_bytes: 0,
        used_percent: 0,
        available: false,
        warning: "disk usage unavailable",
        observed_at: "2026-09-28T10:01:00.000Z",
        shared_with_home: false,
      },
    ],
  };

  const merged = mergeStorageDiskCapacity(previous, incoming);
  expect(merged.temporary_roots).toHaveLength(1);
  expect(merged.temporary_roots?.[0]).toMatchObject({
    path: "/tmp",
    available: true,
    used_percent: 90,
    observed_at: successfulObservedAt,
    stale: true,
  });
  expect(merged.available).toBe(true);
});

it("does not infer temporary capacity from an older response", () => {
  const previous = { ...disk, temporary_roots: [] };
  const incoming = { ...disk, temporary_roots: undefined };
  const merged = mergeStorageDiskCapacity(previous, incoming);
  expect(merged.temporary_roots).toBeUndefined();
});

it("retains and marks the last successful capacity stale after a rejected refresh, then clears stale on recovery", async () => {
  const initialRoot = {
    requested_path: "/tmp",
    path: "/tmp",
    total_bytes: 100,
    used_bytes: 40,
    available_bytes: 60,
    used_percent: 40,
    available: true,
    observed_at: successfulObservedAt,
    shared_with_home: false,
  };
  const initialDisk = {
    ...disk,
    observed_at: initialRoot.observed_at,
    temporary_roots: [initialRoot],
  };
  mocks.fetchDisk.mockResolvedValueOnce(initialDisk);
  const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
  await waitFor(() => expect(result.current.disk?.temporary_roots).toEqual([initialRoot]));

  mocks.fetchDisk.mockRejectedValueOnce(new Error("network unavailable"));
  await act(async () => result.current.reload(["disk"]).catch(() => undefined));
  expect(result.current.disk?.temporary_roots?.[0]).toMatchObject({
    path: "/tmp",
    available: true,
    used_percent: 40,
    observed_at: initialRoot.observed_at,
    stale: true,
  });
  expect(result.current.sectionErrors.disk).toBe("network unavailable");

  const recoveredRoot = { ...initialRoot, used_bytes: 50, available_bytes: 50, used_percent: 50 };
  mocks.fetchDisk.mockResolvedValueOnce({
    ...initialDisk,
    observed_at: "2026-09-28T10:02:00.000Z",
    temporary_roots: [recoveredRoot],
  });
  await act(async () => result.current.reload(["disk"]));
  expect(result.current.disk?.temporary_roots?.[0]).toMatchObject({
    ...recoveredRoot,
    stale: false,
  });
  expect(result.current.sectionErrors.disk).toBeNull();
});

it("retains stale roots when discovery fails but clears them after an authoritative empty response", () => {
  const previous = {
    ...disk,
    temporary_roots: [
      {
        requested_path: "/tmp",
        path: "/tmp",
        total_bytes: 100,
        used_bytes: 40,
        available_bytes: 60,
        used_percent: 40,
        available: true,
        observed_at: successfulObservedAt,
        shared_with_home: false,
      },
    ],
  };
  const discoveryFailure = mergeStorageDiskCapacity(previous, {
    ...disk,
    temporary_roots: [],
    temporary_roots_warning: "temporary storage paths unavailable",
  });
  expect(discoveryFailure.temporary_roots).toHaveLength(1);
  expect(discoveryFailure.temporary_roots?.[0]).toMatchObject({
    available: true,
    used_percent: 40,
    observed_at: successfulObservedAt,
    stale: true,
  });
  expect(discoveryFailure.temporary_roots_warning).toBe("temporary storage paths unavailable");

  const authoritativeEmpty = mergeStorageDiskCapacity(previous, {
    ...disk,
    temporary_roots: [],
  });
  expect(authoritativeEmpty.temporary_roots).toEqual([]);
});
