"use client";

import { useEffect } from "react";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
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
    const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, null);
    const pick = lastId && worktreeProfileIds.includes(lastId) ? lastId : worktreeProfileIds[0];
    void Promise.resolve().then(() => setExecutorProfileId(pick));
  }, [open, executorProfileId, executors, selectedRepoCount, setExecutorProfileId]);
}
