"use client";

import { CardContent } from "@kandev/ui/card";
import { useTranslation } from "react-i18next";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { formatDateTime } from "@/lib/i18n/formats";
import { buildRetentionStatusViewModel } from "@/lib/utils/retention-status";
import type {
  RetentionStatus,
  RetentionTableCensus,
  RetentionTableSweepResult,
  RetentionSweptTableResult,
  RetentionUnknownStatusCount,
} from "@/lib/types/system";

function formatUnknownStatuses(unknown: RetentionUnknownStatusCount[]): string {
  return unknown.map((item) => `${item.status} (${item.count})`).join(", ");
}

function SweepTableRow({
  label,
  targetId,
  result,
  preview,
}: {
  label: string;
  targetId: string;
  result: RetentionTableSweepResult;
  preview?: RetentionSweptTableResult;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-wrap items-center gap-x-4 gap-y-1 py-1 text-xs"
      data-testid={`retention-swept-row-${targetId}`}
    >
      <span className="min-w-0 font-medium text-foreground">{label}</span>
      <span className="text-muted-foreground">
        {t("system:retentionDeletedLabel")}: {result.deleted}
      </span>
      {preview?.previewed && (
        <span className="text-muted-foreground">
          {t("system:retentionWouldDeleteLabel")}: {preview.would_delete}
        </span>
      )}
      {result.backlog && (
        <span className="text-amber-600">{t("system:retentionBacklogLabel")}</span>
      )}
      {result.error && (
        <span className="text-destructive">
          {t("system:retentionTableErrorLabel")}: {result.error}
        </span>
      )}
    </div>
  );
}

function LastSweepDetails({ status }: { status: RetentionStatus }) {
  const { t } = useTranslation();
  const lastSweep = status.last_sweep;
  if (!lastSweep) return null;
  return (
    <div className="space-y-2" data-testid="retention-last-sweep">
      <p className="text-xs text-muted-foreground">
        {t("system:retentionSweepStartedAtLabel")}: {formatDateTime(lastSweep.started_at)}
        {" · "}
        {t("system:retentionSweepFinishedAtLabel")}: {formatDateTime(lastSweep.finished_at)}
      </p>
      <SweepTableRow
        label={t("system:retentionRoutineHistoryLabel")}
        targetId="office_routine_runs"
        result={lastSweep.office_routine_runs}
        preview={lastSweep.office_routine_runs}
      />
      <SweepTableRow
        label={t("system:retentionAgentRunHistoryLabel")}
        targetId="runs"
        result={lastSweep.runs}
        preview={lastSweep.runs}
      />
      <SweepTableRow
        label={t("system:retentionRunEventsLabel")}
        targetId="run_events"
        result={lastSweep.run_events}
      />
      <SweepTableRow
        label={t("system:retentionProviderAttemptsLabel")}
        targetId="office_run_route_attempts"
        result={lastSweep.route_attempts}
      />
      <SweepTableRow
        label={t("system:retentionRunSkillRecordsLabel")}
        targetId="office_run_skills"
        result={lastSweep.run_skills}
      />
    </div>
  );
}

function RetainedCountRow({
  label,
  targetId,
  census,
  overThreshold,
}: {
  label: string;
  targetId: string;
  census: RetentionTableCensus;
  overThreshold: boolean;
}) {
  const { t } = useTranslation();
  let count: string;
  if (census.state === "not_computed") count = t("system:retentionCensusUnavailable");
  else if (census.retained_count === 0) count = t("system:retentionCensusZero");
  else count = t("system:retentionRecords", { count: census.retained_count });
  return (
    <div
      className="flex flex-wrap items-center gap-x-4 gap-y-1 py-1 text-xs"
      data-testid={`retention-retained-${targetId}`}
    >
      <span className="min-w-0 font-medium text-foreground">{label}</span>
      <span className="text-muted-foreground">{count}</span>
      {census.state !== "not_computed" && (
        <span className="text-muted-foreground">
          {t("system:retentionCensusAsOfLabel")}: {formatDateTime(census.as_of)}
        </span>
      )}
      {census.state === "stale" && (
        <span className="text-amber-600">{t("system:retentionCensusStale")}</span>
      )}
      {census.unknown_statuses && census.unknown_statuses.length > 0 && (
        <span className="text-amber-600">
          {t("system:retentionUnknownStatusesLabel")}:{" "}
          {formatUnknownStatuses(census.unknown_statuses)}
        </span>
      )}
      {census.top_routine_id && (
        <span className="text-muted-foreground">
          {t("system:retentionTopRoutineShareLabel")}:{" "}
          {Math.round((census.top_routine_share ?? 0) * 100)}% ({census.top_routine_id})
        </span>
      )}
      {overThreshold && (
        <span className="text-amber-600">{t("system:retentionAboveWarningThreshold")}</span>
      )}
    </div>
  );
}

