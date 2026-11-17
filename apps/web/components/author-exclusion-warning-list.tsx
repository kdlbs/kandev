"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import type { AuthorExclusionWarning } from "@/lib/types/author-exclusion";

type Props = {
  warnings: AuthorExclusionWarning[];
  formatProvider?: (providerId: string) => string;
  testId?: string;
};

// Catalog keys, not copy: the record keys are the wire warning codes.
const MESSAGE_KEY: Record<AuthorExclusionWarning["code"], string> = {
  routing_disabled: "workflows:authorExclusionRoutingDisabled",
  single_provider: "workflows:authorExclusionSingleProvider",
  agent_single_provider: "workflows:authorExclusionAgentSingleProvider",
  check_failed: "workflows:authorExclusionCheckFailed",
};

const ROLE_KEY: Record<string, string> = {
  reviewer: "task:roleReviewer",
  approver: "task:roleApprover",
};

function warningText(
  t: TFunction,
  w: AuthorExclusionWarning,
  formatProvider: (providerId: string) => string,
): string {
  const roleKey = w.role ? ROLE_KEY[w.role] : undefined;
  return t(MESSAGE_KEY[w.code] ?? MESSAGE_KEY.check_failed, {
    workflow: w.workflow_name ?? "",
    step: w.step_name ?? "",
    role: roleKey ? t(roleKey) : (w.role ?? ""),
    provider: formatProvider(w.provider_id ?? ""),
    agent: w.agent_name ?? "",
  });
}

const identity = (providerId: string) => providerId;

/**
 * Non-blocking notice listing exclude_author_provider steps whose current
 * setup can hold reviews or will not enforce the exclusion. Renders nothing
 * when there is nothing to warn about.
 */
export function AuthorExclusionWarningList({
  warnings,
  formatProvider = identity,
  testId = "author-exclusion-warnings",
}: Props) {
  const { t } = useTranslation();
  if (warnings.length === 0) return null;
  return (
    <div
      role="status"
      data-testid={testId}
      className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-900 dark:bg-amber-950/30"
    >
      <div className="flex items-center gap-2 font-medium">
        <IconAlertTriangle className="h-4 w-4 shrink-0 text-amber-600" aria-hidden />
        <span>{t("workflows:authorExclusionWarningTitle")}</span>
      </div>
      <ul className="mt-2 list-disc space-y-1 pl-6 break-words">
        {warnings.map((w, i) => (
          <li
            key={`${w.code}-${w.workflow_name ?? ""}-${w.step_name ?? ""}-${w.agent_name ?? ""}-${i}`}
          >
            {warningText(t, w, formatProvider)}
          </li>
        ))}
      </ul>
    </div>
  );
}
