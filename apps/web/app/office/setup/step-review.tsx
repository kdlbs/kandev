"use client";

import { Badge } from "@kandev/ui/badge";
import { Card } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { useTranslation } from "react-i18next";

type StepReviewProps = {
  workspaceName: string;
  taskPrefix: string;
  agentName: string;
  agentProfileLabel: string;
  executorPreference: string;
  taskTitle: string;
};

// Fallback used only when meta has not been hydrated yet (graceful degradation).
// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// record keys are the persisted executor-type values.
const FALLBACK_EXECUTOR_LABEL_KEYS: Record<string, string> = {
  local_pc: "office:localStandalone",
  local_docker: "office:localDocker",
  sprites: "office:spritesRemoteSandbox",
};

export function StepReview({
  workspaceName,
  taskPrefix,
  agentName,
  agentProfileLabel,
  executorPreference,
  taskTitle,
}: StepReviewProps) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);

  const fallbackExecutorKey = FALLBACK_EXECUTOR_LABEL_KEYS[executorPreference];
  const executorLabel =
    meta?.executorTypes.find((e) => e.id === executorPreference)?.label ??
    // `?? executorPreference` keeps an unknown wire value visible rather than blank.
    (fallbackExecutorKey ? t(fallbackExecutorKey) : executorPreference);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{t("office:reviewAndLaunch")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("office:confirmTheDetailsBelowEverythingCan")}
        </p>
      </div>
      <Card className="divide-y divide-border">
        {/*
          `"KAN"` and `"CEO"` stay literal: they are the exact fallbacks
          `submitOnboarding` persists as the task prefix and the agent name, so a
          translated preview here would not match what gets created.
        */}
        <ReviewRow
          label={t("common:workspace")}
          value={workspaceName || t("office:defaultWorkspace")}
        >
          <Badge variant="secondary" className="ml-2">
            {taskPrefix || "KAN"}
          </Badge>
        </ReviewRow>
        <ReviewRow label={t("office:coordinatorAgent")} value={agentName || "CEO"}>
          {agentProfileLabel && (
            <span className="text-xs text-muted-foreground ml-2">({agentProfileLabel})</span>
          )}
        </ReviewRow>
        <ReviewRow label={t("office:executor")} value={executorLabel} />
        <ReviewRow label={t("office:firstTask")} value={taskTitle || t("office:noInitialTask")} />
      </Card>
    </div>
  );
}

function ReviewRow({
  label,
  value,
  children,
}: {
  label: string;
  value: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-medium flex items-center">
        {value}
        {children}
      </span>
    </div>
  );
}
