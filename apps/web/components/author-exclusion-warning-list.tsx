"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import type { AuthorExclusionWarning } from "@/lib/types/author-exclusion";

export type AuthorExclusionWarningSurface = "routing" | "elsewhere";

type Props = {
  warnings: AuthorExclusionWarning[];
  formatProvider?: (providerId: string) => string;
  /** Where the list is shown: the routing page, or anywhere else (import, sync). */
  surface?: AuthorExclusionWarningSurface;
  testId?: string;
};

type WarningKeys = { problem: string; fixHere: string; fixElsewhere: string };

// Catalog keys, not copy: the record keys are the wire warning codes.
const WARNING_KEYS: Record<AuthorExclusionWarning["code"], WarningKeys> = {
  routing_disabled: {
    problem: "workflows:authorExclusionRoutingDisabled",
    fixHere: "workflows:authorExclusionRoutingDisabledFixHere",
    fixElsewhere: "workflows:authorExclusionRoutingDisabledFixElsewhere",
  },
  single_provider: {
    problem: "workflows:authorExclusionSingleProvider",
    fixHere: "workflows:authorExclusionSingleProviderFixHere",
    fixElsewhere: "workflows:authorExclusionSingleProviderFixElsewhere",
  },
  agent_single_provider: {
    problem: "workflows:authorExclusionAgentSingleProvider",
    fixHere: "workflows:authorExclusionAgentSingleProviderFixHere",
    fixElsewhere: "workflows:authorExclusionAgentSingleProviderFixElsewhere",
  },
  check_failed: {
    problem: "workflows:authorExclusionCheckFailed",
    fixHere: "workflows:authorExclusionCheckFailedFixHere",
    fixElsewhere: "workflows:authorExclusionCheckFailedFixElsewhere",
  },
};

const ROLE_KEY: Record<string, string> = {
  reviewer: "task:roleReviewer",
  approver: "task:roleApprover",
};

function roleLabel(t: TFunction, role: string | undefined): string {
  const key = role ? ROLE_KEY[role] : undefined;
  return key ? t(key) : (role ?? "");
}

function WarningItem({
  warning,
  formatProvider,
  surface,
}: {
  warning: AuthorExclusionWarning;
  formatProvider: (providerId: string) => string;
  surface: AuthorExclusionWarningSurface;
}) {
  const { t } = useTranslation();
  const keys = WARNING_KEYS[warning.code] ?? WARNING_KEYS.check_failed;
  const vars = {
    provider: formatProvider(warning.provider_id ?? ""),
    agent: warning.agent_name ?? "",
  };
  const problem = t(keys.problem, vars);
  const fix = t(surface === "routing" ? keys.fixHere : keys.fixElsewhere, vars);
  return (
    <li>
      {warning.workflow_name && (
        <>
          <strong className="font-medium">
            {t("workflows:authorExclusionItemLead", {
              workflow: warning.workflow_name,
              step: warning.step_name ?? "",
              role: roleLabel(t, warning.role),
            })}
          </strong>{" "}
        </>
      )}
      {`${problem} ${fix}`}
    </li>
  );
}

const identity = (providerId: string) => providerId;

/**
 * Non-blocking notice listing steps whose Review on a different provider
 * setting cannot work as set up. Each item names the workflow and step, says
 * why that is bad, and how to fix it. Renders nothing without warnings.
 */
export function AuthorExclusionWarningList({
  warnings,
  formatProvider = identity,
  surface = "elsewhere",
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
          <WarningItem
            key={`${w.code}-${w.workflow_name ?? ""}-${w.step_name ?? ""}-${w.agent_name ?? ""}-${i}`}
            warning={w}
            formatProvider={formatProvider}
            surface={surface}
          />
        ))}
      </ul>
    </div>
  );
}