function RetentionSweepIssues({ status }: { status: RetentionStatus }) {
  const { t } = useTranslation();
  const lastSweep = status.last_sweep;
  if (!lastSweep) return null;
  const entries = [
    {
      id: "office_routine_runs",
      label: t("system:retentionRoutineHistoryLabel"),
      result: lastSweep.office_routine_runs,
    },
    { id: "runs", label: t("system:retentionAgentRunHistoryLabel"), result: lastSweep.runs },
    {
      id: "run_events",
      label: t("system:retentionRunEventsLabel"),
      result: lastSweep.run_events,
    },
    {
      id: "office_run_route_attempts",
      label: t("system:retentionProviderAttemptsLabel"),
      result: lastSweep.route_attempts,
    },
    {
      id: "office_run_skills",
      label: t("system:retentionRunSkillRecordsLabel"),
      result: lastSweep.run_skills,
    },
  ];
  return (
    <div className="space-y-1 text-xs">
      {entries
        .filter(({ result }) => result.backlog || result.error)
        .map(({ id, label, result }) => (
          <p key={id} data-testid={result.backlog ? `retention-backlog-${id}` : undefined}>
            <span className={result.error ? "text-destructive" : "text-amber-600"}>
              {label}: {result.error || t("system:retentionBacklogLabel")}
            </span>
          </p>
        ))}
    </div>
  );
}

function CleanupDetails({ status }: { status: RetentionStatus }) {
  const { t } = useTranslation();
  return (
    <details data-testid="retention-cleanup-details">
      <summary className="cursor-pointer text-sm font-medium">
        {t("system:retentionCleanupDetailsTitle")}
      </summary>
      <div className="space-y-3 pt-3">
        <LastSweepDetails status={status} />
        {status.skip_count > 0 && (
          <p className="text-xs text-muted-foreground" data-testid="retention-skip-count">
            {t("system:retentionSkipCountLabel")}: {status.skip_count}
            {status.last_skip_at && (
              <>
                {" · "}
                {t("system:retentionLastSkipAtLabel")}: {formatDateTime(status.last_skip_at)}
              </>
            )}
          </p>
        )}
      </div>
    </details>
  );
}

export function RetentionStatusCard({ status }: { status: RetentionStatus | null }) {
  const { t } = useTranslation();
  if (!status) return null;
  const model = buildRetentionStatusViewModel(status);
  const labels = {
    office_routine_runs: t("system:retentionRoutineHistoryLabel"),
    runs: t("system:retentionAgentRunHistoryLabel"),
    run_events: t("system:retentionRunEventsLabel"),
  } as const;
  const outcomeLabel = {
    none: t("system:retentionNeverSweptMessage"),
    success: t("system:retentionOutcomeCompleted"),
    preview: t("system:retentionOutcomePreview"),
    mixed: t("system:retentionOutcomeMixed"),
    error: t("system:retentionOutcomeError"),
  }[model.sweep.outcome];
  return (
    <SettingsCard className="min-w-0" data-testid="retention-status-card">
      <SettingsCardHeader
        title={t("system:retentionStatusTitle")}
        description={t("system:retentionStatusDescription")}
      />
      <CardContent className="space-y-4">
        <div className="grid min-w-0 gap-3 sm:grid-cols-2">
          <div>
            <p className="text-xs text-muted-foreground">
              {t("system:retentionAutomaticCleanupLabel")}
            </p>
            <p className="text-sm font-medium" data-testid="retention-automatic-state">
              {status.settings.enabled
                ? t("system:retentionEnabledValue")
                : t("system:retentionDisabledValue")}
            </p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t("system:retentionLastCleanupLabel")}</p>
            {status.last_sweep ? (
              <p className="text-sm font-medium">
                {outcomeLabel} · {formatDateTime(status.last_sweep.finished_at)}
              </p>
            ) : (
              <p className="text-sm text-muted-foreground" data-testid="retention-never-swept">
                {outcomeLabel}
              </p>
            )}
          </div>
        </div>
        {model.sweep.deletionTotal > 0 && (
          <p className="text-sm" data-testid="retention-deletion-total">
            {t("system:retentionDeletionTotal", { count: model.sweep.deletionTotal })}
          </p>
        )}
        {model.sweep.hasError && (
          <p className="text-sm text-destructive" data-testid="retention-error-notice">
            {t("system:retentionOutcomeError")}
          </p>
        )}
        {model.sweep.hasBacklog && (
          <p className="text-sm text-amber-600" data-testid="retention-backlog-notice">
            {t("system:retentionBacklogNotice")}
          </p>
        )}
        <RetentionSweepIssues status={status} />
        <div>
          <p className="text-sm font-medium">{t("system:retentionRetainedCountsTitle")}</p>
          <div className="grid min-w-0 grid-cols-1 gap-x-4 lg:grid-cols-3">
            {model.counts.map((count) => (
              <RetainedCountRow
                key={count.key}
                label={labels[count.key]}
                targetId={count.key}
                census={count.census}
                overThreshold={count.overThreshold}
              />
            ))}
          </div>
          <p className="pt-2 text-xs text-muted-foreground">
            {t("system:retentionStoredCountsDescription")}
          </p>
          {model.commonAsOf && (
            <p className="pt-1 text-xs text-muted-foreground">
              {t("system:retentionUpdatedLabel")}: {formatDateTime(model.commonAsOf)}
            </p>
          )}
        </div>
        <CleanupDetails status={status} />
      </CardContent>
    </SettingsCard>
  );
}
