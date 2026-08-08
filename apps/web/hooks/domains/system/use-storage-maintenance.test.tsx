import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import type {
  StorageMaintenanceSettings,
  StorageOverviewResponse,
  StoragePolicyResponse,
} from "@/lib/types/system";

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

const settings: StorageMaintenanceSettings = {
  enabled: false,
  check_interval_hours: 24,
  idle_for_minutes: 10,
  orphan_grace_hours: 168,
  quarantine_retention_hours: 168,
  workspaces: { enabled: true, dependency_cleanup_enabled: false },
  kandev_containers: { enabled: true },
  go_cache: { enabled: false, max_bytes: 16106127360, adopted_path: "" },
  docker: {
    dedicated_daemon_acknowledged: true,
    build_cache_enabled: true,
    build_cache_keep_bytes: 10737418240,
    build_cache_unused_hours: 168,
    unused_images_enabled: true,
    unused_images_hours: 168,
  },
};

const overview: StorageOverviewResponse = {
  settings,
  capabilities: {
    managed_go_cache_path: "/data/cache/go-build",
    go_cache_adoption_available: true,
    docker_available: true,
    docker_host: "unix:///var/run/docker.sock",
    host_global_docker_cleanup_allowed: true,
  },
  summary: {
    workspaces: { active_bytes: 10, candidate_bytes: 20 },
    go_cache: { path: "/data/cache/go-build", size_bytes: 30, owned: true, enabled: false },
    quarantine: { count: 2, size_bytes: 35 },
    docker: {
      available: true,
      build_cache_bytes: 40,
      unused_image_bytes: 50,
      managed_container_count: 3,
      managed_container_bytes: 60,
    },
  },
  analyzed_at: "2026-07-23T12:00:00Z",
  last_run: null,
};

const disk = {
  path: "/data",
  total_bytes: 100,
  used_bytes: 80,
  available_bytes: 20,
  used_percent: 80,
  available: true,
};

const cleanupJobId = "cleanup-job";
const cleanupJob = {
  id: cleanupJobId,
  kind: "storage-cleanup",
  state: "running",
  started_at: "2026-07-15T00:00:00Z",
};
const STORAGE_BUSY_ERROR_MESSAGE = "storage cleanup is blocked by active Kandev work";
const TEST_COMMAND_BUSY_LABEL = "A test command is running";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

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

describe("useStorageMaintenance", () => {
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
