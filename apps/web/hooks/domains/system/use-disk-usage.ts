"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { refreshDiskUsage } from "@/lib/api/domains/system-api";
import { diskUsageQueryOptions, systemJobsQueryOptions } from "@/lib/query/query-options/system";

/**
 * Fetch-on-mount hook for `/api/v1/system/disk-usage`. The backend serves the
 * cached value (or null while computing) and publishes a `system.job.update`
 * event with kind=disk-walk when the background walk finishes. That event is
 * already routed into the Query jobs map — this hook
 * watches for the transition (running → succeeded/failed) and refetches the
 * usage payload once so the cards swap in the fresh value without polling.
 */
export function useDiskUsage() {
  const query = useQuery(diskUsageQueryOptions());
  const jobsQuery = useQuery(systemJobsQueryOptions());
  // Pick the last disk-walk job we have seen, regardless of id. There is at
  // most one in flight at a time.
  const diskWalkJob = useMemo(() => {
    const jobs = Object.values(jobsQuery.data ?? {}).filter((j) => j.kind === "disk-walk");
    return jobs.at(-1) ?? null;
  }, [jobsQuery.data]);

  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      await query.refetch();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [query]);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      await refreshDiskUsage();
      // Re-read so the `computing: true` flag shows up immediately.
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [reload]);

  useEffect(() => {
    if (!query.error) return;
    setError(query.error instanceof Error ? query.error.message : String(query.error));
  }, [query.error]);

  // Refetch when the disk-walk job reports a terminal state.
  useEffect(() => {
    if (!diskWalkJob) return;
    if (diskWalkJob.state === "succeeded" || diskWalkJob.state === "failed") {
      void reload();
    }
  }, [diskWalkJob, reload]);

  // Polling fallback: keep refetching while the backend reports
  // computing=true. The primary path is the WS system.job.update event above,
  // but if the WS connection is not yet open when the disk-walk job finishes
  // (typical on first page load) the broadcast is dropped and the UI would
  // otherwise sit on "Calculating..." forever. Polling stops as soon as the
  // backend reports the cached value.
  useEffect(() => {
    if (!query.data?.computing) return;
    const interval = setInterval(() => {
      void reload();
    }, 1500);
    return () => clearInterval(interval);
  }, [query.data, reload]);

  return {
    diskUsage: query.data ?? null,
    isLoading: isLoading || (query.isFetching && !query.isSuccess),
    error,
    reload,
    refresh,
  };
}
