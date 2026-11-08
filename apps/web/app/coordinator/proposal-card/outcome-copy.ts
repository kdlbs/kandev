import type { TFunction } from "i18next";
import type {
  ImprovementProposal,
  KindProposal,
  StoredProposal,
} from "@/lib/api/domains/coordinator-api";
import { ACTION_LABEL_KEY } from "@/lib/coordinators/control-draft";

const FAILURE_KEYS: Record<string, string> = {
  outcome_unknown: "coordinator:outcomeUnknown",
  task_archived: "coordinator:outcomeTaskArchived",
};

/** The status line of a refused answer: the stale target or the unavailable task. */
export function answerRefusedKey(code: string | null): string {
  return code === "task_archived"
    ? "coordinator:answerRefusedTaskUnavailable"
    : "coordinator:answerRefusedStale";
}

const KIND_FAILURE_KEYS: Record<KindProposal["kind"], Record<string, string>> = {
  resume: { not_resumable: "coordinator:outcomeNotResumable" },
  message: {
    not_accepting: "coordinator:outcomeNotAccepting",
    queue_full: "coordinator:outcomeQueueFull",
  },
  move: {
    task_left_workflow: "coordinator:outcomeTaskLeftWorkflow",
    moved: "coordinator:outcomeMoved",
    step_missing: "coordinator:outcomeStepMissing",
    step_is_done: "coordinator:outcomeStepIsDone",
    step_starts_agent: "coordinator:outcomeStepStartsAgent",
    agent_running: "coordinator:outcomeAgentRunning",
    step_full: "coordinator:outcomeStepFull",
  },
  answer: {
    stale: "coordinator:answerRefusedStale",
    invalid_draft: "coordinator:outcomeInvalidDraft",
    option_not_offered: "coordinator:outcomeOptionNotOffered",
    resolution_in_progress: "coordinator:outcomeResolutionInProgress",
    not_delivered: "coordinator:outcomeNotDelivered",
  },
};

function outcomeFlags(proposal: KindProposal): {
  deferred: boolean;
  queued: boolean;
  noop: boolean;
} {
  const outcome = proposal.outcome;
  const flags = outcome && typeof outcome === "object" ? (outcome as Record<string, unknown>) : {};
  return {
    deferred: flags.deferred === true,
    queued: flags.queued === true,
    noop: flags.noop === true,
  };
}

/**
 * The status line of a failed resume, message or move card, chosen by the
 * row's error code (docs/specs/coordinator/system-design/proposal-kinds.md
 * "Card outcome copy"). The error text is untrusted and rendered as text.
 */
export function kindFailureText(proposal: KindProposal, t: TFunction): string {
  const code = proposal.error ?? "";
  const key = FAILURE_KEYS[code] ?? KIND_FAILURE_KEYS[proposal.kind][code];
  if (key) return t(key);
  return code
    ? t("coordinator:outcomeOther", { error: code })
    : t("coordinator:outcomeOtherNoError");
}

/** The approve toast title of an approved resume, message or move card. */
export function kindApprovedToast(
  proposal: KindProposal,
  card: string,
  step: string,
  t: TFunction,
): string {
  const flags = outcomeFlags(proposal);
  switch (proposal.kind) {
    case "resume":
      return flags.deferred
        ? t("coordinator:toastApprovedResumeDeferred", { card })
        : t("coordinator:toastApprovedResume", { card });
    case "message":
      return t("coordinator:toastApprovedMessage", { card });
    case "move":
      if (flags.noop) return t("coordinator:toastApprovedMoveNoop", { card, step });
      return flags.queued
        ? t("coordinator:toastApprovedMoveQueued", { card, step })
        : t("coordinator:toastApprovedMove", { card, step });
    case "answer":
      return t("coordinator:toastApprovedAnswer", { card });
  }
}

export type PolicyAction = "create" | "resume" | "message" | "move" | "answer";

const POLICY_KEYS: Record<PolicyAction, string> = {
  create: "coordinator:policyCreate",
  resume: "coordinator:policyResume",
  message: "coordinator:policyMessage",
  move: "coordinator:policyMove",
  answer: "coordinator:policyAnswer",
};

/** "Policy: <action> requires approval", fixed text from the stored kind. */
export function policyLineText(
  proposal: Exclude<StoredProposal, ImprovementProposal>,
  t: TFunction,
): string {
  const action: PolicyAction =
    proposal.kind === undefined || proposal.kind === "create_task" ? "create" : proposal.kind;
  return t(POLICY_KEYS[action]);
}

type LoweredToastContext = {
  t: TFunction;
  toast: (input: { title: string; variant?: "default" | "success" | "error" | "loading" }) => void;
};

/** Tells the manager which class their decision lowered to requires approval, by display label. */
export function loweredToast(proposal: StoredProposal, ctx: LoweredToastContext) {
  const lowered = proposal.lowered_class;
  if (!lowered) return;
  ctx.toast({
    title: ctx.t("coordinator:loweredClassNotice", { class: ctx.t(ACTION_LABEL_KEY[lowered]) }),
    variant: "default",
  });
}
