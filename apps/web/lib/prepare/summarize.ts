import type {
  PrepareStepInfo,
  SessionPrepareState,
} from "@/lib/state/slices/session-runtime/types";

export type PreparePhase =
  | "idle"
  | "preparing"
  | "preparing_fallback"
  | "resuming"
  | "ready"
  | "failed";

export type PrepareSummary = {
  phase: PreparePhase;
  current: PrepareStepInfo | null;
  currentIndex: number;
  totalSteps: number;
  fallbackWarning: string | null;
  failedStep: PrepareStepInfo | null;
  durationMs: number | null;
};

export const IDLE_PREPARE_SUMMARY: PrepareSummary = {
  phase: "idle",
  current: null,
  currentIndex: 0,
  totalSteps: 0,
  fallbackWarning: null,
  failedStep: null,
  durationMs: null,
};

export function isPreparingPhase(phase: PreparePhase): boolean {
  return phase === "preparing" || phase === "preparing_fallback" || phase === "resuming";
}

// isFallbackNoticeStep returns true when a step is the "previous sandbox went
// away, we provisioned a fresh one" notice. It carries a warning rather than
// an error, so the only signal that the prepare succeeded *with a recovery*
// (not just a generic warning) is matching this row by name + status.
export function isFallbackNoticeStep(step: PrepareStepInfo): boolean {
  return step.status === "skipped" && step.name === "Reconnecting cloud sandbox";
}

/**
 * summarizePrepare reduces the prepare-progress slice + session state for one
 * session into a UI-friendly summary used by the executor popover. It surfaces:
 *  - the missing-sandbox fallback as a first-class "preparing_fallback" phase
 *    so the popover renders dedicated UI for the recovery path;
 *  - a "resuming" phase synthesized from session.state == STARTING when the
 *    backend skips prepare events on resume, so the popover still shows a
 *    spinner instead of looking idle for the entire reconnect.
 */
export function summarizePrepare(
  state: SessionPrepareState | null,
  sessionState: string | null,
): PrepareSummary {
  if (!state) {
    if (sessionState === "STARTING") {
      return { ...IDLE_PREPARE_SUMMARY, phase: "resuming" };
    }
    return IDLE_PREPARE_SUMMARY;
  }

  const steps = state.steps ?? [];
  const fallbackStep = steps.find(isFallbackNoticeStep) ?? null;
  const failedStep = steps.find((s) => s.status === "failed") ?? null;
  const runningStep = steps.find((s) => s.status === "running") ?? null;
  const { current, currentIndex } = pickCurrentStep(steps, runningStep);

  return {
    phase: derivePhase(state.status, fallbackStep, failedStep),
    current,
    currentIndex,
    totalSteps: steps.length,
    fallbackWarning: fallbackStep?.warning ?? null,
    failedStep,
    durationMs: state.durationMs ?? null,
  };
}

function pickCurrentStep(
  steps: PrepareStepInfo[],
  runningStep: PrepareStepInfo | null,
): { current: PrepareStepInfo | null; currentIndex: number } {
  if (runningStep) {
    return { current: runningStep, currentIndex: steps.indexOf(runningStep) };
  }
  const lastCompletedIdx = lastIndexWhere(
    steps,
    (s) => s.status === "completed" || s.status === "skipped" || s.status === "failed",
  );
  if (lastCompletedIdx >= 0) {
    return { current: steps[lastCompletedIdx], currentIndex: lastCompletedIdx };
  }
  return { current: steps[0] ?? null, currentIndex: 0 };
}

function derivePhase(
  status: string,
  fallbackStep: PrepareStepInfo | null,
  failedStep: PrepareStepInfo | null,
): PreparePhase {
  if (status === "preparing") return fallbackStep ? "preparing_fallback" : "preparing";
  if (status === "failed" || failedStep) return "failed";
  if (status === "completed") return "ready";
  return "idle";
}

function lastIndexWhere<T>(items: T[], pred: (item: T) => boolean): number {
  for (let i = items.length - 1; i >= 0; i--) {
    if (pred(items[i])) return i;
  }
  return -1;
}
