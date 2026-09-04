import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ensureTaskSession } from "@/lib/services/session-launch-service";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useAppStore } from "@/components/state-provider";

/** Minimal task shape consumed by useEnsureTaskSession. */
export type EnsureTaskInput = {
  id?: string | null;
  /** Snake_case workflow_step_id from the HTTP Task type. */
  workflowStepId?: string | null;
  /** Snake_case workflow_id from the HTTP Task type. */
  workflowId?: string | null;
} | null;

export type KanbanStepLike = {
  id: string;
  position: number;
};

/**
 * Reports whether the step is the terminal step of its workflow. Terminal is
 * the maximum under the `(position, id)` ordering (ties broken by max id):
 * step positions are caller-supplied and not uniqueness-validated, so equal
 * positions are representable and must not make both steps terminal.
 * Missing step id or an empty step list → not final (no gate).
 */
export function isFinalWorkflowStep(
  workflowStepId: string | null | undefined,
  steps: KanbanStepLike[] | undefined,
): boolean {
  if (!workflowStepId || !steps || steps.length === 0) return false;
  const current = steps.find((s) => s.id === workflowStepId);
  if (!current) return false;
  const terminal = steps.reduce<typeof current | null>((best, step) => {
    if (!best) return step;
    if (step.position > best.position) return step;
    if (step.position === best.position && step.id > best.id) return step;
    return best;
  }, null);
  return terminal?.id === current.id;
}

export type EnsureTaskSessionStatus = "idle" | "preparing" | "error";

export type UseEnsureTaskSessionResult = {
  status: EnsureTaskSessionStatus;
  error: Error | null;
  retry: () => void;
};

/**
 * Ensures the task has at least one session by delegating to the backend's
 * idempotent `session.ensure` endpoint. The backend resolves the agent profile
 * (task metadata → workflow step → workflow → workspace default) and chooses
 * prepare vs start based on the workflow step's auto_start_agent action — the
 * frontend stays thin and contract-free.
 *
 * Behavior:
 * - No-op while sessions are still loading.
 * - No-op when the task already has at least one session.
 * - No-op when `enabled === false` or `task.id` is missing.
 * - Idempotent per task id within a mount; switching tasks resets the latch.
 */
/**
 * Resolves the task's workflow step list and final-step decision for the
 * prevent-auto-start gate. Workflow-aware: the active workflow's steps
 * (state.kanban.steps) when the task is in it, otherwise the multi-workflow
 * snapshot's steps. `stepsKnown` is false only while the ACTIVE workflow's
 * steps are still hydrating (loading, empty) — callers must wait before
 * ensuring, or a final-step task would auto-start. Non-active workflows
 * without a snapshot never resolve here (documented safe default: no gate).
 */
function resolveWorkflowSteps(
  snapshot: { steps: KanbanStepLike[]; isPlaceholder?: boolean } | undefined,
  taskWorkflowId: string | null,
  kanbanWorkflowId: string | null,
  kanbanSteps: KanbanStepLike[],
): KanbanStepLike[] {
  // The ACTIVE workflow's steps always come from state.kanban.steps: WS
  // step events (workflow.step.created/updated) only mutate that list, so a
  // multi-workflow snapshot for the active workflow can be stale and miss a
  // freshly-added terminal step, which would bypass the final-step gate.
  if (taskWorkflowId === null || taskWorkflowId === kanbanWorkflowId) return kanbanSteps;
  if (snapshot !== undefined) return snapshot.steps;
  return [];
}

function resolveStepsKnown(
  snapshot: { steps: KanbanStepLike[]; isPlaceholder?: boolean } | undefined,
  taskWorkflowId: string | null,
  kanbanWorkflowId: string | null,
  kanbanSteps: KanbanStepLike[],
  kanbanLoading: boolean,
): boolean {
  if (taskWorkflowId === null || taskWorkflowId === kanbanWorkflowId) {
    return kanbanSteps.length > 0 || !kanbanLoading;
  }
  if (snapshot !== undefined) {
    // A placeholder snapshot (created by a task event before the real
    // workflow snapshot arrives) carries empty steps — it must NOT satisfy
    // stepsKnown, or the ungated ensure would latch before the real steps
    // hydrate.
    return snapshot.isPlaceholder !== true;
  }
  // Non-active workflow with no snapshot: safe default (no gate), never wait.
  return true;
}

