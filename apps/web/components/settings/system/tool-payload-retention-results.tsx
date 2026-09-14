import { useTranslation } from "react-i18next";
import type {
  ToolPayloadAge,
  ToolPayloadOperation,
  ToolPayloadRetentionStatus,
} from "@/lib/types/tool-payload-retention";
import { sameAge } from "@/hooks/domains/system/use-tool-payload-retention-draft";
import { formatBytes } from "@/lib/utils/format-bytes";
import { formatDateTime } from "@/lib/i18n/formats";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

export function retentionDate(value?: string) {
  return value && parseTurnTimestamp(value) !== null ? formatDateTime(value) : "";
}
const skipReasons = new Set([
  "eligibility_budget",
  "protected_tasks",
  "oversize",
  "already_removed",
  "no_payload",
  "unsupported",
  "malformed",
  "invalid_message",
]);
function SkippedItems({ skipped }: { skipped: Record<string, number> }) {
  const { t } = useTranslation();
  return (
    <dl className="space-y-1 text-xs text-muted-foreground">
      {Object.entries(skipped)
        .filter(([, count]) => count > 0)
        .map(([reason, count]) => (
          <div key={reason} className="flex flex-wrap gap-2">
            <dt>
              {t(`system:toolPayload.skipReason.${skipReasons.has(reason) ? reason : "other"}`)}
            </dt>
            <dd>{count}</dd>
          </div>
        ))}
    </dl>
  );
}
function OperationResult({ operation }: { operation: ToolPayloadOperation }) {
  const { t } = useTranslation();
  if (operation.kind === "backup")
    return <p className="text-sm">{t("system:toolPayload.preparing")}</p>;
  const skipped = Object.values(operation.skipped ?? {}).reduce((sum, value) => sum + value, 0);
  return (
    <div className="space-y-1 break-words text-sm">
      <p>
        {t(`system:toolPayload.${operation.kind}`)}: {t(`system:toolPayload.${operation.state}`)}
      </p>
      <p>
        {t("system:toolPayload.progress", {
          scanned: operation.scanned,
          tasks: operation.eligible_tasks,
          messages: operation.eligible_messages,
        })}
      </p>
      <p>
        {operation.kind === "analysis"
          ? t("system:toolPayload.estimate", { size: formatBytes(operation.payload_bytes) })
          : t("system:toolPayload.removed", {
              count: operation.removed_messages,
              size: formatBytes(operation.payload_bytes),
            })}
      </p>
      {skipped > 0 && (
        <>
          <p>{t("system:toolPayload.skipped", { count: skipped })}</p>
          <SkippedItems skipped={operation.skipped} />
        </>
      )}
      <p className="text-xs text-muted-foreground">
        {t("system:toolPayload.window", {
          start: retentionDate(operation.started_at),
          end: retentionDate(operation.finished_at) || t("system:toolPayload.running"),
          cutoff: retentionDate(operation.cutoff),
        })}
      </p>
      {operation.state === "partial" && <p>{t("system:toolPayload.partialHelp")}</p>}
      {operation.error && (
        <p className="text-destructive">{t("system:toolPayload.operationFailed")}</p>
      )}
    </div>
  );
}
export function ToolPayloadAnalysis({
  status,
  age,
}: {
  status: ToolPayloadRetentionStatus;
  age: ToolPayloadAge;
}) {
  const { t } = useTranslation();
  const analysis = status.last_analysis;
  const finished = analysis?.finished_at;
  const stale = Boolean(
    analysis &&
    (!sameAge(analysis.age, age) ||
      !finished ||
      parseTurnTimestamp(finished) === null ||
      Date.now() - Date.parse(finished) > 86400000),
  );
  return (
    <div className="space-y-3" data-testid="tool-payload-results">
      <div data-testid="tool-payload-estimate">
        {!analysis ? (
          <p className="text-sm">{t("system:toolPayload.notAnalyzed")}</p>
        ) : (
          <>
            {stale && <p className="text-sm text-amber-600">{t("system:toolPayload.stale")}</p>}
            <OperationResult operation={analysis} />
            {analysis.state === "succeeded" && analysis.payload_bytes === 0 && (
              <p className="text-sm">{t("system:toolPayload.empty")}</p>
            )}
          </>
        )}
      </div>
      {status.operation?.state === "running" && <OperationResult operation={status.operation} />}
      <p className="text-xs text-muted-foreground">{t("system:toolPayload.compactionHelp")}</p>
    </div>
  );
}
export function ToolPayloadRetentionResults({ status }: { status: ToolPayloadRetentionStatus }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div className="border-t pt-3" data-testid="tool-payload-last-run">
        <p className="text-sm font-medium">{t("system:toolPayload.lastCleanup")}</p>
        {status.last_run ? (
          <OperationResult operation={status.last_run} />
        ) : (
          <p className="text-sm">{t("system:toolPayload.never")}</p>
        )}
      </div>
      <p className="text-sm">
        {t("system:toolPayload.nextCheck")}:{" "}
        {status.policy.enabled
          ? retentionDate(status.next_due_at) || t("system:toolPayload.pending")
          : t("system:toolPayload.disabled")}
      </p>
    </div>
  );
}
