"use client";

import { useState } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  groupAuthorExclusionWarnings,
  type AuthorExclusionWarningGroup,
} from "@/lib/author-exclusion-warning-groups";
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

// Catalog keys, not copy: the record keys are the wire warning codes. Fix keys
// for codes with steps are plural keys resolved with the step count.
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
  agent_bound_profile: {
    problem: "workflows:authorExclusionAgentBoundProfile",
    fixHere: "workflows:authorExclusionAgentBoundProfileFix",
    fixElsewhere: "workflows:authorExclusionAgentBoundProfileFix",
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

/** Steps listed before the rest are folded behind a toggle. */
const VISIBLE_STEPS = 3;

function roleLabel(t: TFunction, role: string): string {
  const key = ROLE_KEY[role];
  return key ? t(key) : role;
}

function AffectedSteps({ group }: { group: AuthorExclusionWarningGroup }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  if (group.steps.length === 0) return null;
  const folded = group.steps.length > VISIBLE_STEPS;
  const shown = folded && !expanded ? group.steps.slice(0, VISIBLE_STEPS) : group.steps;
  return (
    <div className="mt-1">
      <ul className="list-disc space-y-0.5 pl-5">
        {shown.map((s, i) => (
          <li key={`${s.workflowName}-${s.stepName}-${s.role}-${i}`}>
            {t("workflows:authorExclusionAffectedStep", {
              workflow: s.workflowName,
              step: s.stepName,
              role: roleLabel(t, s.role),
            })}
          </li>
        ))}
      </ul>
      {folded && (
        <Button
          type="button"
          variant="link"
          size="sm"
          className="h-auto cursor-pointer p-0 pl-5 text-sm"
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded
            ? t("workflows:authorExclusionShowFewerSteps")
            : t("workflows:authorExclusionShowAllSteps", { total: group.steps.length })}
        </Button>
      )}
    </div>
  );
}

function WarningGroup({
  group,
  formatProvider,
  surface,
}: {
  group: AuthorExclusionWarningGroup;
  formatProvider: (providerId: string) => string;
  surface: AuthorExclusionWarningSurface;
}) {
  const { t } = useTranslation();
  const keys = WARNING_KEYS[group.code] ?? WARNING_KEYS.check_failed;
  const vars = {
    provider: formatProvider(group.providerId),
    agent: group.agentName,
    count: Math.max(group.steps.length, 1),
  };
  return (
    <div data-testid="author-exclusion-warning-group">
      <p data-testid="author-exclusion-warning-problem">{t(keys.problem, vars)}</p>
      <AffectedSteps group={group} />
      <p data-testid="author-exclusion-warning-fix" className="mt-1">
        {t(surface === "routing" ? keys.fixHere : keys.fixElsewhere, vars)}
      </p>
    </div>
  );
}

const identity = (providerId: string) => providerId;

/**
 * Non-blocking notice for steps whose Review on a different provider setting
 * cannot work as set up. Steps sharing a cause and a fix are shown as one
 * warning: the problem, the affected steps, then the fix. Renders nothing
 * without warnings.
 */
export function AuthorExclusionWarningList({
  warnings,
  formatProvider = identity,
  surface = "elsewhere",
  testId = "author-exclusion-warnings",
}: Props) {
  const { t } = useTranslation();
  if (warnings.length === 0) return null;
  const groups = groupAuthorExclusionWarnings(warnings);
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
      <div className="mt-2 space-y-3 break-words pl-6">
        {groups.map((g) => (
          <WarningGroup key={g.key} group={g} formatProvider={formatProvider} surface={surface} />
        ))}
      </div>
    </div>
  );
}
