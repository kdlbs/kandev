"use client";

import { useEffect, useRef } from "react";
import { subscribeAutomationRunRecorded } from "@/lib/automation-run-recorded";

/** Runs recorded together, such as one GitHub poll, share one refresh. */
export const AUTOMATION_RUN_RECORDED_DEBOUNCE_MS = 300;

/**
 * Re-run `refresh` when a run is recorded in `workspaceId`, optionally only
 * for `automationId`. The run pages otherwise poll only while a run is open,
 * so a run recorded while nothing was open would stay hidden until a reload.
 */
export function useAutomationRunRecorded(
  workspaceId: string | undefined,
  automationId: string | undefined,
  refresh: () => void,
): void {
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  useEffect(() => {
    if (!workspaceId) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const unsubscribe = subscribeAutomationRunRecorded((run) => {
      if (run.workspace_id !== workspaceId) return;
      if (automationId && run.automation_id !== automationId) return;
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => refreshRef.current(), AUTOMATION_RUN_RECORDED_DEBOUNCE_MS);
    });
    return () => {
      unsubscribe();
      if (timer) clearTimeout(timer);
    };
  }, [workspaceId, automationId]);
}
