"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { IconAlertCircle } from "@tabler/icons-react";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { settingsControlClassName } from "@/components/settings/settings-control";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useRetentionSettings } from "@/hooks/domains/system/use-retention-settings";
import { formatDateTime } from "@/lib/i18n/formats";
import { SYSTEM_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/system";
import type {
  RetentionSettings,
  RetentionStatus,
  RetentionTableCensus,
  RetentionTableSweepResult,
  RetentionSweptTableResult,
} from "@/lib/types/system";

function serialize(settings: RetentionSettings | null): string {
  return settings ? JSON.stringify(settings) : "loading";
}

function NumberField({
  label,
  help,
  value,
  min,
  max,
  disabled,
  onChange,
  testId,
}: {
  label: string;
  help: string;
  value: number;
  min: number;
  max?: number;
  disabled?: boolean;
  onChange: (value: number) => void;
  testId: string;
}) {
  return (
    <div className="min-w-0 space-y-1">
      <SettingsFieldLabel htmlFor={testId}>{label}</SettingsFieldLabel>
      <Input
        id={testId}
        type="number"
        min={min}
        max={max}
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        className={settingsControlClassName("h-11")}
        data-testid={testId}
      />
      <SettingsFieldDescription>{help}</SettingsFieldDescription>
    </div>
  );
}

function useRetentionDraft(remote: ReturnType<typeof useRetentionSettings>, isAdmin: boolean) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<RetentionSettings | null>(null);
  const previousSaved = useRef<RetentionSettings | null>(null);
  const saved = remote.status?.settings ?? null;

  useEffect(() => {
    if (!saved) return;
    setDraft((current) => {
      const previous = previousSaved.current;
      if (!current || !previous || serialize(current) === serialize(previous)) return saved;
      return current;
    });
    previousSaved.current = saved;
  }, [saved]);

  const isDirty = Boolean(draft && saved && serialize(draft) !== serialize(saved));
  const canEdit = isAdmin && !remote.isLoading && Boolean(saved);
  const invalidReason = !isAdmin ? t("system:retentionAdminOnly") : undefined;

  useSettingsSaveContributor({
    id: "system:retention",
    order: 25,
    revision: serialize(draft),
    isDirty,
    canSave: canEdit,
    invalidReason,
    save: async () => {
      if (!draft) return;
      await remote.save(draft);
    },
    discard: () => {
      if (saved) setDraft(saved);
    },
  });

  return { draft, setDraft, saved, canEdit };
}

function RetentionEnabledRow({
  settings,
  disabled,
  onChange,
}: {
  settings: RetentionSettings;
  disabled: boolean;
  onChange: (settings: RetentionSettings) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-11 items-center justify-between gap-4 border-b py-3">
      <div className="min-w-0 space-y-0.5">
        <SettingsFieldLabel htmlFor="retention-enabled">
          {t("system:retentionEnabledLabel")}
        </SettingsFieldLabel>
        <SettingsFieldDescription>
          {t("system:retentionEnabledDescription")}
        </SettingsFieldDescription>
      </div>
      <Switch
        id="retention-enabled"
        checked={settings.enabled}
        disabled={disabled}
        onCheckedChange={(enabled) => onChange({ ...settings, enabled })}
        data-testid="retention-enabled"
        className="shrink-0 cursor-pointer"
      />
    </div>
  );
}

function RetentionScheduleFields({
  settings,
  disabled,
  onChange,
}: {
  settings: RetentionSettings;
  disabled: boolean;
  onChange: (settings: RetentionSettings) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
      <NumberField
        label={t("system:retentionSweepIntervalLabel")}
        help={t("system:retentionSweepIntervalHelp")}
        value={settings.sweep_interval_hours}
        min={1}
        max={168}
        disabled={disabled}
        onChange={(sweep_interval_hours) => onChange({ ...settings, sweep_interval_hours })}
        testId="retention-sweep-interval"
      />
      <NumberField
        label={t("system:retentionBatchLimitLabel")}
        help={t("system:retentionBatchLimitHelp")}
        value={settings.batch_limit}
        min={100}
        max={100000}
        disabled={disabled}
        onChange={(batch_limit) => onChange({ ...settings, batch_limit })}
        testId="retention-batch-limit"
      />
    </div>
  );
}

