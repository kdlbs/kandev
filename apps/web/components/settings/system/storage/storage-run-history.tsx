import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import type { StorageMaintenanceRun } from "@/lib/types/system";
import { StorageGoCacheResult } from "./storage-go-cache-result";
import {
  StorageRunTechnicalDetails,
  TemporaryArtifactRunResult,
} from "./storage-run-result-details";
import {
  storageRunSummary,
  type StorageRunDisplayStatus,
  type StorageRunSummary,
} from "./storage-run-summary";

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

function genericExplanationKey(summary: StorageRunSummary): string | null {
  if (summary.hasNonGoProviderError) {
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
