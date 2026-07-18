import { queryOptions } from "@tanstack/react-query";
import {
  fetchBackups,
  fetchDatabaseStats,
  fetchDiskUsage,
  fetchLogFiles,
  fetchLogTail,
  fetchRestartCapability,
  fetchStorageOverview,
  fetchStorageQuarantine,
  fetchStorageRuns,
  fetchSystemInfo,
  fetchSystemJob,
  fetchUpdates,
} from "@/lib/api/domains/system-api";
import { qk } from "../keys";
import { withSignal } from "./utils";
import type { SystemJob, SystemMetricsSnapshot } from "@/lib/types/system";

export function systemInfoQueryOptions() {
  return queryOptions({
    queryKey: qk.system.info(),
    queryFn: ({ signal }) => fetchSystemInfo(withSignal(signal)),
  });
}

export function diskUsageQueryOptions() {
  return queryOptions({
    queryKey: qk.system.diskUsage(),
    queryFn: ({ signal }) => fetchDiskUsage(withSignal(signal)),
  });
}

export function databaseStatsQueryOptions() {
  return queryOptions({
    queryKey: qk.system.database(),
    queryFn: ({ signal }) => fetchDatabaseStats(withSignal(signal)),
  });
}

export function backupsQueryOptions() {
  return queryOptions({
    queryKey: qk.system.backups(),
    queryFn: ({ signal }) => fetchBackups(withSignal(signal)),
  });
}

export function logFilesQueryOptions() {
  return queryOptions({
    queryKey: qk.system.logFiles(),
    queryFn: ({ signal }) => fetchLogFiles(withSignal(signal)),
  });
}

export function logTailQueryOptions(n = 1000) {
  return queryOptions({
    queryKey: qk.system.logTail(n),
    queryFn: ({ signal }) => fetchLogTail(n, withSignal(signal)),
  });
}

export function systemJobsQueryOptions() {
  return queryOptions({
    queryKey: qk.system.jobs(),
    queryFn: async (): Promise<Record<string, SystemJob>> => ({}),
    enabled: false,
  });
}

export function systemJobQueryOptions(jobId: string) {
  return queryOptions({
    queryKey: qk.system.job(jobId),
    queryFn: ({ signal }) => fetchSystemJob(jobId, withSignal(signal)),
    enabled: Boolean(jobId),
  });
}

export function systemMetricsQueryOptions() {
  return queryOptions({
    queryKey: qk.system.metrics(),
    queryFn: async (): Promise<SystemMetricsSnapshot | null> => null,
    enabled: false,
  });
}

export function updatesQueryOptions() {
  return queryOptions({
    queryKey: qk.system.updates(),
    queryFn: ({ signal }) => fetchUpdates(withSignal(signal)),
  });
}

export function restartCapabilityQueryOptions() {
  return queryOptions({
    queryKey: qk.system.restartCapability(),
    queryFn: ({ signal }) => fetchRestartCapability(withSignal(signal)),
  });
}

export function storageOverviewQueryOptions() {
  return queryOptions({
    queryKey: qk.system.storageOverview(),
    queryFn: ({ signal }) => fetchStorageOverview(withSignal(signal)),
  });
}

export function storageRunsQueryOptions(limit = 20) {
  return queryOptions({
    queryKey: qk.system.storageRuns(limit),
    queryFn: ({ signal }) => fetchStorageRuns(limit, withSignal(signal)),
  });
}

export function storageQuarantineQueryOptions() {
  return queryOptions({
    queryKey: qk.system.storageQuarantine(),
    queryFn: ({ signal }) => fetchStorageQuarantine(withSignal(signal)),
  });
}
