import { fetchJson, type ApiRequestOptions } from "../client";
import { getBackendConfig } from "@/lib/config";
import type {
  SystemInfo,
  DiskUsageResponse,
  DatabaseStats,
  SnapshotInfo,
  LogFileInfo,
  LogTailResponse,
  UpdatesResponse,
  JobAcceptResponse,
  RestartCapability,
  RestartResponse,
} from "@/lib/types/system";

const SYSTEM_BASE = "/api/v1/system";

// --- Info ---------------------------------------------------------------

export function fetchSystemInfo(options?: ApiRequestOptions): Promise<SystemInfo> {
  return fetchJson<SystemInfo>(`${SYSTEM_BASE}/info`, options);
}

// --- Disk usage ---------------------------------------------------------

export function fetchDiskUsage(options?: ApiRequestOptions): Promise<DiskUsageResponse> {
  return fetchJson<DiskUsageResponse>(`${SYSTEM_BASE}/disk-usage`, options);
}

export function refreshDiskUsage(options?: ApiRequestOptions): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/disk-usage/refresh`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export function openDataFolder(options?: ApiRequestOptions): Promise<{ path: string }> {
  return fetchJson<{ path: string }>(`${SYSTEM_BASE}/disk-usage/open`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

// --- Database -----------------------------------------------------------

export function fetchDatabaseStats(options?: ApiRequestOptions): Promise<DatabaseStats> {
  return fetchJson<DatabaseStats>(`${SYSTEM_BASE}/database`, options);
}

export function vacuumDatabase(options?: ApiRequestOptions): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/database/vacuum`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export function optimizeDatabase(options?: ApiRequestOptions): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/database/optimize`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export function resetDatabase(
  confirm: string,
  options?: ApiRequestOptions,
): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/database/reset`, {
    ...options,
    init: { method: "POST", body: JSON.stringify({ confirm }), ...(options?.init ?? {}) },
  });
}

// --- Backups ------------------------------------------------------------

export async function fetchBackups(options?: ApiRequestOptions): Promise<SnapshotInfo[]> {
  const res = await fetchJson<{ snapshots: SnapshotInfo[] }>(`${SYSTEM_BASE}/backups`, options);
  return res.snapshots ?? [];
}

export function createBackup(options?: ApiRequestOptions): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/backups`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export function restoreBackup(
  name: string,
  confirm: string,
  options?: ApiRequestOptions,
): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(
    `${SYSTEM_BASE}/backups/${encodeURIComponent(name)}/restore`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify({ confirm }), ...(options?.init ?? {}) },
    },
  );
}

export function deleteBackup(name: string, options?: ApiRequestOptions): Promise<void> {
  return fetchJson<void>(`${SYSTEM_BASE}/backups/${encodeURIComponent(name)}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

/**
 * Builds an absolute URL for downloading a snapshot. Snapshot downloads stream
 * binary data, so they cannot reuse fetchJson — they are typically wired into
 * <a href={...} download> or window.open().
 */
export function buildBackupDownloadUrl(name: string, baseUrl?: string): string {
  const root = baseUrl ?? getBackendConfig().apiBaseUrl;
  return `${root}${SYSTEM_BASE}/backups/${encodeURIComponent(name)}/download`;
}

// --- Logs ---------------------------------------------------------------

export async function fetchLogFiles(options?: ApiRequestOptions): Promise<LogFileInfo[]> {
  const res = await fetchJson<{ files: LogFileInfo[] }>(`${SYSTEM_BASE}/logs`, options);
  return res.files ?? [];
}

export function fetchLogTail(n = 1000, options?: ApiRequestOptions): Promise<LogTailResponse> {
  return fetchJson<LogTailResponse>(`${SYSTEM_BASE}/logs/tail?n=${encodeURIComponent(String(n))}`, {
    cache: "no-store",
    ...options,
  });
}

export function buildLogDownloadUrl(name: string, baseUrl?: string): string {
  const root = baseUrl ?? getBackendConfig().apiBaseUrl;
  return `${root}${SYSTEM_BASE}/logs/${encodeURIComponent(name)}/download`;
}

// --- Jobs ---------------------------------------------------------------

export function fetchSystemJob(
  jobId: string,
  options?: ApiRequestOptions,
): Promise<import("@/lib/types/system").SystemJob> {
  return fetchJson<import("@/lib/types/system").SystemJob>(
    `${SYSTEM_BASE}/jobs/${encodeURIComponent(jobId)}`,
    { cache: "no-store", ...options },
  );
}

// --- Updates ------------------------------------------------------------

export function fetchUpdates(options?: ApiRequestOptions): Promise<UpdatesResponse> {
  return fetchJson<UpdatesResponse>(`${SYSTEM_BASE}/updates`, options);
}

export function checkUpdates(options?: ApiRequestOptions): Promise<UpdatesResponse> {
  return fetchJson<UpdatesResponse>(`${SYSTEM_BASE}/updates/check`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export function applyUpdate(
  confirm = "UPDATE",
  options?: ApiRequestOptions,
): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/updates/apply`, {
    ...options,
    // Spread caller init first so the required method/body can't be overridden.
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify({ confirm }) },
  });
}

// --- Restart ------------------------------------------------------------

export function fetchRestartCapability(options?: ApiRequestOptions): Promise<RestartCapability> {
  return fetchJson<RestartCapability>(`${SYSTEM_BASE}/restart-capability`, {
    cache: "no-store",
    ...options,
  });
}

export function requestRestart(options?: ApiRequestOptions): Promise<RestartResponse> {
  return fetchJson<RestartResponse>(`${SYSTEM_BASE}/restart`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}
