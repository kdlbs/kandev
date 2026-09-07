import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type {
  StorageAnalysisState,
  StorageMaintenanceSettings,
  StorageOverviewResponse,
} from "@/lib/types/system";

export const settings: StorageMaintenanceSettings = {
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

export const overview: StorageOverviewResponse = {
  settings,
  capabilities: {
    managed_go_cache_path: "/data/cache/go-build",
    go_cache_adoption_available: true,
    temporary_artifacts_available: true,
    docker_available: true,
    docker_host: "unix:///var/run/docker.sock",
    host_global_docker_cleanup_allowed: true,
  },
  summary: {
    workspaces: { active_bytes: 10, candidate_bytes: 20 },
    go_cache: { path: "/data/cache/go-build", size_bytes: 30, owned: true, enabled: false },
    quarantine: { count: 2, size_bytes: 35 },
    temporary_artifacts: {
      available: true,
      total_count: 0,
      total_bytes: 0,
      active_count: 0,
      active_bytes: 0,
      protected_count: 0,
      protected_bytes: 0,
      stale_count: 0,
      stale_bytes: 0,
      skipped_count: 0,
    },
    docker: {
      available: true,
      build_cache_bytes: 40,
      unused_image_bytes: 50,
      managed_container_count: 3,
      managed_container_bytes: 60,
    },
  },
  analysis: {
    generation: 1,
    state: "ready",
    started_at: "2026-07-23T11:59:00Z",
    completed_at: "2026-07-23T12:00:00Z",
    duration_ms: 60000,
    cache_ttl_seconds: 900,
    refresh_due_at: "2099-07-23T12:15:00Z",
    stale: false,
    error: null,
    progress: { completed_sources: 5, total_sources: 5, sources: {} },
    partial_summary: null,
  } satisfies StorageAnalysisState,
  analyzed_at: "2026-07-23T12:00:00Z",
  last_run: null,
};

export const disk = {
  path: "/data",
  total_bytes: 100,
  used_bytes: 80,
  available_bytes: 20,
  used_percent: 80,
  available: true,
};

export const cleanupJobId = "cleanup-job";
export const cleanupJob = {
  id: cleanupJobId,
  kind: "storage-cleanup",
  state: "running",
  started_at: "2026-07-15T00:00:00Z",
};
// i18n-exempt: test-only busy-error payload
export const STORAGE_BUSY_ERROR_MESSAGE = "storage cleanup is blocked by active Kandev work";
// i18n-exempt: test-only busy-resource label
export const TEST_COMMAND_BUSY_LABEL = "A test command is running";

export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

export function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}
