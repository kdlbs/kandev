import {
  resolveChangeRequestTerminology,
  type PRCreateResult,
  type getChangeRequestTerminology,
} from "@/hooks/use-git-operations";
import { t } from "@/lib/i18n";

type Terminology = ReturnType<typeof getChangeRequestTerminology>;

export function getChangeRequestFailureFeedback(result: PRCreateResult, fallback: Terminology) {
  const terms = resolveChangeRequestTerminology(result.provider, fallback);
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
    description: result.error || t("integrations:anErrorOccurred"),
    variant: "error" as const,
  };
}
