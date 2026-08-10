import { fetchJson, type ApiRequestOptions } from "../client";
import { getBackendConfig } from "@/lib/config";
import type {
  SystemInfo,
  DiskUsageResponse,
  DatabaseStats,
  SnapshotInfo,
  DiagnosticBundleJob,
  DiagnosticBundleCapabilities,
  DiagnosticBundleSource,
  DiagnosticSession,
  FrontendLogUploadChunk,
  UpdatesResponse,
  JobAcceptResponse,
  RestartCapability,
  RestartResponse,
  StorageAdoptionResponse,
  StorageMaintenanceRun,
  StorageMaintenanceSettings,
  StorageDiskCapacityResponse,
  StorageOverviewResponse,
  StoragePolicyResponse,
  StorageQuarantineEntry,
  StorageQuarantinePurgeScope,
  StorageSettingsResponse,
  UpdatesChannel,
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

export function createDiagnosticBundle(
  sources: DiagnosticBundleSource[] = ["backend", "frontend"],
  sessionIds: string[] = [],
  options?: ApiRequestOptions,
): Promise<DiagnosticBundleJob> {
  return fetchJson<DiagnosticBundleJob>(`${SYSTEM_BASE}/logs/bundles`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify({
        sources,
        ...(sessionIds.length > 0 ? { session_ids: sessionIds } : {}),
      }),
    },
  });
}

export function fetchDiagnosticBundleCapabilities(
  options?: ApiRequestOptions,
): Promise<DiagnosticBundleCapabilities> {
  return fetchJson<DiagnosticBundleCapabilities>(`${SYSTEM_BASE}/logs/capabilities`, {
    ...options,
    cache: "no-store",
  });
}

export async function fetchDiagnosticACPSessions(
  options?: ApiRequestOptions,
): Promise<DiagnosticSession[]> {
  const response = await fetchJson<{ sessions: DiagnosticSession[] }>(
    `${SYSTEM_BASE}/logs/acp-sessions`,
    { ...options, cache: "no-store" },
  );
  return response.sessions ?? [];
}

export function fetchDiagnosticBundle(
  id: string,
  options?: ApiRequestOptions,
): Promise<DiagnosticBundleJob> {
  return fetchJson<DiagnosticBundleJob>(`${SYSTEM_BASE}/logs/bundles/${encodeURIComponent(id)}`, {
    ...options,
    cache: "no-store",
  });
}

export function uploadFrontendBundleChunk(
  id: string,
  chunk: FrontendLogUploadChunk,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`${SYSTEM_BASE}/logs/bundles/${encodeURIComponent(id)}/frontend`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(chunk) },
  });
}

export function buildDiagnosticBundleDownloadUrl(id: string, baseUrl?: string): string {
  const root = baseUrl ?? getBackendConfig().apiBaseUrl;
  return `${root}${SYSTEM_BASE}/logs/bundles/${encodeURIComponent(id)}/download`;
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

export function saveUpdatesChannel(
  channel: UpdatesChannel,
  options?: ApiRequestOptions,
): Promise<UpdatesResponse> {
  return fetchJson<UpdatesResponse>(`${SYSTEM_BASE}/updates/channel`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PATCH",
      body: JSON.stringify({ channel }),
    },
  });
}

