"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { moveTask } from "@/lib/api";
import type { WorkflowMoveEntryOptions, WorkflowMoveResponse } from "@/lib/api/domains/kanban-api";
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
  currentStepId?: string | null;
  taskState?: string | null;
  /** Identity of the presentation issuing the move; see `usePresentationToken`. */
  presentationToken: number;
  onMoveStart?: () => void;
  onMoveError?: (error: unknown) => void;
};

type UseWorkflowStepMoveResult = {
  movingToStepId: string | null;
  /** Accepted destination retained until the task projection reaches it. */
  progressingToStepId: string | null;
  handleMove: (stepId: string, entryOptions?: WorkflowMoveEntryOptions) => Promise<boolean>;
};

type ProgressMove = {
  requestId: number;
  sourceStepId: string | null;
  targetStepId: string;
};

const TERMINAL_TASK_STATES = new Set(["COMPLETED", "FAILED", "CANCELLED"]);

function shouldClearProgressMove(
  move: ProgressMove,
  currentStepId: string | null | undefined,
  taskState: string | null | undefined,
  priorMoveTargets: ReadonlyMap<number, string>,
): boolean {
  if (taskState && TERMINAL_TASK_STATES.has(taskState)) return true;
  if (currentStepId === move.targetStepId) return true;
  return Boolean(
    move.sourceStepId &&
    currentStepId &&
    currentStepId !== move.sourceStepId &&
    !Array.from(priorMoveTargets.values()).includes(currentStepId),
  );
}

