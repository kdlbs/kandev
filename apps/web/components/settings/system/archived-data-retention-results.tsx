import { useTranslation } from "react-i18next";
import type {
  ArchivedDataOperation,
  ArchivedDataPolicy,
  ArchivedDataRetentionStatus,
  ArchivedDataTarget,
} from "@/lib/types/archived-data-retention";
import { cutoff } from "@/hooks/domains/system/use-archived-data-retention-draft";
import { formatBytes } from "@/lib/utils/format-bytes";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { RetentionSkippedItems, retentionDate, skippedTotal } from "./retention-shared";

const dayMilliseconds = 24 * 60 * 60 * 1000;

const targetLabel = {
  messages: "system:archivedData.targetMessages",
  cleanup_jobs: "system:archivedData.targetCleanupJobs",
} as const;

// The two targets are reported separately everywhere. Collapsing them into one
// number would let one target's effect be read as the other's.
function TargetResult({ target, labelKey }: { target: ArchivedDataTarget; labelKey: string }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-0.5">
      <p className="text-sm font-medium">{t(labelKey)}</p>
      <p className="text-sm">
        {t("system:archivedData.targetRemoved", {
          count: target.reduced,
          size: formatBytes(target.bytes),
        })}
      </p>
      <p className="text-xs text-muted-foreground">
        {t("system:archivedData.targetScanned", {
          rows: target.rows,
          eligible: target.eligible,
          before: formatBytes(target.total_bytes),
        })}
      </p>
      {target.oversized > 0 && (
        <p className="text-xs text-muted-foreground">
          {t("system:archivedData.targetOversized", { count: target.oversized })}
        </p>
      )}
    </div>
  );
}

function OperationResult({ operation }: { operation: ArchivedDataOperation }) {
  const { t } = useTranslation();
  if (operation.kind === "backup")
    return <p className="text-sm">{t("system:archivedData.preparing")}</p>;
  const skipped = skippedTotal(operation.skipped);
  // A completed pass with no scanned rows found nothing. An analysis never
  // reduces rows, so its zero reduced count must not hide the per-target report.
  const scanned = operation.messages.rows + operation.cleanup_jobs.rows;
  return (
    <div className="space-y-2 break-words text-sm">
      <p className="text-xs text-muted-foreground">
        {t(`system:archivedData.${operation.kind}`)}: {t(`system:archivedData.${operation.state}`)}
        {operation.finished_at && <> · {retentionDate(operation.finished_at)}</>}
      </p>
      {operation.state === "succeeded" && scanned === 0 ? (
        <p>{t("system:archivedData.empty")}</p>
      ) : (
        <>
          <TargetResult target={operation.messages} labelKey={targetLabel.messages} />
          <TargetResult target={operation.cleanup_jobs} labelKey={targetLabel.cleanup_jobs} />
        </>
      )}
      <details className="text-xs text-muted-foreground">
        <summary className="cursor-pointer py-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
          {t(
            operation.kind === "analysis"
              ? "system:archivedData.analysisDetails"
              : "system:archivedData.runDetails",
          )}
        </summary>
        {skipped > 0 && (
          <>
            <p>{t("system:archivedData.skipped", { count: skipped })}</p>
            <RetentionSkippedItems
              skipped={operation.skipped}
              labelKey="system:archivedData.skipReason"
            />
          </>
        )}
        <p className="text-xs text-muted-foreground">
          {t("system:archivedData.window", {
            start: retentionDate(operation.started_at),
            end: retentionDate(operation.finished_at) || t("system:archivedData.running"),
            archivedCutoff: retentionDate(operation.archived_cutoff),
            cleanupCutoff: retentionDate(operation.cleanup_cutoff),
          })}
        </p>
      </details>
      {operation.state === "partial" && <p>{t("system:archivedData.partialHelp")}</p>}
      {operation.error && <p className="text-destructive">{t("system:archivedData.failed")}</p>}
    </div>
  );
}

// An estimate is stale once the policy it measured has been edited, or once a
// day has passed. The saved revision only moves when the policy is saved, so the
// current draft windows are compared at day granularity: recomputing a cutoff
// from the wall clock differs from the stored value by milliseconds, not days.
function windowEdited(storedCutoff: string, days: number, now: Date): boolean {
  const stored = parseTurnTimestamp(storedCutoff);
  if (stored === null) return true;
  const day = BigInt(dayMilliseconds);
  const expected = BigInt(Math.trunc(cutoff(now, days)));
  return stored / day !== expected / day;
}

function analysisIsStale(
  operation: ArchivedDataOperation | undefined,
  status: ArchivedDataRetentionStatus,
  draft: ArchivedDataPolicy | null,
) {
  if (!operation) return false;
  if (operation.policy_revision !== status.policy.revision) return true;
  const finished = operation.finished_at;
  if (!finished || parseTurnTimestamp(finished) === null) return true;
  if (Date.now() - Date.parse(finished) > dayMilliseconds) return true;
  if (!draft) return true;
  const now = new Date();
  return (
    windowEdited(operation.archived_cutoff, draft.archived_age.days, now) ||
    windowEdited(operation.cleanup_cutoff, draft.cleanup_age.days, now)
  );
}

export function ArchivedDataAnalysis({
  status,
  draft,
}: {
  status: ArchivedDataRetentionStatus;
  draft: ArchivedDataPolicy | null;
}) {
  const { t } = useTranslation();
  const analysis = status.last_analysis;
  const stale = analysisIsStale(analysis, status, draft);
  return (
    <div className="space-y-3" data-testid="archived-data-results">
      <div data-testid="archived-data-estimate">
        {!analysis ? (
          <p className="text-sm">{t("system:archivedData.notAnalyzed")}</p>
        ) : (
          <>
            {stale && <p className="text-sm text-amber-600">{t("system:archivedData.stale")}</p>}
            <OperationResult operation={analysis} />
          </>
        )}
      </div>
      {status.operation?.state === "running" && <OperationResult operation={status.operation} />}
    </div>
  );
}

export function ArchivedDataRetentionResults({ status }: { status: ArchivedDataRetentionStatus }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div className="space-y-2" data-testid="archived-data-last-run">
        <p className="text-sm font-medium">{t("system:archivedData.lastCleanup")}</p>
        {status.last_run ? (
          <OperationResult operation={status.last_run} />
        ) : (
          <p className="text-sm">{t("system:archivedData.never")}</p>
        )}
      </div>
      <p className="text-sm">
        {t("system:archivedData.nextCheck")}:{" "}
        {status.policy.enabled
          ? retentionDate(status.next_due_at) || t("system:archivedData.pending")
          : t("system:archivedData.disabled")}
      </p>
    </div>
  );
}
