"use client";

import { IconCheck } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { SetupStepId } from "@/lib/coordinators/setup";

export const STEP_LABEL: Record<SetupStepId, string> = {
  identity: "coordinator:setupStepIdentity",
  watches: "coordinator:setupStepWatches",
  projects: "coordinator:setupStepProjects",
  goal: "coordinator:setupStepGoal",
  context: "coordinator:setupStepContext",
  "may-do": "coordinator:setupStepMayDo",
  review: "coordinator:setupStepReview",
};

type Props = {
  steps: readonly SetupStepId[];
  current: SetupStepId;
  marked: ReadonlySet<SetupStepId>;
};

export function SetupStepList({ steps, current, marked }: Props) {
  const { t } = useTranslation();
  const index = steps.indexOf(current);
  return (
    <>
      <p className="text-sm font-medium md:hidden" data-testid="setup-step-compact">
        {t("coordinator:setupStepOf", { current: index + 1, total: steps.length })}
        {": "}
        {t(STEP_LABEL[current])}
      </p>
      <ol
        className="hidden flex-wrap gap-x-4 gap-y-1 text-sm md:flex"
        aria-label={t("coordinator:setupStepListLabel")}
        data-testid="setup-step-list"
      >
        {steps.map((step, i) => {
          const isCurrent = step === current;
          return (
            <li
              key={step}
              data-testid={`setup-step-${step}`}
              aria-current={isCurrent ? "step" : undefined}
              className={isCurrent ? "font-bold" : "text-muted-foreground"}
            >
              <span aria-hidden="true">{isCurrent ? "▶ " : ""}</span>
              {i + 1}. {t(STEP_LABEL[step])}
              {marked.has(step) && !isCurrent && (
                <IconCheck
                  className="ml-1 inline h-3.5 w-3.5"
                  data-testid={`setup-step-done-${step}`}
                  aria-label={t("coordinator:setupStepDone")}
                />
              )}
            </li>
          );
        })}
      </ol>
    </>
  );
}
