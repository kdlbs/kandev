import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import type {
  StorageMaintenanceSettings,
  StorageOverviewResponse,
  StoragePolicyResponse,
} from "@/lib/types/system";
import {
  cleanupJob,
  cleanupJobId,
  deferred,
  disk,
  overview,
  settings,
  STORAGE_BUSY_ERROR_MESSAGE,
  TEST_COMMAND_BUSY_LABEL,
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

import {
  settingsWithDockerAcknowledgement,
  useStorageMaintenance,
} from "./use-storage-maintenance";

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
  // Keep cleanup jobs deterministic for controller action tests.
  mocks.run.mockResolvedValue({ job_id: cleanupJobId });
  mocks.purge.mockResolvedValue({ job_id: "purge-job" });
});

describe("useStorageMaintenance loading", () => {
  it("publishes fast sections before a cold overview scan finishes", async () => {
    const overviewRequest = deferred<StorageOverviewResponse>();
    mocks.fetchOverview.mockReturnValueOnce(overviewRequest.promise);

    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });

    await waitFor(() => expect(result.current.policy?.settings).toEqual(settings));
    expect(result.current.loading).toMatchObject({
      policy: false,
      runs: false,
      quarantine: false,
      overview: true,
      disk: false,
    });
    expect(result.current.overview).toBeNull();

    await act(async () => {
      overviewRequest.resolve(overview);
      await overviewRequest.promise;
    });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    expect(result.current.loading.overview).toBe(false);
  });

  it("loads overview, run history, and quarantine through the domain controller", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    expect(mocks.fetchRuns).toHaveBeenCalledWith(20);
    expect(mocks.fetchQuarantine).toHaveBeenCalledTimes(1);
    expect(mocks.fetchDisk).toHaveBeenCalledTimes(1);
    expect(result.current.pendingAction).toBeNull();
  });
});

describe("useStorageMaintenance analysis updates", () => {
  it("reloads only the overview when a live analysis event advances the revision", async () => {
    let bumpRevision: (() => void) | undefined;
    function CaptureRevisionAction() {
      bumpRevision = useAppStoreApi().getState().bumpSystemStorageAnalysisRevision;
      return null;
    }
    const capturedWrapper = ({ children }: { children: ReactNode }) => (
      <StateProvider>
        <CaptureRevisionAction />
        {children}
      </StateProvider>
    );
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper: capturedWrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    mocks.fetchOverview.mockClear();
    mocks.fetchPolicy.mockClear();
    mocks.fetchDisk.mockClear();
    mocks.fetchRuns.mockClear();
    mocks.fetchQuarantine.mockClear();

    await act(async () => {
      bumpRevision?.();
      await Promise.resolve();
    });
    await waitFor(() => expect(mocks.fetchOverview).toHaveBeenCalledTimes(1));
    expect(mocks.fetchPolicy).not.toHaveBeenCalled();
    expect(mocks.fetchDisk).not.toHaveBeenCalled();
    expect(mocks.fetchRuns).not.toHaveBeenCalled();
    expect(mocks.fetchQuarantine).not.toHaveBeenCalled();
  });

  it("polls a scanning overview every 1.5 seconds and stops after completion", async () => {
    vi.useFakeTimers();
    try {
      const scanning = {
        ...overview,
        summary: null,
        analyzed_at: null,
        analysis: { ...overview.analysis, state: "scanning" as const },
      };
      mocks.fetchOverview.mockReset().mockResolvedValueOnce(scanning).mockResolvedValue(overview);
      const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
      await act(async () => {
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(result.current.overview?.analysis.state).toBe("scanning");
      mocks.fetchOverview.mockClear();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1499);
      });
      expect(mocks.fetchOverview).not.toHaveBeenCalled();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(1);
      expect(result.current.overview?.analysis.state).toBe("ready");
      mocks.fetchOverview.mockClear();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(3000);
      });
      expect(mocks.fetchOverview).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("useStorageMaintenance analysis refresh", () => {
  it("requests a new overview when its refresh deadline arrives", async () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date("2026-07-23T12:00:00Z"));
      const dueOverview = {
        ...overview,
        analysis: {
          ...overview.analysis,
          refresh_due_at: "2026-07-23T12:00:00.500Z",
        },
      };
      mocks.fetchOverview.mockReset().mockResolvedValue(dueOverview);
      const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
      await act(async () => {
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(result.current.overview).toEqual(dueOverview);
      mocks.fetchOverview.mockClear();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(499);
      });
      expect(mocks.fetchOverview).not.toHaveBeenCalled();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(mocks.fetchOverview).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("useStorageMaintenance actions", () => {
  it("owns confirmed settings persistence and success feedback", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    await act(async () => {
      await result.current.save(settings, "DEDICATED");
    });
    expect(mocks.save).toHaveBeenCalledWith(settings, "DEDICATED");
    expect(mocks.toast).toHaveBeenCalledWith({
      title: "Storage policy saved",
      variant: "success",
    });
  });

  it("refreshes policy after save without starting another overview scan", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    mocks.fetchOverview.mockClear();
    mocks.fetchPolicy.mockClear();

    await act(async () => {
      await result.current.save(settings);
    });

    expect(mocks.fetchPolicy).toHaveBeenCalledTimes(1);
    expect(mocks.fetchOverview).not.toHaveBeenCalled();
  });

  it("starts eligible and forced quarantine bulk jobs", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));

    await act(async () => {
      await result.current.clearEligible();
      await result.current.forceClearAll();
    });

    expect(mocks.purge).toHaveBeenNthCalledWith(1, "eligible");
    expect(mocks.purge).toHaveBeenNthCalledWith(2, "all");
  });

  it("rejects failed saves so the settings coordinator can keep the draft dirty", async () => {
    mocks.save.mockRejectedValueOnce(new Error("save unavailable"));
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));

    await expect(result.current.save(settings)).rejects.toThrow("save unavailable");

    await waitFor(() => expect(result.current.error).toBe("save unavailable"));
  });

  it("clearing Docker acknowledgement also disables global cleanup", () => {
    const updated = settingsWithDockerAcknowledgement(settings, false);
    expect(updated.docker).toMatchObject({
      dedicated_daemon_acknowledged: false,
      build_cache_enabled: false,
      unused_images_enabled: false,
    });
  });

  it("passes a named resource through for explicit cleanup", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    await act(async () => {
      await result.current.runNow(["go_cache"]);
    });
    expect(mocks.run).toHaveBeenCalledWith(["go_cache"]);
  });

  it("does not retain the prior cleanup job when a second run is rejected", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    await act(async () => {
      await result.current.runNow();
    });
    await waitFor(() => expect(result.current.cleanupJob?.id).toBe(cleanupJobId));

    mocks.run.mockRejectedValueOnce(new Error("storage maintenance is busy"));
    await act(async () => {
      await result.current.runNow();
    });

    expect(result.current.cleanupJob).toBeUndefined();
    expect(result.current.error).toBe("storage maintenance is busy");
  });
});

