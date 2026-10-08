import { IconAlertTriangle, IconCircleCheck } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { providerRecoveryKey, executorFailureTitleKey } from "@/lib/executor-failure";
import type { ExecutorObservation } from "@/lib/types/executor-failure";

export function ExecutorRecoveryHistory({
  outcome,
  evidence,
  recoveredAt,
  workspace,
  workspaceObservedAt,
}: {
  outcome?: unknown;
  evidence?: ExecutorObservation;
  recoveredAt?: string;
  workspace?: string;
  workspaceObservedAt?: string;
}) {
  const { t } = useTranslation();
  const warning = Boolean(evidence) || outcome !== "restored";
  const Icon = warning ? IconAlertTriangle : IconCircleCheck;
  return (
    <section
      role={warning ? "alert" : "status"}
      className={cn(
        "my-3 flex min-w-0 gap-3 rounded-md border p-3 text-sm wrap-anywhere whitespace-normal md:p-4",
        warning
          ? "border-amber-500/35 bg-amber-500/5 dark:bg-amber-500/10"
          : "border-emerald-500/25 bg-emerald-500/5",
      )}
      data-testid="executor-recovery-history"
    >
      <Icon
        className={cn(
          "mt-0.5 size-4 shrink-0",
          warning ? "text-amber-600 dark:text-amber-400" : "text-emerald-600 dark:text-emerald-400",
        )}
        aria-hidden="true"
      />
      <div className="min-w-0 flex-1 space-y-2">
        {evidence ? (
          <>
            <h3 className="font-medium">{t(executorFailureTitleKey({ observation: evidence }))}</h3>
            <p>{evidence.reason}</p>
            <p className="whitespace-pre-wrap">{evidence.message}</p>
            <p className="text-muted-foreground">
              {t("task:executorFailureObserved", {
                time: new Date(evidence.observed_at).toLocaleString(),
              })}
            </p>
          </>
        ) : (
          <>
            {recoveredAt && (
              <h3 className="font-medium">
                {t("task:executorFailureRecovered", {
                  time: new Date(recoveredAt).toLocaleString(),
                })}
              </h3>
            )}
            <p className="font-medium">{t(providerRecoveryKey(outcome))}</p>
            <p>
              {t(
                workspace === "retained"
                  ? "task:executorFailureWorkspaceRetained"
                  : "task:executorFailureWorkspaceUnknown",
              )}
            </p>
            {workspaceObservedAt && (
              <p className="text-muted-foreground">
                {t("task:executorFailureObserved", {
                  time: new Date(workspaceObservedAt).toLocaleString(),
                })}
              </p>
            )}

            <p className="text-muted-foreground">{t("task:executorFailureTranscriptRetained")}</p>
          </>
        )}
      </div>
    </section>
  );
}
