import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";
import type { WorkflowStep } from "@/lib/types/http";

export const IMPROVE_KANDEV_SKIP_INTRO_KEY = "kandev.improveKandev.skipIntro";

export type ImproveKandevKind = "bug" | "feature" | "issue";
export type ImproveKandevMode = "intro" | "create";

type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export type ImproveKandevReadyState = {
  data: Pick<
    ImproveKandevBootstrapResponse,
    "workflow_id" | "issue_workflow_id" | "fork_status" | "fork_message"
  >;
  steps: WorkflowStep[];
  issueSteps: WorkflowStep[];
};

export function readImproveKandevSkipIntro(storage: StorageLike | null | undefined): boolean {
  try {
    return storage?.getItem(IMPROVE_KANDEV_SKIP_INTRO_KEY) === "true";
  } catch {
    return false;
  }
}

export function writeImproveKandevSkipIntro(
  storage: StorageLike | null | undefined,
  skip: boolean,
): void {
  try {
    if (skip) {
      storage?.setItem(IMPROVE_KANDEV_SKIP_INTRO_KEY, "true");
    } else {
      storage?.removeItem(IMPROVE_KANDEV_SKIP_INTRO_KEY);
    }
  } catch {
    // Browser privacy settings can reject localStorage writes. The preference
    // is best-effort and must never prevent the dialog from opening.
  }
}

export function initialImproveKandevMode(skipIntro: boolean): ImproveKandevMode {
  return skipIntro ? "create" : "intro";
}

export function resolveImproveKandevWorkflow(
  ready: ImproveKandevReadyState | null,
  kind: ImproveKandevKind,
): {
  workflowId: string | null;
  steps: WorkflowStep[];
  startStep: WorkflowStep | null;
} {
  let steps: WorkflowStep[] = [];
  if (ready) {
    steps = kind === "issue" ? ready.issueSteps : ready.steps;
  }
  let workflowId: string | null = null;
  if (ready) {
    workflowId = kind === "issue" ? ready.data.issue_workflow_id : ready.data.workflow_id;
  }
  return {
    workflowId,
    steps,
    startStep: steps.find((step) => step.is_start_step) ?? steps[0] ?? null,
  };
}

export function getImproveKandevForkBlockedReason(
  kind: ImproveKandevKind,
  forkStatus: ImproveKandevBootstrapResponse["fork_status"] | undefined,
  forkMessage: string | null | undefined,
): string | null {
  if (kind === "issue" || forkStatus !== "blocked_emu") return null;
  return forkMessage ?? "This GitHub account cannot fork kdlbs/kandev.";
}
