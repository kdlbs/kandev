"use client";

import { useCallback, useEffect, useRef } from "react";
import type { ActiveDocument } from "@/lib/state/slices/ui/types";
import type { BuiltInPreset } from "@/lib/state/layout-manager/presets";

const PLAN_CONTEXT_PATH = "plan:context";

// --- Auto-disable plan mode ---

type AutoDisablePlanOpts = {
  resolvedSessionId: string | null;
  taskId: string | null;
  taskStepId: string | null;
  sessionMetaPlanMode: boolean;
  planModeFromStore: boolean;
  applyBuiltInPreset: (preset: BuiltInPreset) => void;
  closeDocument: (sid: string) => void;
  setActiveDocument: (sid: string, doc: ActiveDocument | null) => void;
  setPlanMode: (sid: string, enabled: boolean) => void;
  removeContextFile: (sid: string, path: string) => void;
};

/**
 * Auto-disable plan mode when the task moves to a different workflow step.
 *
 * Two triggers:
 * 1. `taskStepId` changes — the task moved to a new step (via stepper, proceed button, or backend).
 *    This is the primary signal because `task.updated` WS events always fire on step changes.
 * 2. `sessionMetaPlanMode` transitions from true to false — the backend cleared plan_mode from
 *    session metadata. This is a fallback for cases where the step didn't change but plan mode was
 *    explicitly cleared (e.g., on_turn_complete: disable_plan_mode).
 *
 * The disable is idempotent — safe to call even if plan mode is already off.
 */
export function useAutoDisablePlanMode(opts: AutoDisablePlanOpts) {
  const {
    resolvedSessionId,
    taskId,
    taskStepId,
    sessionMetaPlanMode,
    planModeFromStore,
    applyBuiltInPreset,
    closeDocument,
    setActiveDocument,
    setPlanMode,
    removeContextFile,
  } = opts;

  const prevStepIdRef = useRef(taskStepId);
  const prevSessionMetaPlanRef = useRef(sessionMetaPlanMode);

  useEffect(() => {
    const prevStepId = prevStepIdRef.current;
    const wasPlanMode = prevSessionMetaPlanRef.current;
    prevStepIdRef.current = taskStepId;
    prevSessionMetaPlanRef.current = sessionMetaPlanMode;

    if (!resolvedSessionId || !taskId) return;

    const stepChanged = prevStepId != null && taskStepId != null && prevStepId !== taskStepId;
    const metaCleared = wasPlanMode && !sessionMetaPlanMode;

    // On step change: always reset plan mode state + layout. This is idempotent — if
    // the proceed button already disabled plan mode, these calls are harmless no-ops.
    // On meta cleared: only act if the store still has plan mode enabled.
    if (stepChanged || (metaCleared && planModeFromStore)) {
      applyBuiltInPreset("default");
      closeDocument(resolvedSessionId);
      setActiveDocument(resolvedSessionId, null);
      setPlanMode(resolvedSessionId, false);
      removeContextFile(resolvedSessionId, PLAN_CONTEXT_PATH);
    }
  }, [
    resolvedSessionId,
    taskId,
    taskStepId,
    sessionMetaPlanMode,
    planModeFromStore,
    applyBuiltInPreset,
    closeDocument,
    setActiveDocument,
    setPlanMode,
    removeContextFile,
  ]);
}

// --- Plan layout handlers ---

type PlanLayoutHandlersOpts = {
  resolvedSessionId: string | null;
  taskId: string | null;
  setActiveDocument: (sid: string, doc: ActiveDocument | null) => void;
  applyBuiltInPreset: (preset: BuiltInPreset) => void;
  closeDocument: (sid: string) => void;
  setPlanMode: (sid: string, enabled: boolean) => void;
  addContextFile: (sid: string, file: { path: string; name: string }) => void;
  removeContextFile: (sid: string, path: string) => void;
  refocusChatAfterLayout: () => void;
};

/** Returns togglePlanLayout and handlePlanModeChange callbacks. */
export function usePlanLayoutHandlers(opts: PlanLayoutHandlersOpts) {
  const {
    resolvedSessionId,
    taskId,
    setActiveDocument,
    applyBuiltInPreset,
    closeDocument,
    setPlanMode,
    addContextFile,
    removeContextFile,
    refocusChatAfterLayout,
  } = opts;

  const togglePlanLayout = useCallback(
    (show: boolean) => {
      if (!resolvedSessionId || !taskId) return;
      if (show) {
        setActiveDocument(resolvedSessionId, { type: "plan", taskId });
        applyBuiltInPreset("plan");
      } else {
        applyBuiltInPreset("default");
        closeDocument(resolvedSessionId);
        setActiveDocument(resolvedSessionId, null);
      }
      refocusChatAfterLayout();
    },
    [
      resolvedSessionId,
      taskId,
      setActiveDocument,
      applyBuiltInPreset,
      closeDocument,
      refocusChatAfterLayout,
    ],
  );

  const handlePlanModeChange = useCallback(
    (enabled: boolean) => {
      if (!resolvedSessionId || !taskId) return;
      if (enabled) {
        setActiveDocument(resolvedSessionId, { type: "plan", taskId });
        applyBuiltInPreset("plan");
        setPlanMode(resolvedSessionId, true);
        addContextFile(resolvedSessionId, { path: PLAN_CONTEXT_PATH, name: "Plan" });
      } else {
        applyBuiltInPreset("default");
        closeDocument(resolvedSessionId);
        setActiveDocument(resolvedSessionId, null);
        setPlanMode(resolvedSessionId, false);
        removeContextFile(resolvedSessionId, PLAN_CONTEXT_PATH);
      }
      refocusChatAfterLayout();
    },
    [
      resolvedSessionId,
      taskId,
      setActiveDocument,
      applyBuiltInPreset,
      closeDocument,
      setPlanMode,
      addContextFile,
      removeContextFile,
      refocusChatAfterLayout,
    ],
  );

  return { togglePlanLayout, handlePlanModeChange };
}
