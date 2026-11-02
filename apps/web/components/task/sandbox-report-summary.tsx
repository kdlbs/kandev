"use client";
import { useTranslation } from "react-i18next";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";
export function SandboxReportSummary({ report }: { report: SandboxReport }) {
  const { t } = useTranslation();
  const { data } = report;
  const legacyUnavailable =
    data.status === "unsupported"
      ? "agents:reportUnsupported"
      : "agents:executionDenialsUnavailable";
  const unavailable = data.v2 ? "agents:reportCollectionUnavailable" : legacyUnavailable;
  return (
    <>
      {data.status !== "available" && !data.refresh_error && <p role="status">{t(unavailable)}</p>}
      {!data.complete && <p className="text-muted-foreground">{t("agents:reportPartial")}</p>}
      {!data.v2 && Boolean(data.sources?.length) && (
        <p>{t(data.finalized ? "agents:reportFinalized" : "agents:reportLive")}</p>
      )}
      <ReportReadAge report={report} />
      {!data.v2 && (
        <>
          <p>
            {t("agents:reportCoverage", {
              classes: (data.supported_classes ?? [])
                .map((kind) => t(`agents:reportClass_${kind}`))
                .join(", "),
            })}
          </p>
          <p>
            {t("agents:reportIntegrity", {
              integrity: t(`agents:reportIntegrity_${data.integrity}`),
            })}
          </p>
        </>
      )}
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
    </>
  );
}

function ReportReadAge({ report }: { report: SandboxReport }) {
  const { t } = useTranslation();
  const { data } = report;
  return (
    <p>
      {data.last_successful_read_at || data.status === "available"
        ? t("agents:reportLastSuccessfulRead", {
            time: data.last_successful_read_at ?? report.captured_at,
          })
        : t("agents:reportNoSuccessfulRead")}
    </p>
  );
}
