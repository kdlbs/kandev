"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { isMergeStatusTransient } from "@/lib/gitlab-merge-state";
import type { ReloadResult } from "./use-mr-feedback";

export const POST_READY_MAX_RELOADS = 5;
export const POST_READY_RELOAD_DELAY_MS = 2000;

type Scope = { identityKey: string; revision: number };

/**
 * The bounded wait that follows a Mark as ready outcome. `holding` is true only
 * while the wait runs. The wait ends on unmount, an identity change or a manual
 * refresh (`revision`), and when a reload is superseded.
 */
export function usePostReadyWait(scope: Scope, reload: () => Promise<ReloadResult>) {
  const [holding, setHolding] = useState(false);
  const token = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reloadRef = useRef(reload);
  const scopeGeneration = useRef(0);
  const mounted = useRef(true);
  const readySeq = useRef(0);

  useEffect(() => {
    reloadRef.current = reload;
  });

  const cancel = useCallback(() => {
    token.current += 1;
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    setHolding(false);
  }, []);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useLayoutEffect(
    () => () => {
      scopeGeneration.current += 1;
      cancel();
    },
    [scope.identityKey, scope.revision, cancel],
  );

  const step = useCallback((own: number, reloads: number) => {
    void reloadRef.current().then((result) => {
      if (own !== token.current) return;
      const settled =
        result.outcome === "ok" && !isMergeStatusTransient(result.mr.detailed_merge_status);
      if (result.outcome === "superseded" || settled || reloads >= POST_READY_MAX_RELOADS) {
        setHolding(false);
        return;
      }
      timer.current = setTimeout(() => step(own, reloads + 1), POST_READY_RELOAD_DELAY_MS);
    });
  }, []);

  const start = useCallback(() => {
    cancel();
    setHolding(true);
    step(token.current, 1);
  }, [cancel, step]);

  /**
   * Runs a Mark as ready request and starts the wait afterwards, unless the
   * panel unmounted, changed identity, refreshed, or a newer request began.
   */
  const trackReady = useCallback(
    async (request: () => Promise<unknown>) => {
      const issuedGeneration = scopeGeneration.current;
      const seq = ++readySeq.current;
      await request();
      if (
        mounted.current &&
        seq === readySeq.current &&
        scopeGeneration.current === issuedGeneration
      ) {
        start();
      }
    },
    [start],
  );

  return { holding, trackReady };
}
