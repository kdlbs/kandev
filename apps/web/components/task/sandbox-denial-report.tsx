"use client";

import { useLayoutEffect, useRef, useState } from "react";
import { useSandboxLearning } from "@/hooks/domains/session/use-sandbox-learning";
import { SandboxLearningReview } from "./sandbox-learning-review";
import { SandboxTypedReportContent } from "./sandbox-typed-report";
import { SandboxReportSummary } from "./sandbox-report-summary";
import { SandboxReportHeader } from "./sandbox-report-header";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useSandboxReport } from "@/hooks/domains/session/use-sandbox-report";
import { useSandboxDenialVisibility } from "@/hooks/domains/session/use-sandbox-denial-visibility";
import { SandboxReportLearning } from "./sandbox-report-learning";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";

export function SandboxDenialReport({
  sessionId,
  onBack,
}: {
  sessionId: string;
  onBack: () => void;
}) {
  const query = useSandboxReport(sessionId);
  if (query.report)
    return (
      <ReportContent report={query.report} sessionId={sessionId} onBack={onBack} query={query} />
    );
  return (
    <>
      <SandboxReportHeader onBack={onBack} />
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-1 py-2 text-xs">
        <ReportRefresh query={query} />
      </div>
    </>
  );
}

function ReportRefresh({ query }: { query: ReturnType<typeof useSandboxReport> }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <Button
        type="button"
        variant="outline"
        className="h-7 min-h-7 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer"
        disabled={!query.canRefresh}
        onClick={() => void query.refresh()}
      >
        {t("agents:reportRefresh")}
      </Button>
      {query.isFetching && <p role="status">{t("common:loading")}</p>}
      {query.isError && <p role="alert">{t("agents:reportReadFailed")}</p>}
      {!query.report && !query.isPending && (
        <p className="text-muted-foreground">{t("agents:executionDenialsUnavailable")}</p>
      )}
    </div>
  );
}

function ReportContent({
  report,
  sessionId,
  onBack,
  query,
}: {
  report: SandboxReport;
  sessionId: string;
  onBack: () => void;
  query: ReturnType<typeof useSandboxReport>;
}) {
  const { t } = useTranslation();
  const visibility = useSandboxDenialVisibility(report);
  const { data } = report;
  const model = useSandboxLearning(sessionId, report);
  const [selection, setSelection] = useState({ digest: report.digest, ids: [] as string[] });
  const [reviewDigest, setReviewDigest] = useState<string | null>(null);
  const reviewButton = useRef<HTMLButtonElement>(null);
  const wasReview = useRef(false);
  const reviewing = reviewDigest === report.digest;
  useLayoutEffect(() => {
    if (!reviewing && wasReview.current) reviewButton.current?.focus();
    wasReview.current = reviewing;
  }, [reviewing]);
  const selectedIds = selection.digest === report.digest ? selection.ids : [];
  const setSelectedIds: React.Dispatch<React.SetStateAction<string[]>> = (update) =>
    setSelection((current) => ({
      digest: report.digest,
      ids:
        typeof update === "function"
          ? update(current.digest === report.digest ? current.ids : [])
          : update,
    }));
  if (reviewDigest === report.digest)
    return (
      <SandboxLearningReview
        model={model}
        selectedIds={selectedIds}
        onBack={() => setReviewDigest(null)}
      />
    );
  return (
    <>
      <SandboxReportHeader onBack={onBack} />
      <div
        className="min-w-0 min-h-0 flex-1 space-y-3 overflow-y-auto overscroll-contain px-1 py-2 text-xs"
        data-testid="sandbox-denial-report"
      >
        <ReportRefresh query={query} />
        {data.refresh_error && (
          <p role="alert">
            {t(data.last_successful_read_at ? "agents:reportStale" : "agents:reportReadFailed")}
          </p>
        )}
        <SandboxReportSummary report={report} />
        {data.v2 ? (
          <SandboxTypedReportContent
            report={data.v2}
            visibility={visibility}
            onClearSelection={() => setSelectedIds([])}
            selectedIds={selectedIds}
            onSelect={(id, checked) =>
              setSelectedIds((ids) =>
                checked ? [...ids, id] : ids.filter((value) => value !== id),
              )
            }
          />
        ) : (
          <SandboxReportLearning
            report={report}
            visibility={visibility}
            model={model}
            selectedIds={selectedIds}
            setSelectedIds={setSelectedIds}
            reviewRef={reviewButton}
            onReview={() => setReviewDigest(report.digest)}
          />
        )}
      </div>
      {data.v2 && (
        <div className="shrink-0 border-t pt-3 flex justify-end">
          <Button
            type="button"
            variant="outline"
            className="h-7 min-h-7 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer"
            disabled={selectedIds.length === 0 || (!model.canPropose && !model.revision)}
            ref={reviewButton}
            onClick={() => setReviewDigest(report.digest)}
          >
            {t(model.revision ? "agents:learningViewRevision" : "agents:learningReviewSelected")}
          </Button>
        </div>
      )}
    </>
  );
}