function TableSection({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-3 border-b py-4 last:border-b-0">
      <div>
        <p className="text-sm font-medium">{title}</p>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-3">{children}</div>
    </div>
  );
}

type WindowedTableKey = "routine_runs" | "runs";

function WindowedTableSection({
  tableKey,
  title,
  description,
  settings,
  disabled,
  onChange,
}: {
  tableKey: WindowedTableKey;
  title: string;
  description: string;
  settings: RetentionSettings;
  disabled: boolean;
  onChange: (settings: RetentionSettings) => void;
}) {
  const { t } = useTranslation();
  const table = settings[tableKey];
  const testPrefix = tableKey === "routine_runs" ? "retention-routine-runs" : "retention-runs";
  return (
    <TableSection title={title} description={description}>
      <NumberField
        label={t("system:retentionWindowDaysLabel")}
        help={t("system:retentionWindowDaysHelp")}
        value={table.window_days}
        min={1}
        max={3650}
        disabled={disabled}
        onChange={(window_days) => onChange({ ...settings, [tableKey]: { ...table, window_days } })}
        testId={`${testPrefix}-window-days`}
      />
      <NumberField
        label={t("system:retentionFloorPerOwnerLabel")}
        help={t("system:retentionFloorPerOwnerHelp")}
        value={table.floor_per_owner}
        min={0}
        max={10000}
        disabled={disabled}
        onChange={(floor_per_owner) =>
          onChange({ ...settings, [tableKey]: { ...table, floor_per_owner } })
        }
        testId={`${testPrefix}-floor-per-owner`}
      />
      <NumberField
        label={t("system:retentionWarnRowsLabel")}
        help={t("system:retentionWarnRowsHelp")}
        value={table.warn_rows}
        min={0}
        disabled={disabled}
        onChange={(warn_rows) => onChange({ ...settings, [tableKey]: { ...table, warn_rows } })}
        testId={`${testPrefix}-warn-rows`}
      />
    </TableSection>
  );
}

function RunEventsSection({
  settings,
  disabled,
  onChange,
}: {
  settings: RetentionSettings;
  disabled: boolean;
  onChange: (settings: RetentionSettings) => void;
}) {
  const { t } = useTranslation();
  return (
    <TableSection
      title={t("system:retentionRunEventsSectionTitle")}
      description={t("system:retentionRunEventsSectionDescription")}
    >
      <NumberField
        label={t("system:retentionWarnRowsLabel")}
        help={t("system:retentionRunEventsWarnRowsHelp")}
        value={settings.run_events.warn_rows}
        min={0}
        disabled={disabled}
        onChange={(warn_rows) => onChange({ ...settings, run_events: { warn_rows } })}
        testId="retention-run-events-warn-rows"
      />
    </TableSection>
  );
}

function RoutineRunsAndRunsSections({
  settings,
  disabled,
  onChange,
}: {
  settings: RetentionSettings;
  disabled: boolean;
  onChange: (settings: RetentionSettings) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <WindowedTableSection
        tableKey="routine_runs"
        title={t("system:retentionRoutineRunsSectionTitle")}
        description={t("system:retentionRoutineRunsSectionDescription")}
        settings={settings}
        disabled={disabled}
        onChange={onChange}
      />
      <WindowedTableSection
        tableKey="runs"
        title={t("system:retentionRunsSectionTitle")}
        description={t("system:retentionRunsSectionDescription")}
        settings={settings}
        disabled={disabled}
        onChange={onChange}
      />
      <RunEventsSection settings={settings} disabled={disabled} onChange={onChange} />
    </>
  );
}

