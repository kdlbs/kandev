"use client";

import { useEffect } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { fetchSystemJob } from "@/lib/api/domains/system-api";
import type { SystemJob } from "@/lib/types/system";

/**
 * Reads the system-job map from the store. The map is kept in sync with the
 * backend by the central `system.job.update` WS handler in
 * `lib/ws/handlers/system-events.ts`, so this hook is purely a selector.
 *
 * When `kind` is provided, only jobs with that kind are returned.
 *
 * Uses `useShallow` so the derived array reference is reused when its element
 * identities haven't changed — without it, the inline selector returns a
 * fresh array on every render and triggers "Maximum update depth" in any
 * consumer that renders a `JobProgressIndicator`.
 */
export function useSystemJobs(kind?: SystemJob["kind"]): SystemJob[] {
  return useAppStore(
    useShallow((s) => {
      const all = Object.values(s.system.jobs);
      if (!kind) return all;
      return all.filter((j) => j.kind === kind);
    }),
  );
}

const POLL_INTERVAL_MS = 800;

/**
 * Returns a single job by id, or undefined if not tracked.
 *
 * Polling fallback: while `jobId` is set and the locally observed job has not
 * reached a terminal state (succeeded/failed), this hook fetches
 * `GET /api/v1/system/jobs/:id` every ~800ms and upserts the response into
 * the store. This is needed because the primary signal (the
 * `system.job.update` WS broadcast) can be dropped when the WS connection
 * isn't open at the moment the job transitions - typical for fast operations
 * (restore is a tiny copy) and for factory-reset which tears down the
 * orchestrator first.
 */
export function useSystemJob(jobId: string | null | undefined): SystemJob | undefined {
  const job = useAppStore((s) => (jobId ? s.system.jobs[jobId] : undefined));
  const upsertSystemJob = useAppStore((s) => s.upsertSystemJob);

  useEffect(() => {
    if (!jobId) return;
    if (job?.state === "succeeded" || job?.state === "failed") return;

    let cancelled = false;
    const tick = async () => {
      try {
        const fresh = await fetchSystemJob(jobId);
        if (!cancelled) upsertSystemJob(fresh);
      } catch {
        // 404 / network error - keep polling; the WS event may still arrive,
        // or the next tick will succeed.
      }
    };
    // Kick once immediately to close out fast jobs that already finished
    // before we started listening.
    void tick();
    const interval = setInterval(() => void tick(), POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [jobId, job?.state, upsertSystemJob]);

  return job;
}
