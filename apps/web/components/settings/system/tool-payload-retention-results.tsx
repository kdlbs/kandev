import { useTranslation } from "react-i18next";
import type {
  ToolPayloadAge,
  ToolPayloadOperation,
  ToolPayloadRetentionStatus,
} from "@/lib/types/tool-payload-retention";
import { sameAge } from "@/hooks/domains/system/use-tool-payload-retention-draft";
import { formatBytes } from "@/lib/utils/format-bytes";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { RetentionSkippedItems, retentionDate, skippedTotal } from "./retention-shared";

export { retentionDate };

function OperationOutcome({ operation }: { operation: ToolPayloadOperation }) {
  const { t } = useTranslation();
  if (operation.kind === "analysis") {
    if (operation.state === "succeeded" && operation.payload_bytes === 0)
      return <p>{t("system:toolPayload.empty")}</p>;
    return (
      <p>{t("system:toolPayload.estimate", { size: formatBytes(operation.payload_bytes) })}</p>
    );
  }
  return (
    <p>
      {t("system:toolPayload.removed", {
        count: operation.removed_messages,
        size: formatBytes(operation.payload_bytes),
      })}
    </p>
  );
}
function OperationResult({ operation }: { operation: ToolPayloadOperation }) {
  const { t } = useTranslation();
  if (operation.kind === "backup")
    return <p className="text-sm">{t("system:toolPayload.preparing")}</p>;
  const skipped = skippedTotal(operation.skipped);
  return (
    <div className="space-y-1 break-words text-sm">
      <p className="text-xs text-muted-foreground">
        {t(`system:toolPayload.${operation.kind}`)}: {t(`system:toolPayload.${operation.state}`)}
        {operation.finished_at && <> · {retentionDate(operation.finished_at)}</>}
      </p>
      <OperationOutcome operation={operation} />
      <details className="text-xs text-muted-foreground">
        <summary className="cursor-pointer py-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
          {t(
            operation.kind === "analysis"
              ? "system:toolPayload.analysisDetails"
              : "system:toolPayload.runDetails",
          )}
        </summary>
        <p>
          {t("system:toolPayload.progress", {
            scanned: operation.scanned,
            tasks: operation.eligible_tasks,
            messages: operation.eligible_messages,
          })}
        </p>
        {skipped > 0 && (
          <>
            <p>{t("system:toolPayload.skipped", { count: skipped })}</p>
            <RetentionSkippedItems
              skipped={operation.skipped}
              labelKey="system:toolPayload.skipReason"
            />
          </>
        )}
        <p className="text-xs text-muted-foreground">
          {t("system:toolPayload.window", {
            start: retentionDate(operation.started_at),
            end: retentionDate(operation.finished_at) || t("system:toolPayload.running"),
            cutoff: retentionDate(operation.cutoff),
          })}
        </p>
      </details>
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
          </>
        )}
      </div>
      {status.operation?.state === "running" && <OperationResult operation={status.operation} />}
    </div>
  );
}
export function ToolPayloadRetentionResults({ status }: { status: ToolPayloadRetentionStatus }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div className="space-y-1" data-testid="tool-payload-last-run">
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
