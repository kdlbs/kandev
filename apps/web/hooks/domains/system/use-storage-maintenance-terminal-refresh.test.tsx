import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StorageMaintenanceSettings } from "@/lib/types/system";
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

import { useStorageMaintenance } from "./use-storage-maintenance";

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
  mocks.save.mockResolvedValue({ settings } satisfies { settings: StorageMaintenanceSettings });
  mocks.run.mockResolvedValue({ job_id: cleanupJobId });
  mocks.purge.mockResolvedValue({ job_id: "purge-job" });
});

describe("useStorageMaintenance terminal refresh", () => {
  it("surfaces and retries a failed refresh after a cleanup job finishes", async () => {
    mocks.fetchJob.mockResolvedValue({
      ...cleanupJob,
      state: "succeeded",
      ended_at: "2026-07-15T00:01:00Z",
    });
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    mocks.fetchOverview.mockRejectedValueOnce(new Error("refresh unavailable"));

    await act(async () => {
      await result.current.runNow();
    });

    await waitFor(() => expect(String(result.current.error)).toContain("refresh unavailable"));
    await waitFor(() => expect(mocks.fetchOverview).toHaveBeenCalledTimes(3), { timeout: 2500 });
    await waitFor(() => expect(result.current.error).toBeNull());
  });

  it("backs off and stops after six terminal refresh attempts", async () => {
    vi.useFakeTimers();
    try {
      mocks.fetchJob.mockResolvedValue({
        ...cleanupJob,
        state: "succeeded",
        ended_at: "2026-07-15T00:01:00Z",
      });
      const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
      await act(async () => {
        await Promise.resolve();
      });
      expect(result.current.overview).toEqual(overview);

      mocks.fetchOverview.mockRejectedValue(new Error("refresh unavailable"));
      await act(async () => {
        await result.current.runNow();
        await vi.advanceTimersByTimeAsync(0);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(2);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(999);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(2);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(3);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1999);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(3);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(4);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(3999);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(4);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(5);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(7999);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(5);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(6);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(7999);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(6);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(7);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(60000);
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(7);
    } finally {
      vi.useRealTimers();
    }
  });
});
