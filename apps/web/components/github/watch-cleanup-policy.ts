import type { TFunction } from "i18next";
import type { CleanupPolicy } from "@/lib/types/github";

/**
 * Cleanup-policy choices for the review and issue watch dialogs.
 *
 * `id` is the wire `CleanupPolicy` enum the backend stores and switches on, so it
 * is never translated. The copy travels as catalog keys resolved at render: a
 * `t()` call at module scope would freeze at the boot locale (see docs/i18n.md).
 * Both key fields are required, so `t()` can never receive `string | undefined`.
 *
 * The labels are shared; only the descriptions differ, because a merged PR and a
 * closed issue retain tasks for different reasons.
 */
export type CleanupPolicyOption = {
  id: CleanupPolicy;
  labelKey: string;
  descriptionKey: string;
};

export const REVIEW_CLEANUP_POLICY_OPTIONS: CleanupPolicyOption[] = [
  {
    id: "auto",
    labelKey: "github:cleanupPolicyAuto",
    descriptionKey: "github:cleanupPolicyAutoDescription",
  },
  {
    id: "always",
    labelKey: "github:cleanupPolicyAlways",
    descriptionKey: "github:cleanupPolicyAlwaysDescription",
  },
  {
    id: "never",
    labelKey: "github:cleanupPolicyNever",
    descriptionKey: "github:cleanupPolicyNeverDescription",
  },
];

export const ISSUE_CLEANUP_POLICY_OPTIONS: CleanupPolicyOption[] = [
  {
    id: "auto",
    labelKey: "github:cleanupPolicyAuto",
    descriptionKey: "github:issueCleanupPolicyAutoDescription",
  },
  {
    id: "always",
    labelKey: "github:cleanupPolicyAlways",
    descriptionKey: "github:issueCleanupPolicyAlwaysDescription",
  },
  {
    id: "never",
    labelKey: "github:cleanupPolicyNever",
    descriptionKey: "github:cleanupPolicyNeverDescription",
  },
];

/** Items for the policy `SelectField`. */
export function cleanupPolicyItems(t: TFunction, options: CleanupPolicyOption[]) {
  return options.map((option) => ({ id: option.id, label: t(option.labelKey) }));
}

/** Description shown under the policy select; "" when the id is unknown. */
export function cleanupPolicyDescription(
  t: TFunction,
  options: CleanupPolicyOption[],
  policy: CleanupPolicy,
): string {
  const option = options.find((candidate) => candidate.id === policy);
  return option ? t(option.descriptionKey) : "";
}
