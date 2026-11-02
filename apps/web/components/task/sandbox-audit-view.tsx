"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useSandboxAudit } from "@/hooks/domains/session/use-sandbox-audit";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";
import {
  SandboxReportHeader,
  sandboxReportActionClass as auditActionClass,
} from "./sandbox-report-header";
import { SandboxAuditTrustSettings } from "./sandbox-audit-trust";
import { SandboxAuditHistory } from "./sandbox-audit-history";

export type AuditModel = ReturnType<typeof useSandboxAudit>;

export function SandboxAuditView({
  sessionId,
  report,
  onBack,
}: {
  sessionId: string;
  report: SandboxReport;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  const model = useSandboxAudit(sessionId, report);
  return (
    <>
      <SandboxReportHeader
        onBack={onBack}
        title={t("agents:auditTitle")}
        description={t("agents:auditDescription")}
      />
      <div
        className="min-h-0 min-w-0 flex-1 space-y-5 overflow-y-auto overscroll-contain px-1 py-3 text-xs"
        data-testid="sandbox-audit-body"
      >
        <div className="flex flex-wrap items-center gap-3">
          <Button
            type="button"
            variant="outline"
            className={auditActionClass}
            disabled={!model.canRefresh}
            onClick={() => void model.refresh()}
          >
            {t("agents:auditRefresh")}
          </Button>
          {(model.isPending || model.isBusy) && <p role="status">{t("common:loading")}</p>}
        </div>
        {model.failed && <p role="alert">{t("agents:auditReadFailed")}</p>}
        <SandboxAuditTrustSettings model={model} />
        <SandboxAuditHistory model={model} />
      </div>
      <div className="shrink-0 space-y-2 border-t pt-3">
        {!report.data.final_audit && (
          <p className="text-xs text-muted-foreground">
            {t(report.data.finalized ? "agents:auditFilesUnavailable" : "agents:auditStopFirst")}
          </p>
        )}
        {!model.operator && (
          <p className="text-xs text-muted-foreground">{t("agents:auditOperatorOnly")}</p>
        )}
        {model.operator && (
          <div className="flex justify-end">
            <Button
              type="button"
              className={auditActionClass}
              disabled={!model.canVerify}
              onClick={() => void model.verify()}
            >
              {t("agents:auditVerify")}
            </Button>
          </div>
        )}
      </div>
    </>
  );
}