export function resolveFinalStepInfo(
  task: EnsureTaskInput,
  kanbanWorkflowId: string | null,
  kanbanSteps: KanbanStepLike[],
  kanbanLoading: boolean,
  snapshots: Record<string, { steps: KanbanStepLike[]; isPlaceholder?: boolean }>,
): { isFinalStep: boolean; stepsKnown: boolean } {
  const workflowStepId = task?.workflowStepId ?? null;
  const taskWorkflowId = task?.workflowId ?? null;
  // A task whose own workflow is unknown must never resolve against another
  // workflow's steps: if workflowStepId merely matches a terminal step of the
  // ACTIVE workflow, gating would suppress auto-start for a task whose own
  // workflow we cannot see. No workflow id → no gate.
  if (!workflowStepId || !taskWorkflowId) return { isFinalStep: false, stepsKnown: true };
  const snapshot = taskWorkflowId ? snapshots[taskWorkflowId] : undefined;
  const steps = resolveWorkflowSteps(snapshot, taskWorkflowId, kanbanWorkflowId, kanbanSteps);
  const isFinalStep = isFinalWorkflowStep(workflowStepId, steps);
  const stepsKnown = resolveStepsKnown(
    snapshot,
    taskWorkflowId,
    kanbanWorkflowId,
    kanbanSteps,
    kanbanLoading,
  );
  return { isFinalStep, stepsKnown };
}

export function useEnsureTaskSession(
  task: EnsureTaskInput,
  opts?: { enabled?: boolean },
): UseEnsureTaskSessionResult {
  const enabled = opts?.enabled ?? true;
  const taskId = task?.id ?? null;
  const { sessions, isLoaded, loadSessions } = useTaskSessions(taskId);

  const [status, setStatus] = useState<EnsureTaskSessionStatus>("idle");
  const [error, setError] = useState<Error | null>(null);
  const [retryToken, setRetryToken] = useState(0);

  // Prevent-auto-start preference + workflow-aware step resolution. The
  // terminal-step gate applies to the task's OWN workflow: the active
  // workflow's steps (state.kanban.steps) when the task is in it, otherwise
  // the multi-workflow snapshot's steps for that workflow.
  const preventAutoStart = useAppStore((state) => state.userSettings.preventAutoStartAgentOnOpen);
  const kanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  const kanbanLoading = useAppStore((state) => state.kanban.isLoading === true);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);

  const { isFinalStep, stepsKnown } = useMemo(
    () =>
      preventAutoStart
        ? resolveFinalStepInfo(task, kanbanWorkflowId, kanbanSteps, kanbanLoading, snapshots)
        : { isFinalStep: false, stepsKnown: true },
    [preventAutoStart, task, kanbanWorkflowId, kanbanSteps, kanbanLoading, snapshots],
  );

  // Latch keyed by `${taskId}:${retryToken}` so a re-render on the same task
  // doesn't refire, but switching tasks or calling retry() does.
  const launchedKeyRef = useRef<string | null>(null);
  const previousTaskIdRef = useRef<string | null>(taskId);

  /* eslint-disable react-hooks/set-state-in-effect -- task changes must clear stale ensure-session errors before early returns */
  useEffect(() => {
    if (previousTaskIdRef.current === taskId) return;
    previousTaskIdRef.current = taskId;
    launchedKeyRef.current = null;
    setStatus("idle");
    setError(null);
  }, [taskId]);
  /* eslint-enable react-hooks/set-state-in-effect */

  /* eslint-disable react-hooks/set-state-in-effect -- ensuring a session is a side effect; status mirrors that external work */
  useEffect(() => {
    if (!enabled || !taskId || !isLoaded) return;
    if (sessions.length > 0) return;
    // Wait for the workflow steps to resolve before deciding the gate. This
    // branch does NOT latch, so a later steps hydration re-runs the effect
    // with the correct isFinalStep value (fixes late-hydration auto-start).
    if (preventAutoStart && !stepsKnown) return;
    const key = `${taskId}:${retryToken}`;
    if (launchedKeyRef.current === key) return;
    launchedKeyRef.current = key;

    // Cancel guard so a stale resolution can't overwrite a switched-away task.
    let cancelled = false;
    setStatus("preparing");
    setError(null);
    const ensurePromise = isFinalStep
      ? ensureTaskSession(taskId, { autoStart: false })
      : ensureTaskSession(taskId);
    ensurePromise
      .then(async () => {
        if (cancelled || launchedKeyRef.current !== key) return;
        // Force-reload: backend may have returned an existing_* source our initial list missed.
        await loadSessions(true);
        if (cancelled || launchedKeyRef.current !== key) return;
        setStatus("idle");
      })
      .catch((err: unknown) => {
        if (cancelled || launchedKeyRef.current !== key) return;
        setStatus("error");
        setError(err instanceof Error ? err : new Error(String(err)));
        // Keep the key latched. The retry token or a task change owns the next attempt.
      });

    return () => {
      cancelled = true;
    };
  }, [
    enabled,
    taskId,
    isLoaded,
    loadSessions,
    sessions.length,
    retryToken,
    preventAutoStart,
    stepsKnown,
    isFinalStep,
  ]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const retry = useCallback(() => {
    setStatus("idle");
    setError(null);
    setRetryToken((n) => n + 1);
  }, []);

  return { status, error, retry };
}
