"use client";

import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
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

/** Returns a single job by id, or undefined if not tracked. */
export function useSystemJob(jobId: string | null | undefined): SystemJob | undefined {
  return useAppStore((s) => (jobId ? s.system.jobs[jobId] : undefined));
}
