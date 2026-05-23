"use client";

import { useCallback, useEffect, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { fetchDiskUsage, refreshDiskUsage } from "@/lib/api/domains/system-api";

/**
 * Fetch-on-mount hook for `/api/v1/system/disk-usage`. The backend serves the
 * cached value (or null while computing) and publishes a `system.job.update`
 * event with kind=disk-walk when the background walk finishes. That event is
 * already routed into the jobs map by registerSystemEventsHandlers — this hook
 * watches for the transition (running → succeeded/failed) and refetches the
 * usage payload once so the cards swap in the fresh value without polling.
 */
export function useDiskUsage() {
  const diskUsage = useAppStore((s) => s.system.diskUsage);
  const setSystemDiskUsage = useAppStore((s) => s.setSystemDiskUsage);
  // Pick the last disk-walk job we have seen, regardless of id. There is at
  // most one in flight at a time.
  // Wrapped in useShallow so the inline derivation doesn't create a fresh
  // reference each render and trip "Maximum update depth exceeded" in
  // consumers when there are no disk-walk jobs (the array reduces to the
  // same null, but a missing memo would still re-run the equality check).
  const diskWalkJob = useAppStore(
    useShallow((s) => {
      const jobs = Object.values(s.system.jobs).filter((j) => j.kind === "disk-walk");
      return jobs.length > 0 ? jobs[jobs.length - 1] : null;
    }),
  );

  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchDiskUsage({ cache: "no-store" });
      setSystemDiskUsage(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [setSystemDiskUsage]);

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

  // Initial fetch.
  useEffect(() => {
    if (diskUsage) return;
    void reload();
  }, [diskUsage, reload]);

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
    if (!diskUsage?.computing) return;
    const interval = setInterval(() => {
      void reload();
    }, 1500);
    return () => clearInterval(interval);
  }, [diskUsage?.computing, reload]);

  return { diskUsage, isLoading, error, reload, refresh };
}
