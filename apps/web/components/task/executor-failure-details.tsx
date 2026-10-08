"use client";
import { useTranslation } from "react-i18next";
import type { ExecutorFailureEpisode } from "@/lib/types/executor-failure";
import { ExecutorTechnicalEvidence } from "./executor-technical-evidence";

export function ExecutorFailureDetails({ episode }: { episode: ExecutorFailureEpisode }) {
  const { t } = useTranslation();
  const evidence = episode.observation;
  const workerUnavailable =
    evidence.outcome === "unknown" && evidence.reason === "WorkerUnavailable";
  return (
    <div className="min-w-0 space-y-3 text-sm wrap-anywhere" data-testid="executor-failure-details">
      <p>
        {t(
          evidence.workspace === "retained"
            ? "task:executorFailureWorkspaceRetained"
            : "task:executorFailureWorkspaceUnknown",
        )}
      </p>
      {workerUnavailable && <p>{t("task:executorFailureWorkspaceUnavailable")}</p>}
      <p>{t("task:executorFailureConversationUnknown")}</p>
      <p className="text-xs text-muted-foreground">
        {t("task:executorFailureObserved", {
          time: new Date(evidence.observed_at).toLocaleString(),
        })}
      </p>
      <ExecutorTechnicalEvidence evidence={evidence} />
    </div>
  );
}