function RetentionPolicyCard({
  draft,
  canEdit,
  onChange,
}: {
  draft: RetentionSettings;
  canEdit: boolean;
  onChange: (settings: RetentionSettings) => void;
}) {
  const { t } = useTranslation();
  const disabled = !canEdit;
  return (
    <SettingsCard
      className="min-w-0"
      discoveryTargetId={SYSTEM_SETTINGS_TARGETS.retention}
      data-testid="retention-policy-card"
    >
      <SettingsCardHeader
        title={t("system:retentionPolicyTitle")}
        description={t("system:retentionPolicyDescription")}
      />
      <CardContent>
        <RetentionEnabledRow settings={draft} disabled={disabled} onChange={onChange} />
        <RetentionScheduleFields settings={draft} disabled={disabled} onChange={onChange} />
        <RoutineRunsAndRunsSections settings={draft} disabled={disabled} onChange={onChange} />
        {!canEdit && (
          <p className="pt-3 text-xs text-muted-foreground">{t("system:retentionAdminOnly")}</p>
        )}
      </CardContent>
    </SettingsCard>
  );
}

function SweptTableRow({ label, result }: { label: string; result: RetentionSweptTableResult }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-wrap items-center gap-x-4 gap-y-1 py-1 text-xs"
      data-testid={`retention-swept-row-${label}`}
    >
      <span className="min-w-0 font-medium text-foreground">{label}</span>
      <span className="text-muted-foreground">
        {t("system:retentionDeletedLabel")}: {result.deleted}
      </span>
      {result.previewed && (
        <span className="text-muted-foreground">
          {t("system:retentionWouldDeleteLabel")}: {result.would_delete}
        </span>
      )}
      {result.backlog && (
        <span className="text-amber-600" data-testid={`retention-backlog-${label}`}>
          {t("system:retentionBacklogLabel")}
        </span>
      )}
      {result.error && (
        <span className="text-destructive">
          {t("system:retentionTableErrorLabel")}: {result.error}
        </span>
      )}
    </div>
  );
}

function SatelliteTableRow({
  label,
  result,
}: {
  label: string;
  result: RetentionTableSweepResult;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 py-1 text-xs">
      <span className="min-w-0 font-medium text-foreground">{label}</span>
      <span className="text-muted-foreground">
        {t("system:retentionDeletedLabel")}: {result.deleted}
      </span>
      {result.error && (
        <span className="text-destructive">
          {t("system:retentionTableErrorLabel")}: {result.error}
        </span>
      )}
    </div>
  );
}

function LastSweepSection({ status }: { status: RetentionStatus }) {
  const { t } = useTranslation();
  const lastSweep = status.last_sweep;
  if (!lastSweep) {
    return (
      <p className="text-sm text-muted-foreground" data-testid="retention-never-swept">
        {t("system:retentionNeverSweptMessage")}
      </p>
    );
  }
  return (
    <div className="space-y-2" data-testid="retention-last-sweep">
      <p className="text-xs text-muted-foreground">
        {t("system:retentionSweepStartedAtLabel")}: {formatDateTime(lastSweep.started_at)}
        {" · "}
        {t("system:retentionSweepFinishedAtLabel")}: {formatDateTime(lastSweep.finished_at)}
      </p>
      <SweptTableRow label="office_routine_runs" result={lastSweep.office_routine_runs} />
      <SweptTableRow label="runs" result={lastSweep.runs} />
      <SatelliteTableRow label="run_events" result={lastSweep.run_events} />
      <SatelliteTableRow label="office_run_route_attempts" result={lastSweep.route_attempts} />
      <SatelliteTableRow label="office_run_skills" result={lastSweep.run_skills} />
    </div>
  );
}

