"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  inspectRepositoryCloneSource,
  type RepositoryCloneSourceResponse,
} from "@/lib/api/domains/workspace-api";

export type RepositoryCloneSourceCandidate = {
  key: string;
  repositoryId?: string;
  localPath?: string;
};

export type RepositoryCloneSourceState =
  | { status: "checking" }
  | { status: "ready"; result: RepositoryCloneSourceResponse }
  | { status: "unavailable"; result?: RepositoryCloneSourceResponse }
  | { status: "error"; error: Error };

const MAX_CONCURRENT_INSPECTIONS = 4;

function candidateIdentity(candidate: RepositoryCloneSourceCandidate): string {
  return `${candidate.repositoryId ?? ""}\u0000${candidate.localPath ?? ""}`;
}

export function useRepositoryCloneSource(
  workspaceId: string | null,
  candidates: RepositoryCloneSourceCandidate[],
  enabled = true,
) {
  const [states, setStates] = useState<Record<string, RepositoryCloneSourceState>>({});
  const [refreshVersion, setRefreshVersion] = useState(0);
  const generationRef = useRef(0);
  const candidatesKey = useMemo(
    () =>
      candidates
        .map((candidate) => `${candidate.key}:${candidateIdentity(candidate)}`)
        .join("\u0001"),
    [candidates],
  );

  const inspect = useCallback(
    async (candidate: RepositoryCloneSourceCandidate, generation: number, signal: AbortSignal) => {
      if (!workspaceId || signal.aborted) return;
      try {
        const result = await inspectRepositoryCloneSource(
          workspaceId,
          { repositoryId: candidate.repositoryId, localPath: candidate.localPath },
          { init: { signal } },
        );
        if (signal.aborted || generationRef.current !== generation) return;
        setStates((current) => ({
          ...current,
          [candidate.key]: result.ready
            ? { status: "ready", result }
            : { status: "unavailable", result },
        }));
      } catch (error) {
        if (signal.aborted || generationRef.current !== generation) return;
        setStates((current) => ({
          ...current,
          [candidate.key]: {
            status: "error",
            error: error instanceof Error ? error : new Error(String(error)),
          },
        }));
      }
    },
    [workspaceId],
  );

  useEffect(() => {
    const generation = ++generationRef.current;
    const controller = new AbortController();
    if (!enabled || !workspaceId || candidates.length === 0) {
      setStates({});
      return () => controller.abort();
    }
    setStates(
      Object.fromEntries(candidates.map((candidate) => [candidate.key, { status: "checking" }])),
    );
    let cursor = 0;
    let active = 0;
    let settled = false;
    const runNext = () => {
      if (settled || controller.signal.aborted || generationRef.current !== generation) return;
      if (cursor >= candidates.length && active === 0) {
        settled = true;
        return;
      }
      while (active < MAX_CONCURRENT_INSPECTIONS && cursor < candidates.length) {
        const candidate = candidates[cursor++];
        active += 1;
        void inspect(candidate, generation, controller.signal).finally(() => {
          active -= 1;
          runNext();
        });
      }
    };
    runNext();
    return () => controller.abort();
  }, [candidates, candidatesKey, enabled, inspect, refreshVersion, workspaceId]);

  const refresh = useCallback(() => {
    generationRef.current += 1;
    setStates({});
    setRefreshVersion((version) => version + 1);
  }, []);

  return { states, refresh };
}

export function cloneSourceStateFor(
  states: Record<string, RepositoryCloneSourceState>,
  key: string,
): RepositoryCloneSourceState | undefined {
  return states[key];
}
