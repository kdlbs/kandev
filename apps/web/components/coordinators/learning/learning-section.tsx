"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { Switch } from "@kandev/ui/switch";
import { useLearning } from "@/hooks/domains/coordinator/use-learning";
import { DreamReportDetail } from "./dream-report-detail";
import { LearningHealthLine } from "./learning-health";
import { LearningMeasures } from "./learning-measures";
import { LearningReports } from "./learning-reports";

type Props = { workspaceId: string; coordinatorId: string; canManage: boolean };

/** The Learning section: shadow dream switch, health, measures, and reports with their detail. */
export function LearningSection({ workspaceId, coordinatorId, canManage }: Props) {
  const { t } = useTranslation();
  const learning = useLearning(workspaceId, coordinatorId);
  const [openId, setOpenId] = useState<string | null>(null);

  if (openId) {
    return (
      <DreamReportDetail
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        dreamId={openId}
        canManage={canManage}
        onBack={() => setOpenId(null)}
      />
    );
  }
  if (learning.status === "error" && !learning.value) {
    return (
      <div className="space-y-2" data-testid="learning-error">
        <p className="text-sm text-destructive">{t("coordinator:loadError")}</p>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer"
          onClick={learning.retry}
        >
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (!learning.value) return <Skeleton className="h-4 w-64" data-testid="learning-loading" />;
  return (
    <div className="space-y-6" data-testid="learning-section">
      <div className="space-y-2">
        <label className="flex min-h-11 cursor-pointer items-center justify-between gap-3 text-sm font-medium">
          {t("coordinator:learningShadowLabel")}
          <Switch
            checked={learning.value.shadow_dream}
            disabled={!canManage || learning.pending}
            onCheckedChange={(on) => void learning.setShadowDream(on)}
            data-testid="learning-shadow-toggle"
          />
        </label>
        <p className="text-xs text-muted-foreground">{t("coordinator:learningShadowHelp")}</p>
        {learning.failed && (
          <p className="text-sm text-destructive" data-testid="learning-toggle-error">
            {t("coordinator:learningToggleFailed")}
          </p>
        )}
      </div>
      <LearningHealthLine health={learning.value.health} />
      <LearningMeasures workspaceId={workspaceId} coordinatorId={coordinatorId} />
      <LearningReports workspaceId={workspaceId} coordinatorId={coordinatorId} onOpen={setOpenId} />
    </div>
  );
}