function useWorkflowStepMoveFocus({
  taskId,
  workflowId,
  presentationToken,
}: Pick<UseWorkflowStepMoveParams, "taskId" | "workflowId" | "presentationToken">) {
  const navigationRevision = useAppStore((s) => s.taskRemoval.navigationRevision);
  const beginWorkflowSessionFocus = useAppStore((s) => s.beginWorkflowSessionFocus);
  const bindWorkflowSessionFocus = useAppStore((s) => s.bindWorkflowSessionFocus);
  const reconcileWorkflowSessionFocus = useAppStore((s) => s.reconcileWorkflowSessionFocus);
  const cancelWorkflowSessionFocus = useAppStore((s) => s.cancelWorkflowSessionFocus);
  const activeFocusRequestIdRef = useRef<number | null>(null);

  const beginFocus = useCallback(
    (destinationStepId: string) => {
      if (!taskId || !workflowId) return null;
      const requestId = beginWorkflowSessionFocus({
        taskId,
        workflowId,
        destinationStepId,
        presentationToken,
        navigationRevision,
      });
      activeFocusRequestIdRef.current = requestId;
      return requestId;
    },
    [beginWorkflowSessionFocus, navigationRevision, presentationToken, taskId, workflowId],
  );
  const commitFocus = useCallback(
    (requestId: number | null, response: WorkflowMoveResponse) => {
      if (requestId === null) return;
      try {
        if (!response.workflow_entry_identity) {
          cancelWorkflowSessionFocus({ requestId, presentationToken });
          return;
        }
        bindWorkflowSessionFocus({
          requestId,
          presentationToken,
          entryIdentity: response.workflow_entry_identity,
        });
        if (!taskId) return;
        reconcileWorkflowSessionFocus(taskId, {
          metadata: response.task?.metadata,
          updatedAt: response.task?.updated_at,
          workflowStepId: response.task?.workflow_step_id,
          entryIdentity: response.workflow_entry_identity,
        });
      } finally {
        if (activeFocusRequestIdRef.current === requestId) {
          activeFocusRequestIdRef.current = null;
        }
      }
    },
    [
      bindWorkflowSessionFocus,
      cancelWorkflowSessionFocus,
      presentationToken,
      reconcileWorkflowSessionFocus,
      taskId,
    ],
  );
  const cancelFocus = useCallback(
    (requestId: number | null) => {
      if (requestId === null) return;
      cancelWorkflowSessionFocus({ requestId, presentationToken });
      if (activeFocusRequestIdRef.current === requestId) {
        activeFocusRequestIdRef.current = null;
      }
    },
    [cancelWorkflowSessionFocus, presentationToken],
  );

  useEffect(
    () => () => {
      const requestId = activeFocusRequestIdRef.current;
      if (requestId !== null) cancelWorkflowSessionFocus({ requestId });
    },
    [cancelWorkflowSessionFocus],
  );

  return { beginFocus, cancelFocus, commitFocus };
}

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
  currentStepId,
  taskState,
  presentationToken,
  onMoveStart,
  onMoveError,
}: UseWorkflowStepMoveParams): UseWorkflowStepMoveResult {
  const disablePlanMode = useDisablePlanMode();
  const focus = useWorkflowStepMoveFocus({ taskId, workflowId, presentationToken });
  const [movingToStepId, setMovingToStepId] = useState<string | null>(null);
  const [progressingToStepId, setProgressingToStepId] = useState<string | null>(null);
  // Only the in-flight step's own button is disabled, so every other step stays
  // clickable and two moves can overlap. This counter marks which one is the
  // latest; a slower predecessor must not own the banner or the loading state.
  const moveRequestRef = useRef(0);
  const tokenRef = useRef(presentationToken);
  const progressMoveRef = useRef<ProgressMove | null>(null);
  const priorMoveTargetsRef = useRef(new Map<number, string>());

  // Invalidate synchronously in the render that changes `presentationToken`,
  // not in a passive effect: a `moveTask` rejection can reach its `catch`
  // before a `useEffect` scheduled by the same commit has flushed, which
  // would let a stale request still pass the `requestId` check below.
  if (tokenRef.current !== presentationToken) {
    tokenRef.current = presentationToken;
    // A new presentation invalidates any request still in flight from the one
    // it replaced, and starts with no disabled control of its own.
    moveRequestRef.current += 1;
    progressMoveRef.current = null;
    priorMoveTargetsRef.current.clear();
    setMovingToStepId(null);
    setProgressingToStepId(null);
  }
  if (
    progressingToStepId &&
    progressMoveRef.current &&
    shouldClearProgressMove(
      progressMoveRef.current,
      currentStepId,
      taskState,
      priorMoveTargetsRef.current,
    )
  ) {
    progressMoveRef.current = null;
    priorMoveTargetsRef.current.clear();
    setMovingToStepId(null);
    setProgressingToStepId(null);
  }

  const handleMove = useCallback(
    async (stepId: string, entryOptions?: WorkflowMoveEntryOptions): Promise<boolean> => {
      if (!taskId || !workflowId) return false;
      const focusRequestId = focus.beginFocus(stepId);
      onMoveStart?.();
      disablePlanMode();
      const requestId = ++moveRequestRef.current;
      if (progressMoveRef.current) {
        priorMoveTargetsRef.current.set(
          progressMoveRef.current.requestId,
          progressMoveRef.current.targetStepId,
        );
      }
      progressMoveRef.current = {
        requestId,
        sourceStepId: currentStepId ?? null,
        targetStepId: stepId,
      };
      setMovingToStepId(stepId);
      setProgressingToStepId(stepId);
      try {
        const response = await moveTask(taskId, {
          workflow_id: workflowId,
          workflow_step_id: stepId,
          position: 0,
          entry_options: entryOptions,
        });
        if (requestId === moveRequestRef.current) focus.commitFocus(focusRequestId, response);
        return true;
      } catch (err) {
        console.error("[useWorkflowStepMove] Failed to move task:", err);
        focus.cancelFocus(focusRequestId);
        if (requestId === moveRequestRef.current) {
          progressMoveRef.current = null;
          priorMoveTargetsRef.current.clear();
          setProgressingToStepId(null);
          onMoveError?.(err);
        } else {
          priorMoveTargetsRef.current.delete(requestId);
        }
        return false;
      } finally {
        if (requestId === moveRequestRef.current) setMovingToStepId(null);
      }
    },
    [taskId, workflowId, currentStepId, focus, disablePlanMode, onMoveStart, onMoveError],
  );

  return { movingToStepId, progressingToStepId, handleMove };
}
