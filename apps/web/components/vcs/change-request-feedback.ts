import {
  resolveChangeRequestTerminology,
  type PRCreateResult,
  type getChangeRequestTerminology,
} from "@/hooks/use-git-operations";

type Terminology = ReturnType<typeof getChangeRequestTerminology>;

export function getChangeRequestFailureFeedback(result: PRCreateResult, fallback: Terminology) {
  const terms = resolveChangeRequestTerminology(result.provider, fallback);
  if (result.branch_pushed) {
    return {
      title: `Branch pushed; ${terms.shortName} not created`,
      description: `Branch was pushed; retry ${terms.longName.toLowerCase()} creation.`,
      variant: "default" as const,
    };
  }
  return {
    title: `Create ${terms.shortName} failed`,
    description: result.error || "An error occurred",
    variant: "error" as const,
  };
}