describe("useStorageMaintenance disk isolation", () => {
  it("keeps the other sections usable when the disk request fails", async () => {
    mocks.fetchDisk.mockRejectedValueOnce(new Error("disk unavailable"));
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });

    await waitFor(() => expect(result.current.sectionErrors.disk).toBe("disk unavailable"));
    expect(result.current.policy?.settings).toEqual(settings);
    expect(result.current.overview).toEqual(overview);
    expect(result.current.runs).toEqual([]);
    expect(result.current.quarantine).toEqual([]);
    expect(result.current.loading.disk).toBe(false);
  });
});

describe("useStorageMaintenance busy feedback", () => {
  it("retains labeled busy feedback and reruns the same resources with force", async () => {
    mocks.run.mockRejectedValueOnce(
      new ApiError(STORAGE_BUSY_ERROR_MESSAGE, 409, {
        busy_resources: [{ kind: "test_command", label: TEST_COMMAND_BUSY_LABEL }],
        force_available: true,
      }),
    );
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));

    await act(async () => {
      await result.current.runNow(["go_cache"]);
    });
    expect(result.current.busy).toEqual({
      resources: [{ kind: "test_command", label: TEST_COMMAND_BUSY_LABEL }],
      forceAvailable: true,
      resourceSelection: ["go_cache"],
    });

    await act(async () => {
      await result.current.runAnyway();
    });
    expect(mocks.run).toHaveBeenNthCalledWith(2, ["go_cache"], true);
  });

  it("restores busy feedback when the forced retry is rejected", async () => {
    const initialBusyError = new ApiError(STORAGE_BUSY_ERROR_MESSAGE, 409, {
      busy_resources: [{ kind: "test_command", label: TEST_COMMAND_BUSY_LABEL }],
      force_available: true,
    });
    const forcedBusyError = new ApiError(STORAGE_BUSY_ERROR_MESSAGE, 409, {
      busy_resources: [{ kind: "maintenance_running", label: "Storage maintenance is running" }],
      force_available: false,
    });
    mocks.run.mockRejectedValueOnce(initialBusyError).mockRejectedValueOnce(forcedBusyError);
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));

    await act(async () => {
      await result.current.runNow(["go_cache"]);
    });
    await act(async () => {
      await result.current.runAnyway();
    });

    expect(result.current.busy).toEqual({
      resources: [{ kind: "maintenance_running", label: "Storage maintenance is running" }],
      forceAvailable: false,
      resourceSelection: ["go_cache"],
    });
    expect(mocks.toast).not.toHaveBeenCalledWith(
      expect.objectContaining({ title: "Storage action failed" }),
    );
  });

  it("clears stale busy feedback when another storage action starts", async () => {
    mocks.run.mockRejectedValueOnce(
      new ApiError(STORAGE_BUSY_ERROR_MESSAGE, 409, {
        busy_resources: [{ kind: "test_command", label: TEST_COMMAND_BUSY_LABEL }],
        force_available: true,
      }),
    );
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));

    await act(async () => {
      await result.current.runNow();
    });
    expect(result.current.busy).not.toBeNull();

    await act(async () => {
      await result.current.save(settings);
    });
    expect(result.current.busy).toBeNull();
  });
});

