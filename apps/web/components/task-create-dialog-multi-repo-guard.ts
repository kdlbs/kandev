"use client";

import { useEffect } from "react";
import type { Executor } from "@/lib/types/http";

export function useMultiRepoGuardEffect(
  open: boolean,
  executorProfileId: string,
  setExecutorProfileId: (id: string) => void,
  executors: Executor[],
  selectedRepoCount: number,
) {
  useEffect(() => {
    if (!open || !executorProfileId || executors.length === 0) return;
    if (selectedRepoCount <= 1) return;
    const profileToType = new Map<string, string | undefined>();
    const worktreeProfileIds: string[] = [];
    for (const e of executors) {
      for (const p of e.profiles ?? []) {
        const type = p.executor_type ?? e.type;
        profileToType.set(p.id, type);
        if (type === "worktree") worktreeProfileIds.push(p.id);
      }
    }
    if (worktreeProfileIds.length === 0) return;
    if (profileToType.get(executorProfileId) === "worktree") return;
    void Promise.resolve().then(() => setExecutorProfileId(worktreeProfileIds[0]));
  }, [open, executorProfileId, executors, selectedRepoCount, setExecutorProfileId]);
}
