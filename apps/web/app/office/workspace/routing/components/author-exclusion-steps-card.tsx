"use client";

import { Badge } from "@kandev/ui/badge";
import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";
import type { AuthorExclusionStep } from "@/lib/types/author-exclusion";

/** Draft switch values keyed by authorExclusionStepKey. */
export type AuthorExclusionDraft = Record<string, boolean>;

export function authorExclusionStepKey(step: Pick<AuthorExclusionStep, "step_id" | "role">) {
  return `${step.step_id}:${step.role}`;
}

// Catalog keys, not copy: the record keys are participant role identifiers.
const ROLE_KEY: Record<string, string> = {
  reviewer: "task:roleReviewer",
  approver: "task:roleApprover",
};

type Props = {
  steps: AuthorExclusionStep[];
  draft: AuthorExclusionDraft;
  onChange: (key: string, enabled: boolean) => void;
  disabled?: boolean;
};

/**
 * Review on a different provider switches, one per seat step. Lives on the
 * routing page until the Office workflow editor (issue #5347) can host it.
 */
export function AuthorExclusionStepsCard({ steps, draft, onChange, disabled }: Props) {
  const { t } = useTranslation();
  return (
    <section
      className="rounded-lg border border-border p-4 space-y-3"
      data-testid="author-exclusion-steps"
    >
      <div>
        <h2 className="text-sm font-medium">{t("office:authorExclusionStepsTitle")}</h2>
        <p className="text-xs text-muted-foreground mt-0.5">
          {t("office:authorExclusionStepsDescription")}
        </p>
      </div>
      {steps.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("office:authorExclusionStepsEmpty")}</p>
      ) : (
        <ul className="divide-y divide-border">
          {steps.map((step) => {
            const key = authorExclusionStepKey(step);
            const role = ROLE_KEY[step.role] ? t(ROLE_KEY[step.role]) : step.role;
            const names = { workflow: step.workflow_name, step: step.step_name };
            return (
              <li
                key={key}
                className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 py-2 [@media(pointer:coarse)]:min-h-11"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <span className="text-sm break-words">
                    {t("office:authorExclusionStepLabel", names)}
                  </span>
                  <Badge variant="outline">{role}</Badge>
                </div>
                <Switch
                  checked={draft[key] ?? step.enabled}
                  onCheckedChange={(v) => onChange(key, v)}
                  disabled={disabled}
                  aria-label={t("office:authorExclusionStepSwitchLabel", { ...names, role })}
                  className="cursor-pointer [@media(pointer:coarse)]:scale-125"
                />
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
