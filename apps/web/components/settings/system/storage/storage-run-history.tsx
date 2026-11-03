import { IconChevronRight } from "@tabler/icons-react";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import type { StorageMaintenanceRun } from "@/lib/types/system";
import { StorageGoCacheResult } from "./storage-go-cache-result";
import {
  storageRunSummary,
  type StorageRunDisplayStatus,
  type StorageRunSummary,
} from "./storage-run-summary";
import { formatGigabytes } from "./storage-units";

const STATUS_LABEL_KEYS: Record<StorageRunDisplayStatus, string> = {
  queued: "system:storageRunStatusQueued",
  running: "system:storageRunStatusRunning",
  succeeded: "system:storageRunStatusSucceeded",
  failed: "system:storageRunStatusFailed",
  cancelled: "system:storageRunStatusCancelled",
  skipped_busy: "system:storageRunStatusSkippedBusy",
  incomplete: "system:storageRunStatusIncomplete",
  partial: "system:storageRunStatusPartial",
  unknown: "system:storageRunStatusUnknown",
};

const TRIGGER_LABEL_KEYS: Record<string, string> = {
  scheduled: "system:storageRunTriggerScheduled",
  manual: "system:storageRunTriggerManual",
  analysis: "system:storageRunTriggerAnalysis",
};

function dateLabel(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : formatDateTime(parsed);
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function temporaryArtifactResult(result: Record<string, unknown>): {
  quarantinedBytes: number;
  failed: number;
} | null {
  const outer = recordValue(result.result) ?? result;
  const provider = recordValue(outer.temporary_artifacts);
  const cleanup = recordValue(provider?.result) ?? provider;
  if (!cleanup) return null;
  const rawBytes = cleanup.quarantined_bytes ?? cleanup.reclaimed_bytes;
  if (typeof rawBytes !== "number" || !Number.isFinite(rawBytes) || rawBytes < 0) return null;
  const rawFailed = cleanup.failed;
  return {
    quarantinedBytes: rawBytes,
    failed: typeof rawFailed === "number" && Number.isFinite(rawFailed) ? rawFailed : 0,
  };
}

function TemporaryArtifactRunResult({ result }: { result: Record<string, unknown> }) {
  const { t } = useTranslation();
  const cleanup = temporaryArtifactResult(result);
  if (!cleanup) return null;
  return (
    <div className="space-y-1 text-sm" data-testid="storage-temporary-artifacts-result">
      <div>
        {t("system:storageTemporaryArtifactsQuarantinedResult", {
          size: formatGigabytes(cleanup.quarantinedBytes),
        })}
      </div>
      <div className="text-muted-foreground">
        {t("system:storageTemporaryArtifactsFreedAfterDelete")}
      </div>
      {cleanup.failed > 0 && (
        <div className="text-amber-600">{t("system:storageTemporaryArtifactsMoveFailed")}</div>
      )}
    </div>
  );
}

function genericExplanationKey(summary: StorageRunSummary): string | null {
  if (summary.status === "partial" && summary.hasNonGoProviderError) {
    return "system:storageHistoryOtherCleanupFailed";
  }
  if (
    summary.status === "failed" &&
    (summary.goCache === null ||
      (summary.goCache.issueCategories.length === 0 && !summary.goCache.partial))
  ) {
    return "system:storageHistoryCouldNotComplete";
  }
  return null;
}

function StorageRunTechnicalDetails({
  message,
  result,
}: {
  message: string;
  result: Record<string, unknown>;
}) {
  const { t } = useTranslation();
  return (
    <details className="group" data-testid="storage-run-technical-details">
      <summary
        className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 text-sm text-muted-foreground md:min-h-7 [&::-webkit-details-marker]:hidden"
        data-testid="storage-run-technical-trigger"
      >
        <IconChevronRight
          className="size-4 transition-transform group-open:rotate-90"
          aria-hidden="true"
        />
        {t("system:storageHistoryTechnicalDetails")}
      </summary>
      <div
        className="mt-2 max-h-64 space-y-2 overflow-auto rounded-md border bg-muted/40 p-3"
        data-testid="storage-run-technical-content"
      >
        {message && (
          <div
            className="break-words whitespace-pre-wrap text-muted-foreground"
            data-testid="storage-run-technical-message"
          >
            {message}
          </div>
        )}
        <pre
          className="max-w-full whitespace-pre-wrap break-words text-xs"
          data-testid="storage-run-technical-json"
        >
          {JSON.stringify(result, null, 2)}
        </pre>
      </div>
    </details>
  );
}

function StorageRunRow({ run }: { run: StorageMaintenanceRun }) {
  const { t } = useTranslation();
  const summary = storageRunSummary(run);
  const explanationKey = genericExplanationKey(summary);
  return (
    <AccordionItem value={run.id} data-testid={`storage-run-${run.id}`}>
      <AccordionTrigger className="min-h-11 items-center px-3 no-underline">
        <span className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <Badge
            variant={summary.status === "failed" ? "destructive" : "outline"}
            className={
              summary.status === "partial" || summary.status === "incomplete"
                ? "border-amber-600/40 text-amber-700 dark:text-amber-500"
                : undefined
            }
            data-testid="storage-run-status"
            data-status={summary.status}
          >
            {t(STATUS_LABEL_KEYS[summary.status])}
          </Badge>
          <span data-testid="storage-run-trigger">
            {t(TRIGGER_LABEL_KEYS[run.trigger] ?? "system:storageRunTriggerUnknown")}
          </span>
          <span className="text-muted-foreground">{dateLabel(run.started_at)}</span>
        </span>
      </AccordionTrigger>
      <AccordionContent className="h-auto px-3 text-sm">
        <div className="space-y-2" data-testid="storage-run-result">
          <StorageGoCacheResult
            result={run.result}
            message={run.message}
            busyPolicyEnabled={summary.busyPolicyEnabled}
            testId="storage-go-cache-result"
          />
          <TemporaryArtifactRunResult result={run.result} />
          {explanationKey && (
            <div className="text-amber-600" data-testid="storage-run-generic-explanation">
              {t(explanationKey)}
            </div>
          )}
          <StorageRunTechnicalDetails message={run.message} result={run.result} />
        </div>
      </AccordionContent>
    </AccordionItem>
  );
}

export function StorageRunHistory({
  runs,
  loading = false,
  error,
}: {
  runs: StorageMaintenanceRun[];
  loading?: boolean;
  error?: string | null;
}) {
  const { t } = useTranslation();
  const content = <StorageRunHistoryContent runs={runs} loading={loading} error={error} />;
  return (
    <Card className="min-w-0" data-testid="storage-run-history">
      <CardHeader>
        <CardTitle className="text-base">{t("system:storageMaintenanceHistory")}</CardTitle>
      </CardHeader>
      <CardContent>{content}</CardContent>
    </Card>
  );
}

function StorageRunHistoryContent({
  runs,
  loading,
  error,
}: {
  runs: StorageMaintenanceRun[];
  loading: boolean;
  error?: string | null;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner className="size-4" data-testid="storage-run-history-spinner" />
        {t("system:storageHistoryLoading")}
      </div>
    );
  }
  if (error) {
    return (
      <div className="break-words text-sm text-destructive" data-testid="storage-run-history-error">
        {t("system:storageSectionUnavailable")}: {error}
      </div>
    );
  }
  if (runs.length === 0) {
    return <div className="text-sm text-muted-foreground">{t("system:storageHistoryEmpty")}</div>;
  }
  return (
    <Accordion type="multiple">
      {runs.map((run) => (
        <StorageRunRow key={run.id} run={run} />
      ))}
    </Accordion>
  );
}
