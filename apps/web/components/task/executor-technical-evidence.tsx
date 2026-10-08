import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import type { ExecutorObservation } from "@/lib/types/executor-failure";

export function ExecutorTechnicalEvidence({ evidence }: { evidence: ExecutorObservation }) {
  const { t } = useTranslation();
  return (
    <details className="min-w-0 border-t pt-2 text-xs text-muted-foreground">
      <summary className="flex min-h-7 cursor-pointer list-none items-center gap-1.5 rounded-sm focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
        <IconChevronDown className="size-3.5" aria-hidden="true" />
        {t("task:executorFailureTechnicalDetails")}
      </summary>
      <div
        className="mt-2 space-y-3 rounded-md bg-muted/40 p-3"
        data-testid="executor-technical-evidence"
      >
        <dl className="space-y-1.5">
          <EvidenceRow label={t("common:executor")} value={evidence.runtime} />
          <EvidenceRow label={t("common:status")} value={evidence.pod_phase ?? evidence.outcome} />
          {evidence.reason && <EvidenceRow label={t("office:reason")} value={evidence.reason} />}
        </dl>
        {evidence.containers?.map((container) => (
          <div key={container.name} className="border-t pt-2">
            <p className="mb-1.5 font-medium text-foreground">{container.name}</p>
            <dl className="space-y-1.5">
              <EvidenceRow label={t("common:status")} value={container.reason ?? container.state} />
              {(container.exit_code ?? container.last_exit_code) !== undefined && (
                <EvidenceRow
                  label={t("task:executorFailureExitCode")}
                  value={container.exit_code ?? container.last_exit_code}
                />
              )}
              {container.restarts !== undefined && (
                <EvidenceRow label={t("task:restarts")} value={container.restarts} />
              )}
            </dl>
          </div>
        ))}
        {evidence.pod_conditions?.map((condition, index) => (
          <div key={index} className="border-t pt-2">
            <p className="font-medium text-foreground">{condition.type}</p>
            <p className="mt-1 font-mono wrap-anywhere">
              {condition.status}
              {condition.reason && <>: {condition.reason}</>}
            </p>
            {condition.message && <p className="mt-1 wrap-anywhere">{condition.message}</p>}
          </div>
        ))}
        {evidence.secondary?.map((secondary, index) => (
          <div key={index} className="border-t pt-2">
            <p className="font-mono wrap-anywhere">
              {secondary.operation}: {secondary.reason}
            </p>
            <p className="mt-1">
              {t("task:executorFailureObserved", {
                time: new Date(secondary.occurred_at).toLocaleString(),
              })}
            </p>
          </div>
        ))}
      </div>
    </details>
  );
}
function EvidenceRow({ label, value }: { label: string; value: string | number | undefined }) {
  return (
    <div className="flex min-w-0 flex-wrap justify-between gap-x-4 gap-y-1">
      <dt>{label}</dt>
      <dd className="min-w-0 font-mono text-foreground wrap-anywhere">{value}</dd>
    </div>
  );
}
