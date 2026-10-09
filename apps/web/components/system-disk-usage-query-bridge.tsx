"use client";

import { useLayoutEffect, useMemo, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAppStoreApi } from "@/components/state-provider";
import type { SystemJob } from "@/lib/types/system";
import {
  createDiskUsageQueryKey,
  useSystemInfoQueryIdentity,
} from "@/hooks/domains/system/system-info-query";

const MAX_SEEN_TERMINAL_JOBS = 64;

function isTerminalState(state: SystemJob["state"] | undefined): boolean {
  return state === "succeeded" || state === "failed";
}

export function SystemDiskUsageQueryBridge({ bootId }: { bootId: string | undefined }) {
  const store = useAppStoreApi();
  const queryClient = useQueryClient();
  const identity = useSystemInfoQueryIdentity(bootId);
  const queryKey = useMemo(
    () => createDiskUsageQueryKey(identity),
    [
      identity.apiBaseUrl,
      identity.authMode,
      identity.authenticated,
      identity.bootId,
      identity.userId,
    ],
  );
  const identityKey = JSON.stringify(queryKey.slice(2));
  const activeIdentityRef = useRef(identityKey);
  const generationRef = useRef(0);
  activeIdentityRef.current = identityKey;

  useLayoutEffect(() => {
    const generation = ++generationRef.current;
    let active = true;
    let previousJobs = store.getState().system.jobs;
    const seenIds = new Set<string>();
    const fifo: string[] = [];

    const remember = (jobId: string) => {
      if (seenIds.has(jobId)) return;
      seenIds.add(jobId);
      fifo.push(jobId);
      if (fifo.length > MAX_SEEN_TERMINAL_JOBS) {
        const oldest = fifo.shift();
        if (oldest !== undefined) seenIds.delete(oldest);
      }
    };

    for (const job of Object.values(previousJobs)) {
      if (job.kind === "disk-walk" && isTerminalState(job.state)) remember(job.id);
    }

    const isCurrentSubscription = () =>
      active && generationRef.current === generation && activeIdentityRef.current === identityKey;

    const unsubscribe = store.subscribe((state) => {
      if (!isCurrentSubscription()) return;
      const nextJobs = state.system.jobs;
      if (nextJobs === previousJobs) return;

      const priorJobs = previousJobs;
      previousJobs = nextJobs;
      for (const [jobId, job] of Object.entries(nextJobs)) {
        const previous = priorJobs[jobId];
        if (
          job.kind !== "disk-walk" ||
          !isTerminalState(job.state) ||
          isTerminalState(previous?.state) ||
          seenIds.has(jobId)
        ) {
          continue;
        }

        remember(jobId);
        void revalidateDiskUsage(queryClient, queryKey, isCurrentSubscription);
      }
    });

    return () => {
      active = false;
      unsubscribe();
      if (generationRef.current === generation) generationRef.current += 1;
    };
  }, [identityKey, queryClient, queryKey, store]);

  return null;
}

async function revalidateDiskUsage(
  queryClient: ReturnType<typeof useQueryClient>,
  queryKey: ReturnType<typeof createDiskUsageQueryKey>,
  isCurrentSubscription: () => boolean,
): Promise<void> {
  if (!isCurrentSubscription()) return;
  await queryClient.cancelQueries({ queryKey, exact: true });
  if (!isCurrentSubscription()) return;
  await queryClient.invalidateQueries({ queryKey, exact: true, refetchType: "active" });
}
