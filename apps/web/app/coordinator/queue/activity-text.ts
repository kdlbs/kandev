import type { TFunction } from "i18next";
import type { ActivityItem } from "@/lib/api/domains/coordinator-activity-api";
import type { UndoMessageKey } from "@/hooks/domains/coordinator/activity-controller";
import type { PersonName } from "@/hooks/domains/coordinator/use-activity-members";

export const CLASS_LABEL_KEY: Record<string, string> = {
  create_task: "coordinator:activityClassCreateTask",
  start_agent: "coordinator:activityClassStartAgent",
  message: "coordinator:activityClassMessage",
  move: "coordinator:activityClassMove",
  resume: "coordinator:activityClassResume",
  stop: "coordinator:activityClassStop",
  unknown: "coordinator:activityClassUnknown",
};

export const MESSAGE_TEXT_KEY: Record<UndoMessageKey, string> = {
  activityConflictMoved: "coordinator:activityConflictMoved",
  activityConflictAgentRunning: "coordinator:activityConflictAgentRunning",
  activityConflictStepDeleted: "coordinator:activityConflictStepDeleted",
  activityConflictStepDone: "coordinator:activityConflictStepDone",
  activityConflictStepFull: "coordinator:activityConflictStepFull",
  activityNotUndoable: "coordinator:activityNotUndoable",
  activityUndoFailed: "coordinator:activityUndoFailed",
  activityGone: "coordinator:activityGone",
};

export function classLabel(actionClass: string, t: TFunction): string {
  return t(CLASS_LABEL_KEY[actionClass] ?? CLASS_LABEL_KEY.unknown);
}

const REFUSED_REASON_KEY: Record<string, string> = {
  binding_invalid: "coordinator:activityRefusedBindingInvalid",
  not_in_profile: "coordinator:activityRefusedNotInProfile",
  policy_denied: "coordinator:activityRefusedPolicyDenied",
};

function refusedText(code: string | null, t: TFunction): string {
  if (!code) return t("coordinator:activityRefused");
  const key = REFUSED_REASON_KEY[code];
  return key ? t(key) : t("coordinator:activityRefusedCode", { code });
}

/** The Action cell text: refused reason, prefixed rejected or failed detail, else the detail or the class label. */
export function actionText(item: ActivityItem, t: TFunction): string {
  if (item.outcome === "refused") return refusedText(item.reason_code, t);
  const detail = item.detail.trim() === "" ? "" : item.detail;
  if (item.outcome === "rejected") {
    return detail
      ? t("coordinator:activityRejectedDetail", { detail })
      : t("coordinator:activityRejected");
  }
  if (item.outcome === "returned") {
    return detail
      ? t("coordinator:activityReturnedDetail", { detail })
      : t("coordinator:activityReturned");
  }
  if (item.outcome === "failed") {
    return detail
      ? t("coordinator:activityFailedDetail", { detail })
      : t("coordinator:activityFailed");
  }
  return detail || classLabel(item.action_class, t);
}

export function authorizationLine(item: ActivityItem, t: TFunction): string | null {
  if (item.authorization === "requires_approval") return t("coordinator:activityAuthRequires");
  if (item.authorization !== "denied") return null;
  return item.refusal_count > 1
    ? t("coordinator:activityAuthDeniedRepeated", { count: item.refusal_count })
    : t("coordinator:activityAuthDenied");
}

function personName(person: PersonName, t: TFunction): string | null {
  if (person.kind === "named") return person.name;
  if (person.kind === "former") return t("coordinator:activityFormerMember");
  return null;
}

/** The second line of How it was authorised; null for proposed and refused rows. */
export function outcomeLine(item: ActivityItem, person: PersonName, t: TFunction): string | null {
  const name = personName(person, t);
  switch (item.outcome) {
    case "approved":
      if (item.edited) {
        return name
          ? t("coordinator:activityApprovedByEdited", { name })
          : t("coordinator:activityApprovedEdited");
      }
      return name
        ? t("coordinator:activityApprovedBy", { name })
        : t("coordinator:activityApproved");
    case "rejected":
      return name
        ? t("coordinator:activityRejectedBy", { name })
        : t("coordinator:activityRejected");
    case "returned":
      return name
        ? t("coordinator:activityReturnedBy", { name })
        : t("coordinator:activityReturned");
    case "failed":
      return t("coordinator:activityFailed");
    case "undone":
      return name ? t("coordinator:activityUndoneBy", { name }) : t("coordinator:activityUndone");
    default:
      return null;
  }
}

export function undoneText(person: PersonName, time: string, t: TFunction): string {
  const name = personName(person, t);
  return name
    ? t("coordinator:activityUndoneByAt", { name, time })
    : t("coordinator:activityUndoneAt", { time });
}

/** One whole sentence chosen by the row; never a noun phrase spliced into a sentence. */
export function undoDialogSentence(
  item: ActivityItem,
  stepName: string | undefined,
  t: TFunction,
): string {
  const identifier = item.target_task_identifier || undefined;
  if (item.action_class === "create_task") {
    return identifier
      ? t("coordinator:activityUndoConfirmCreate", { identifier })
      : t("coordinator:activityUndoConfirmCreateNoId");
  }
  if (identifier) {
    return stepName
      ? t("coordinator:activityUndoConfirmMove", { identifier, step: stepName })
      : t("coordinator:activityUndoConfirmMoveNoStep", { identifier });
  }
  return stepName
    ? t("coordinator:activityUndoConfirmMoveNoId", { step: stepName })
    : t("coordinator:activityUndoConfirmMoveNoIdNoStep");
}
