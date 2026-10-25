import type { TFunction } from "i18next";
import type { AutonomyCondition } from "@/lib/api/domains/coordinator-autonomy-api";
import type { PersistentHoldReason } from "@/lib/coordinator/autonomy";

type Condition = Pick<AutonomyCondition, "name" | "detail">;

/** The display label of a known containment condition, or undefined for a name the client does not know. */
export function containmentLabel(name: string, t: TFunction): string | undefined {
  switch (name) {
    case "executor_isolated":
      return t("coordinator:containmentLabel_executor_isolated");
    case "auth_enabled":
      return t("coordinator:containmentLabel_auth_enabled");
    case "no_kandev_credential":
      return t("coordinator:containmentLabel_no_kandev_credential");
    case "no_extra_tools":
      return t("coordinator:containmentLabel_no_extra_tools");
    default:
      return undefined;
  }
}

function detailOverrideFix(detail: string, t: TFunction): string | undefined {
  switch (detail) {
    case "changed_since_launch":
      return t("coordinator:containmentDetailFix_changed_since_launch");
    case "unreadable":
      return t("coordinator:containmentDetailFix_unreadable");
    case "unverified_source":
      return t("coordinator:containmentDetailFix_unverified_source");
    default:
      return undefined;
  }
}

function nameFix(name: string, t: TFunction): string | undefined {
  switch (name) {
    case "executor_isolated":
      return t("coordinator:containmentFix_executor_isolated");
    case "auth_enabled":
      return t("coordinator:containmentFix_auth_enabled");
    case "no_kandev_credential":
      return t("coordinator:containmentFix_no_kandev_credential");
    case "no_extra_tools":
      return t("coordinator:containmentFix_no_extra_tools");
    default:
      return undefined;
  }
}

/**
 * The fix line of a Not met condition: a detail override replaces the
 * condition's own fix; a condition name the client does not know has none.
 */
export function containmentFix(condition: Condition, t: TFunction): string | undefined {
  return detailOverrideFix(condition.detail, t) ?? nameFix(condition.name, t);
}

/**
 * A non-empty detail that is not one of the three overrides is a machine
 * token (executor type, `disabled`, `KANDEV_API_KEY`, ...) shown untranslated.
 */
export function containmentMachineToken(condition: Condition): string | undefined {
  if (!condition.detail) return undefined;
  const overrides = ["changed_since_launch", "unreadable", "unverified_source"];
  return overrides.includes(condition.detail) ? undefined : condition.detail;
}

/** The held reason text of the strip and the item title. */
export function heldReasonText(reason: PersistentHoldReason, detail: string, t: TFunction): string {
  switch (reason) {
    case "containment":
      return t("coordinator:autonomyHeldContainment", {
        condition: containmentLabel(detail, t) ?? detail,
      });
    case "spend_unmeasured":
      return t("coordinator:autonomyHeldSpendUnmeasured");
    case "ceiling_reached":
      return t("coordinator:autonomyHeldCeilingReached");
    case "no_conversation":
      return t("coordinator:autonomyHeldNoConversation");
    case "conversation_unavailable":
      return detail === "session_not_started"
        ? t("coordinator:autonomyHeldSessionNotStarted")
        : t("coordinator:autonomyHeldConversationUnavailable");
  }
}

/** The item's "what clears it" line; undefined when the condition is not in the read. */
export function heldFixText(
  reason: PersistentHoldReason,
  detail: string,
  conditions: AutonomyCondition[],
  t: TFunction,
): string | undefined {
  switch (reason) {
    case "containment": {
      const condition = conditions.find((c) => c.name === detail);
      return condition ? containmentFix(condition, t) : undefined;
    }
    case "spend_unmeasured":
      return t("coordinator:autonomyFixSpendUnmeasured");
    case "ceiling_reached":
      return t("coordinator:autonomyFixCeilingReached");
    default:
      return heldReasonText(reason, detail, t);
  }
}
