"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useSandboxReport } from "@/hooks/domains/session/use-sandbox-report";
import { useSandboxDenialVisibility } from "@/hooks/domains/session/use-sandbox-denial-visibility";
import { SandboxReportLearning } from "./sandbox-report-learning";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";

export function SandboxDenialReport({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation();
  const { report, isPending, isError, isFetching, canRefresh, refresh } =
    useSandboxReport(sessionId);
  return (
    <div className="min-w-0 space-y-3" data-testid="sandbox-denial-report">
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-7 min-h-7 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer"
        disabled={!canRefresh}
        onClick={() => void refresh()}
      >
        {t("agents:reportRefresh")}
      </Button>
      {isFetching && <p role="status">{t("common:loading")}</p>}
      {isError && <p role="alert">{t("agents:reportRefreshFailed")}</p>}
      {!report && !isPending && (
        <p className="text-muted-foreground">{t("agents:executionDenialsUnavailable")}</p>
      )}
      {report && <ReportContent report={report} sessionId={sessionId} />}
    </div>
  );
}

function ReportContent({ report, sessionId }: { report: SandboxReport; sessionId: string }) {
  const { t } = useTranslation();
  const visibility = useSandboxDenialVisibility(report);
  const { data } = report;
  return (
    <>
      {data.status !== "available" && (
        <p role="status">
          {t(
            data.status === "unsupported"
              ? "agents:reportUnsupported"
              : "agents:executionDenialsUnavailable",
          )}
        </p>
      )}
      {!data.complete && <p className="text-muted-foreground">{t("agents:reportPartial")}</p>}
      <p>{t(data.finalized ? "agents:reportFinalized" : "agents:reportLive")}</p>
      <p>{t("agents:reportCapturedAt", { time: report.captured_at })}</p>
      <p>
        {t("agents:reportCoverage", {
          classes: (data.supported_classes ?? [])
            .map((kind) => t(`agents:reportClass_${kind}`))
            .join(", "),
        })}
      </p>
      <p>
        {t("agents:reportIntegrity", { integrity: t(`agents:reportIntegrity_${data.integrity}`) })}
      </p>
      <details>
        <summary className="min-h-7 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer py-2">
          {t("agents:reportSources")}
        </summary>
        <dl className="space-y-2 break-all">
          <div>
            <dt>{t("agents:reportDigest")}</dt>
            <dd className="font-mono">{report.digest}</dd>
          </div>
          {(data.sources ?? []).map((source) => (
            <div key={source.id}>
              <dt>{source.schema}</dt>
              <dd className="font-mono">{source.digest}</dd>
              <dd>
                {t("agents:reportIntegrity", {
                  integrity: t(`agents:reportIntegrity_${source.integrity}`),
                })}
              </dd>
            </div>
          ))}
        </dl>
      </details>
      <SandboxReportLearning
        key={report.digest}
        sessionId={sessionId}
        report={report}
        visibility={visibility}
      />
    </>
  );
}