export function applyUpdate(
  confirm: string,
  targetVersion: string,
  options?: ApiRequestOptions,
): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/updates/apply`, {
    ...options,
    // Spread caller init first so the required method/body can't be overridden.
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify({ confirm, target_version: targetVersion }),
    },
  });
}

// --- Restart ------------------------------------------------------------

export function fetchRestartCapability(options?: ApiRequestOptions): Promise<RestartCapability> {
  return fetchJson<RestartCapability>(`${SYSTEM_BASE}/restart-capability`, {
    ...options,
    cache: "no-store",
  });
}

export function requestRestart(options?: ApiRequestOptions): Promise<RestartResponse> {
  return fetchJson<RestartResponse>(`${SYSTEM_BASE}/restart`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST" },
  });
}

// --- Storage maintenance ------------------------------------------------

export function fetchStorageOverview(
  options?: ApiRequestOptions,
): Promise<StorageOverviewResponse> {
  return fetchJson<StorageOverviewResponse>(`${SYSTEM_BASE}/storage`, {
    ...options,
    cache: "no-store",
  });
}

export function fetchStorageDisk(
  options?: ApiRequestOptions,
): Promise<StorageDiskCapacityResponse> {
  return fetchJson<StorageDiskCapacityResponse>(`${SYSTEM_BASE}/storage/disk`, {
    ...options,
    cache: "no-store",
  });
}

export function fetchStoragePolicy(options?: ApiRequestOptions): Promise<StoragePolicyResponse> {
  return fetchJson<StoragePolicyResponse>(`${SYSTEM_BASE}/storage/settings`, {
    ...options,
    cache: "no-store",
  });
}

export function saveStorageSettings(
  settings: StorageMaintenanceSettings,
  dedicatedDockerConfirmation?: "DEDICATED",
  options?: ApiRequestOptions,
): Promise<StorageSettingsResponse> {
  return fetchJson<StorageSettingsResponse>(`${SYSTEM_BASE}/storage/settings`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PATCH",
      body: JSON.stringify({
        settings,
        confirmations: dedicatedDockerConfirmation
          ? { dedicated_docker: dedicatedDockerConfirmation }
          : {},
      }),
    },
  });
}

export function adoptStorageGoCache(
  path: string,
  options?: ApiRequestOptions,
): Promise<StorageAdoptionResponse> {
  return fetchJson<StorageAdoptionResponse>(`${SYSTEM_BASE}/storage/go-cache/adopt`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify({ path, confirm: "ADOPT" }),
    },
  });
}

export function analyzeStorage(options?: ApiRequestOptions): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/storage/analyze`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST" },
  });
}

export function runStorageMaintenance(
  resources?: string[],
  force = false,
  options?: ApiRequestOptions,
): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/storage/run`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify({
        ...(resources?.length ? { resources } : {}),
        ...(force ? { force: true } : {}),
      }),
    },
  });
}

export async function fetchStorageRuns(
  limit = 20,
  options?: ApiRequestOptions,
): Promise<StorageMaintenanceRun[]> {
  const response = await fetchJson<{ runs: StorageMaintenanceRun[] }>(
    `${SYSTEM_BASE}/storage/runs?limit=${encodeURIComponent(String(limit))}`,
    { ...options, cache: "no-store" },
  );
  return response.runs ?? [];
}

export async function fetchStorageQuarantine(
  options?: ApiRequestOptions,
): Promise<StorageQuarantineEntry[]> {
  const response = await fetchJson<{ entries: StorageQuarantineEntry[] }>(
    `${SYSTEM_BASE}/storage/quarantine`,
    { ...options, cache: "no-store" },
  );
  return response.entries ?? [];
}

export async function restoreStorageQuarantine(
  id: string,
  options?: ApiRequestOptions,
): Promise<StorageQuarantineEntry> {
  const response = await fetchJson<{ entry: StorageQuarantineEntry }>(
    `${SYSTEM_BASE}/storage/quarantine/${encodeURIComponent(id)}/restore`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
  return response.entry;
}

export function deleteStorageQuarantine(
  id: string,
  options?: ApiRequestOptions,
): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(
    `${SYSTEM_BASE}/storage/quarantine/${encodeURIComponent(id)}`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "DELETE",
        body: JSON.stringify({ confirm: "DELETE" }),
      },
    },
  );
}

export function purgeStorageQuarantine(
  scope: StorageQuarantinePurgeScope,
  options?: ApiRequestOptions,
): Promise<JobAcceptResponse> {
  return fetchJson<JobAcceptResponse>(`${SYSTEM_BASE}/storage/quarantine`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "DELETE",
      body: JSON.stringify({
        scope,
        confirm: scope === "eligible" ? "DELETE ELIGIBLE" : "DELETE ALL NOW",
      }),
    },
  });
}
