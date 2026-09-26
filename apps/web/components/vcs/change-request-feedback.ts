import {
  getLocalizedGitOperationError,
  resolveChangeRequestTerminology,
  type PRCreateResult,
  type getChangeRequestTerminology,
} from "@/hooks/use-git-operations";
import { t } from "@/lib/i18n";

type Terminology = ReturnType<typeof getChangeRequestTerminology>;

export function getChangeRequestSuccessFeedback(
  result: PRCreateResult,
  draft: boolean,
  fallback: Terminology,
) {
  const terms = resolveChangeRequestTerminology(result.provider, fallback);
  if (result.linked === false) {
    const associationMessage = (
      result.association_error || t("integrations:taskAssociationCouldNotBeSaved")
    ).replace(/[.!?。]+$/u, "");
    return {
      title: t("integrations:createdLinkNeedsAttention", { shortName: terms.shortName }),
      description: t("integrations:associationRetryViaTaskMenu", {
        message: associationMessage,
      }),
      variant: "default" as const,
    };
  }
  return {
    title: draft
      ? t("integrations:draftCreated", { shortName: terms.shortName })
      : t("integrations:shortNameCreated", { shortName: terms.shortName }),
    description:
      result.pr_url || t("integrations:createdSuccessfully", { longName: terms.longName }),
    variant: "success" as const,
  };
}

export function getChangeRequestFailureFeedback(result: PRCreateResult, fallback: Terminology) {
  const terms = resolveChangeRequestTerminology(result.provider, fallback);
  const localizedError = getLocalizedGitOperationError(result.error_code, result.error);
  if (result.branch_pushed) {
    return {
      title: t("integrations:branchPushedNotCreated", { shortName: terms.shortName }),
      description: t("integrations:branchWasPushedRetryCreation", {
        longName: terms.longName.toLowerCase(),
      }),
      variant: "default" as const,
    };
  }
  return {
    title: t("integrations:createFailed", { shortName: terms.shortName }),
    description: localizedError || t("integrations:anErrorOccurred"),
    variant: "error" as const,
  };
}
