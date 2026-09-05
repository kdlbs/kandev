"use client";

import { useCallback, useRef, useState } from "react";
import { moveTask } from "@/lib/api";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";
import { useAppStore } from "@/components/state-provider";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useDockviewStore } from "@/lib/state/dockview-store";

const PLAN_CONTEXT_PATH = "plan:context";

/** Returns a callback that disables plan mode for the active session of a task. */
export function useDisablePlanMode() {
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const planModeEnabled = useAppStore((s) =>
    activeSessionId ? (s.chatInput.planModeBySessionId[activeSessionId] ?? false) : false,
  );
  const setPlanMode = useAppStore((s) => s.setPlanMode);
  const setActiveDocument = useAppStore((s) => s.setActiveDocument);
  const closeDocument = useLayoutStore((s) => s.closeDocument);
  const removeContextFile = useContextFilesStore((s) => s.removeFile);
  const applyBuiltInPreset = useDockviewStore((s) => s.applyBuiltInPreset);

  return useCallback(() => {
    if (!activeSessionId || !planModeEnabled) return;
    applyBuiltInPreset("default");
    closeDocument(activeSessionId);
    setActiveDocument(activeSessionId, null);
    setPlanMode(activeSessionId, false);
    removeContextFile(activeSessionId, PLAN_CONTEXT_PATH);
  }, [
    activeSessionId,
    planModeEnabled,
    setPlanMode,
    setActiveDocument,
    closeDocument,
    removeContextFile,
    applyBuiltInPreset,
  ]);
}

/**
 * A number that changes whenever `key` changes, including a transition
 * through `null`/`undefined` and back to the same value. Surfaces that render
 * a continuous presentation of one task (the preview panel, the task route)
 * use this as the identity `useWorkflowStepMove` scopes a move request to:
 * closing and reopening on the same task changes `key` to `null` and back,
 * which must count as a new presentation even though the key's final value is
 * unchanged.
 */
export function usePresentationToken(key: string | null | undefined): number {
  const [token, setToken] = useState(0);
  const [prevKey, setPrevKey] = useState(key);
  if (prevKey !== key) {
    setPrevKey(key);
    setToken((current) => current + 1);
  }
  return token;
}

type UseWorkflowStepMoveParams = {
  taskId?: string | null;
  workflowId?: string | null;
  /** Identity of the presentation issuing the move; see `usePresentationToken`. */
  presentationToken: number;
  onMoveStart?: () => void;
  onMoveError?: (error: unknown) => void;
};

type UseWorkflowStepMoveResult = {
  movingToStepId: string | null;
  handleMove: (stepId: string, entryOptions?: WorkflowMoveEntryOptions) => Promise<boolean>;
};

/**
 * The single implementation of the compact stepper's move request. Extracted
 * so the task top bar and the kanban preview header share one plan-mode
 * cleanup and one late-response guard, scoped per `presentationToken` rather
 * than per component instance: a component that stays mounted across task
 * changes, or state lifted above a component that unmounts, both need a late
 * response discarded once the presentation that issued it is gone.
 */
export function useWorkflowStepMove({
  taskId,
  workflowId,
  presentationToken,
  onMoveStart,
  onMoveError,
}: UseWorkflowStepMoveParams): UseWorkflowStepMoveResult {
  const disablePlanMode = useDisablePlanMode();
  const [movingToStepId, setMovingToStepId] = useState<string | null>(null);
  // Only the in-flight step's own button is disabled, so every other step stays
  // clickable and two moves can overlap. This counter marks which one is the
  // latest; a slower predecessor must not own the banner or the loading state.
  const moveRequestRef = useRef(0);
  const tokenRef = useRef(presentationToken);

  // Invalidate synchronously in the render that changes `presentationToken`,
  // not in a passive effect: a `moveTask` rejection can reach its `catch`
  // before a `useEffect` scheduled by the same commit has flushed, which
  // would let a stale request still pass the `requestId` check below.
  if (tokenRef.current !== presentationToken) {
    tokenRef.current = presentationToken;
    // A new presentation invalidates any request still in flight from the one
    // it replaced, and starts with no disabled control of its own.
    moveRequestRef.current += 1;
    setMovingToStepId(null);
  }

  const handleMove = useCallback(
    async (stepId: string, entryOptions?: WorkflowMoveEntryOptions): Promise<boolean> => {
      if (!taskId || !workflowId) return false;
      onMoveStart?.();
      disablePlanMode();
      const requestId = ++moveRequestRef.current;
      setMovingToStepId(stepId);
      try {
        await moveTask(taskId, {
          workflow_id: workflowId,
          workflow_step_id: stepId,
          position: 0,
          entry_options: entryOptions,
        });
        return true;
      } catch (err) {
        console.error("[useWorkflowStepMove] Failed to move task:", err);
        if (requestId === moveRequestRef.current) onMoveError?.(err);
        return false;
      } finally {
        if (requestId === moveRequestRef.current) setMovingToStepId(null);
      }
    },
    [taskId, workflowId, disablePlanMode, onMoveStart, onMoveError],
  );

  return { movingToStepId, handleMove };
}
