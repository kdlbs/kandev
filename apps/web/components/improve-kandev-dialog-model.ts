import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";
import type { WorkflowStep } from "@/lib/types/http";

export const IMPROVE_KANDEV_SKIP_INTRO_KEY = "kandev.improveKandev.skipIntro";

/**
 * Exact name of the dedicated workspace all improve tasks land in. Mirrors
 * `improveWorkspaceName` in the backend bootstrap handler; the frontend uses it
 * to recognize the workspace for New Task routing.
 */
export const IMPROVE_KANDEV_WORKSPACE_NAME = "Improve Kandev";

export type ImproveKandevKind = "bug" | "feature" | "issue";
export type ImproveKandevMode = "intro" | "create";

type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

const STEP_DESCRIPTIONS: Record<string, string> = {
  improve:
    "Agent reads the report, explores the codebase, and implements the change with TDD. Runs make fmt, typecheck, test, lint, then commits.",
  test: "Agent boots a secondary kandev instance with make dev and tells you what to verify. You confirm the change works in the running app.",
  pr: "Agent runs the pr skill: pushes the branch and opens a pull request to kdlbs/kandev for the maintainers to review.",
  "open-issue":
    "Agent gathers every required field from the repository template, confirms the public draft with you, and publishes the GitHub issue.",
  "open issue":
    "Agent gathers every required field from the repository template, confirms the public draft with you, and publishes the GitHub issue.",
};

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

export function getImproveKandevBrowserStorage(): StorageLike | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

export function contributorAccessMessage(kind: ImproveKandevKind, hasWrite: boolean): string {
  if (kind === "issue") return "Opening an issue does not require a fork or push access.";
  if (hasWrite) {
    return "You have write access to kdlbs/kandev, so the agent will push directly to a branch on the upstream repo.";
  }
  return "The agent will fork kdlbs/kandev to your account during the PR step and open a pull request from your fork.";
}

export function getImproveKandevStepDescription(step: Pick<WorkflowStep, "id" | "name">): string {
  return STEP_DESCRIPTIONS[step.id] ?? STEP_DESCRIPTIONS[step.name.toLowerCase()] ?? step.name;
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
