import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { StorageMaintenanceSettings, StorageOverviewResponse } from "@/lib/types/system";

const mocks = vi.hoisted(() => ({
  adopt: vi.fn(),
  analyze: vi.fn(),
  deleteEntry: vi.fn(),
  fetchJob: vi.fn(),
  fetchOverview: vi.fn(),
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
  fetchSystemJob: mocks.fetchJob,
  fetchStorageOverview: mocks.fetchOverview,
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
  workspaces: { enabled: true },
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
  last_run: null,
};

const cleanupJob = {
  id: "cleanup-job",
  kind: "storage-cleanup",
  state: "running",
  started_at: "2026-07-15T00:00:00Z",
};

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.fetchOverview.mockResolvedValue(overview);
  mocks.fetchRuns.mockResolvedValue([]);
  mocks.fetchQuarantine.mockResolvedValue([]);
  mocks.fetchJob.mockResolvedValue(cleanupJob);
  mocks.save.mockResolvedValue({ settings });
  // Keep cleanup jobs deterministic for controller action tests.
  mocks.run.mockResolvedValue({ job_id: "cleanup-job" });
});

describe("useStorageMaintenance", () => {
  it("loads overview, run history, and quarantine through the domain controller", async () => {
    const { result } = renderHook(() => useStorageMaintenance(), { wrapper });
    await waitFor(() => expect(result.current.overview).toEqual(overview));
    expect(mocks.fetchRuns).toHaveBeenCalledWith(20);
    expect(mocks.fetchQuarantine).toHaveBeenCalledTimes(1);
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
    await waitFor(() => expect(result.current.cleanupJob?.id).toBe("cleanup-job"));

    mocks.run.mockRejectedValueOnce(new Error("storage maintenance is busy"));
    await act(async () => {
      await result.current.runNow();
    });

    expect(result.current.cleanupJob).toBeUndefined();
    expect(result.current.error).toBe("storage maintenance is busy");
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
});
