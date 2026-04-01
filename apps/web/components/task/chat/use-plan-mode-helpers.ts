"use client";

import { useCallback, useEffect, useRef } from "react";
import type { ActiveDocument } from "@/lib/state/slices/ui/types";
import type { BuiltInPreset } from "@/lib/state/layout-manager/presets";

const PLAN_CONTEXT_PATH = "plan:context";

// --- Auto-disable plan mode ---

type AutoDisablePlanOpts = {
  resolvedSessionId: string | null;
  taskId: string | null;
  sessionMetaPlanMode: boolean;
  planModeFromStore: boolean;
  applyBuiltInPreset: (preset: BuiltInPreset) => void;
  closeDocument: (sid: string) => void;
  setActiveDocument: (sid: string, doc: ActiveDocument | null) => void;
  setPlanMode: (sid: string, enabled: boolean) => void;
  removeContextFile: (sid: string, path: string) => void;
};

/**
 * Auto-disable plan mode when the backend clears plan_mode from session metadata.
 *
 * Triggers when `sessionMetaPlanMode` transitions from true to false — the backend explicitly
 * cleared plan_mode (e.g., on_exit: disable_plan_mode, or entering a non-plan-mode step).
 *
 * User-initiated step moves (proceed button, stepper) disable plan mode directly in their
 * click handlers rather than relying on this effect.
 */
export function useAutoDisablePlanMode(opts: AutoDisablePlanOpts) {
  const {
    resolvedSessionId,
    taskId,
    sessionMetaPlanMode,
    planModeFromStore,
    applyBuiltInPreset,
    closeDocument,
    setActiveDocument,
    setPlanMode,
    removeContextFile,
  } = opts;

  const prevSessionMetaPlanRef = useRef(sessionMetaPlanMode);

  useEffect(() => {
    const wasPlanMode = prevSessionMetaPlanRef.current;
    prevSessionMetaPlanRef.current = sessionMetaPlanMode;

    if (!resolvedSessionId || !taskId) return;

    if (wasPlanMode && !sessionMetaPlanMode && planModeFromStore) {
      applyBuiltInPreset("default");
      closeDocument(resolvedSessionId);
      setActiveDocument(resolvedSessionId, null);
      setPlanMode(resolvedSessionId, false);
      removeContextFile(resolvedSessionId, PLAN_CONTEXT_PATH);
    }
  }, [
    resolvedSessionId,
    taskId,
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
