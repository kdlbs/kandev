"use client";

import { useTranslation } from "react-i18next";
import type { LearningHealth } from "@/lib/api/domains/coordinator-learning-api";
import { knownCondition } from "@/lib/coordinator/learning";

/** The health line: the state, and for a waiting or failed state the condition with its fix. */
export function LearningHealthLine({ health }: { health: LearningHealth }) {
  const { t } = useTranslation();
  const condition = knownCondition(health.condition);
  return (
    <div className="space-y-1 text-sm" data-testid="learning-health" data-state={health.state}>
      <p>
        <span className="text-muted-foreground">{t("coordinator:learningHealthLabel")}: </span>
        <span className="font-medium">{t(`coordinator:learningHealth_${health.state}`)}</span>
      </p>
      {condition && (
        <p className="text-muted-foreground" data-testid="learning-health-condition">
          {t(`coordinator:learningWait_${condition}`)} {t(`coordinator:learningFix_${condition}`)}
        </p>
      )}
      {health.state === "failed" && health.detail && (
        <p className="text-muted-foreground" data-testid="learning-health-reason">
          {t("coordinator:learningFailedReason", { reason: health.detail })}
        </p>
      )}
    </div>
  );
}