describe("useStorageMaintenance pending action tracking", () => {
  it("returns to a pending resource action after an overlapping save finishes", async () => {
    const pendingRun = deferred<{ job_id: string }>();
    const pendingSave = deferred<{ settings: StorageMaintenanceSettings }>();
    mocks.run.mockReturnValueOnce(pendingRun.promise);
    mocks.save.mockReturnValueOnce(pendingSave.promise);
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));

    let runPromise!: Promise<void>;
    let savePromise!: Promise<void>;
    await act(async () => {
      runPromise = result.current.runNow();
      savePromise = result.current.save(settings);
    });
    await waitFor(() => expect(result.current.pendingAction).toBe("save"));

    await act(async () => {
      pendingSave.resolve({ settings });
      await savePromise;
    });
    expect(result.current.pendingAction).toBe("run");

    await act(async () => {
      pendingRun.resolve({ job_id: cleanupJobId });
      await runPromise;
    });
    expect(result.current.pendingAction).toBeNull();
  });

  it("keeps an overlapping save pending when the resource request finishes first", async () => {
    const pendingRun = deferred<{ job_id: string }>();
    const pendingSave = deferred<{ settings: StorageMaintenanceSettings }>();
    mocks.run.mockReturnValueOnce(pendingRun.promise);
    mocks.save.mockReturnValueOnce(pendingSave.promise);
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));

    let runPromise!: Promise<void>;
    let savePromise!: Promise<void>;
    await act(async () => {
      runPromise = result.current.runNow();
      savePromise = result.current.save(settings);
    });
    await waitFor(() => expect(result.current.pendingAction).toBe("save"));

    await act(async () => {
      pendingRun.resolve({ job_id: cleanupJobId });
      await runPromise;
    });
    expect(result.current.pendingAction).toBe("save");

    await act(async () => {
      pendingSave.resolve({ settings });
      await savePromise;
    });
    expect(result.current.pendingAction).toBeNull();
  });
});

describe("useStorageMaintenance reload ordering", () => {
  it("does not let an older reload overwrite a newer result", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    let resolveOlder!: (value: StorageOverviewResponse) => void;
    const olderResponse = new Promise<StorageOverviewResponse>((resolve) => {
      resolveOlder = resolve;
    });
    const newerOverview = {
      ...overview,
      settings: { ...overview.settings, idle_for_minutes: 22 },
    };
    mocks.fetchOverview.mockReturnValueOnce(olderResponse).mockResolvedValueOnce(newerOverview);

    let olderReload!: Promise<void>;
    await act(async () => {
      olderReload = result.current.reload();
      await result.current.reload();
    });
    await waitFor(() => expect(result.current.overview).toEqual(newerOverview));
    await act(async () => {
      resolveOlder(overview);
      await olderReload;
    });

    expect(result.current.overview).toEqual(newerOverview);
  });

  it("does not surface a stale reload failure after a newer result commits", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    const olderResponse = deferred<StorageOverviewResponse>();
    const newerOverview = {
      ...overview,
      settings: { ...overview.settings, idle_for_minutes: 24 },
    };
    mocks.fetchOverview
      .mockReturnValueOnce(olderResponse.promise)
      .mockResolvedValueOnce(newerOverview);

    let olderReload!: Promise<void>;
    await act(async () => {
      olderReload = result.current.reload(["overview"]);
      await result.current.reload(["overview"]);
    });
    await waitFor(() => expect(result.current.overview).toEqual(newerOverview));

    await act(async () => {
      olderResponse.reject(new Error("stale overview unavailable"));
      await olderReload;
    });

    expect(result.current.overview).toEqual(newerOverview);
    expect(result.current.sectionErrors.overview).toBeNull();
  });

  it("does not let a stale policy response overwrite go-cache adoption", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    const policyResponse = deferred<StoragePolicyResponse>();
    const adoptedSettings = {
      ...settings,
      go_cache: { ...settings.go_cache, adopted_path: "/custom/go-build" },
    };
    const adoptedResponse = { settings: adoptedSettings, capabilities: overview.capabilities };
    mocks.fetchPolicy.mockReturnValueOnce(policyResponse.promise);
    mocks.adopt.mockResolvedValueOnce(adoptedResponse);

    let staleReload!: Promise<void>;
    await act(async () => {
      staleReload = result.current.reload(["policy"]);
      await result.current.adopt("/custom/go-build");
    });
    expect(result.current.policy?.settings).toEqual(adoptedSettings);

    await act(async () => {
      policyResponse.resolve({ settings, capabilities: overview.capabilities });
      await staleReload;
    });

    expect(result.current.policy?.settings).toEqual(adoptedSettings);
  });
});