function RetainedCountRow({ label, census }: { label: string; census: RetentionTableCensus }) {
  const { t } = useTranslation();
  if (census.state === "not_computed") {
    return (
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 py-1 text-xs">
        <span className="min-w-0 font-medium text-foreground">{label}</span>
        <span className="text-muted-foreground">{t("system:retentionCensusNotComputed")}</span>
      </div>
    );
  }
  return (
    <div
      className="flex flex-wrap items-center gap-x-4 gap-y-1 py-1 text-xs"
      data-testid={`retention-retained-${label}`}
    >
      <span className="min-w-0 font-medium text-foreground">{label}</span>
      <span className="text-muted-foreground">{census.retained_count}</span>
      <span className="text-muted-foreground">
        {t("system:retentionCensusAsOfLabel")}: {formatDateTime(census.as_of)}
      </span>
      {census.state === "stale" && (
        <span className="text-amber-600">{t("system:retentionCensusStale")}</span>
      )}
      {census.unknown_statuses && census.unknown_statuses.length > 0 && (
        <span className="text-amber-600">
          {t("system:retentionUnknownStatusesLabel")}: {census.unknown_statuses.join(", ")}
        </span>
      )}
      {census.top_routine_id && (
        <span className="text-muted-foreground">
          {t("system:retentionTopRoutineShareLabel")}:{" "}
          {Math.round((census.top_routine_share ?? 0) * 100)}% ({census.top_routine_id})
        </span>
      )}
    </div>
  );
}

function RetentionStatusCard({ status }: { status: RetentionStatus | null }) {
  const { t } = useTranslation();
  if (!status) return null;
  return (
    <SettingsCard className="min-w-0" data-testid="retention-status-card">
      <SettingsCardHeader
        title={t("system:retentionStatusTitle")}
        description={t("system:retentionStatusDescription")}
      />
      <CardContent className="space-y-4">
        <LastSweepSection status={status} />
        <div>
          <p className="text-sm font-medium">{t("system:retentionRetainedCountsTitle")}</p>
          <RetainedCountRow
            label="office_routine_runs"
            census={status.retained_counts.office_routine_runs}
          />
          <RetainedCountRow label="runs" census={status.retained_counts.runs} />
          <RetainedCountRow label="run_events" census={status.retained_counts.run_events} />
        </div>
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
      </CardContent>
    </SettingsCard>
  );
}

function RetentionSettingsLoading() {
  const { t } = useTranslation();
  return (
    <SettingsCard data-testid="retention-loading">
      <CardContent className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
        <Spinner className="size-4" />
        {t("settings:loading")}
      </CardContent>
    </SettingsCard>
  );
}

function RetentionSettingsLoadError({ error }: { error: string }) {
  const { t } = useTranslation();
  return (
    <Alert variant="destructive" data-testid="retention-load-error">
      <IconAlertCircle className="size-4" />
      <AlertDescription className="break-words">
        {t("system:retentionLoadFailed")}: {error}
      </AlertDescription>
    </Alert>
  );
}

export function RetentionSettingsCard() {
  const remote = useRetentionSettings();
  const isAdmin = useIsAdmin();
  const { draft, setDraft, canEdit } = useRetentionDraft(remote, isAdmin);
  const { t } = useTranslation();

  if (remote.isLoading && !remote.status) return <RetentionSettingsLoading />;
  if (remote.error && !remote.status) return <RetentionSettingsLoadError error={remote.error} />;

  return (
    <div className="min-w-0 space-y-4" data-testid="retention-settings">
      {draft && <RetentionPolicyCard draft={draft} canEdit={canEdit} onChange={setDraft} />}
      <RetentionStatusCard status={remote.status} />
      {remote.saveError && (
        <Alert variant="destructive" data-testid="retention-save-error">
          <IconAlertCircle className="size-4" />
          <AlertDescription className="break-words">
            {t("system:retentionSaveFailed")}: {remote.saveError}
          </AlertDescription>
        </Alert>
      )}
    </div>
  );
}
