import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";
import type { WorkflowStep } from "@/lib/types/http";
import { t } from "@/lib/i18n";

export const IMPROVE_KANDEV_SKIP_INTRO_KEY = "kandev.improveKandev.skipIntro";

/**
 * Exact name of the dedicated workspace all improve tasks land in. Mirrors
 * `improveWorkspaceName` in the backend bootstrap handler; the frontend uses it
 * to recognize the workspace for New Task routing.
 */
// i18n-exempt: matched by value against `improveWorkspaceName` in the backend
// bootstrap handler to recognize the workspace; translating it breaks routing.
export const IMPROVE_KANDEV_WORKSPACE_NAME = "Improve Kandev";

export type ImproveKandevKind = "bug" | "feature" | "issue";
export type ImproveKandevMode = "intro" | "create";

export type ContributorAccessMessageKey =
  | "common:improveKandevIssueNoForkAccess"
  | "common:improveKandevDirectPushAccess"
  | "common:improveKandevManagedForkPrepared"
  | "common:improveKandevExistingForkPrepared"
  | "common:improveKandevExecutorForkAtPr";

type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

// Catalog keys. Module scope, so `t()` here would pin the descriptions to the
// boot locale; `describeStep` resolves them per call.
const STEP_DESCRIPTION_KEYS: Record<string, string> = {
  improve: "common:improveStepImprove",
  test: "common:improveStepTest",
  pr: "common:improveStepPr",
  "open-issue": "common:improveStepOpenIssue",
  "open issue": "common:improveStepOpenIssue",
};

export type ImproveKandevReadyState = {
  data: Pick<
    ImproveKandevBootstrapResponse,
    "workflow_id" | "issue_workflow_id" | "fork_status" | "fork_reason_code"
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

export function contributorAccessMessage(
  kind: ImproveKandevKind,
  hasWrite: boolean,
  forkStatus?: ImproveKandevBootstrapResponse["fork_status"],
): ContributorAccessMessageKey {
  if (kind === "issue") return "common:improveKandevIssueNoForkAccess";
  if (hasWrite) return "common:improveKandevDirectPushAccess";
  if (forkStatus === "creatable") return "common:improveKandevManagedForkPrepared";
  if (forkStatus === "ready") return "common:improveKandevExistingForkPrepared";
  return "common:improveKandevExecutorForkAtPr";
}

/** Resolved per call: the dialog re-renders on a locale switch and follows. */
export function getImproveKandevStepDescription(step: Pick<WorkflowStep, "id" | "name">): string {
  const key = STEP_DESCRIPTION_KEYS[step.id] ?? STEP_DESCRIPTION_KEYS[step.name.toLowerCase()];
  // No key means a workflow step this dialog does not describe; its name is
  // user/backend data and stays verbatim.
  return key ? t(key) : step.name;
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
  forkReasonCode: ImproveKandevBootstrapResponse["fork_reason_code"] | undefined,
): string | null {
  if (kind === "issue" || !isImproveKandevForkBlockedStatus(forkStatus)) return null;
  if (forkReasonCode === "account_cannot_fork") {
    return "common:thisGithubAccountCannotForkKdlbs";
  }
  if (forkStatus === "blocked_emu" && forkReasonCode === undefined) {
    return "common:thisGithubAccountCannotForkKdlbs";
  }
  return "common:managedGithubForkUnavailable";
}

export function isImproveKandevForkBlockedStatus(
  forkStatus: ImproveKandevBootstrapResponse["fork_status"] | undefined,
): boolean {
  return forkStatus === "blocked_emu" || forkStatus === "blocked_managed";
}
