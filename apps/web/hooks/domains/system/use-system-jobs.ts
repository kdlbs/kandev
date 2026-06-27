"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import { systemJobQueryOptions, systemJobsQueryOptions } from "@/lib/query/query-options/system";
import type { SystemJob } from "@/lib/types/system";

/**
 * Reads the system-job map from the Query cache. The map is kept in sync with
 * the backend by the central `system.job.update` query bridge.
 *
 * When `kind` is provided, only jobs with that kind are returned.
 */
export function useSystemJobs(kind?: SystemJob["kind"]): SystemJob[] {
  const query = useQuery(systemJobsQueryOptions());
  const queryJobs = Object.values(query.data ?? {});
  if (!kind) return queryJobs;
  return queryJobs.filter((job) => job.kind === kind);
}

const POLL_INTERVAL_MS = 800;

/**
 * Returns a single job by id, or undefined if not tracked.
 *
 * Polling fallback: while `jobId` is set and the locally observed job has not
 * reached a terminal state (succeeded/failed), this hook fetches
 * `GET /api/v1/system/jobs/:id` every ~800ms and upserts the response into
 * the Query jobs map. This is needed because the primary signal (the
 * `system.job.update` query bridge) can be missed when the WS connection
 * isn't open at the moment the job transitions - typical for fast operations
 * (restore is a tiny copy) and for factory-reset which tears down the
 * orchestrator first.
 */
export function useSystemJob(jobId: string | null | undefined): SystemJob | undefined {
  const queryClient = useQueryClient();
  const jobsQuery = useQuery(systemJobsQueryOptions());
  const cachedJob = jobId ? jobsQuery.data?.[jobId] : undefined;
  const query = useQuery({
    ...systemJobQueryOptions(jobId ?? ""),
    refetchInterval: (result) => {
      const state = result.state.data?.state ?? cachedJob?.state;
      return state === "succeeded" || state === "failed" ? false : POLL_INTERVAL_MS;
    },
  });
  const job = query.data ?? cachedJob;

  useEffect(() => {
    if (!query.data) return;
    queryClient.setQueryData<Record<string, SystemJob>>(qk.system.jobs(), (prev) => ({
      ...(prev ?? {}),
      [query.data.id]: query.data,
    }));
  }, [query.data, queryClient]);

  return job;
}
