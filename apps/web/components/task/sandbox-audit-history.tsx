"use client";

import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { SandboxAuditJob, SandboxAuditView } from "@/lib/api/domains/sandbox-audit-api";
import { isAuditSignerTrustedNow } from "@/lib/sandbox-audit";
import type { AuditModel } from "./sandbox-audit-view";
import { sandboxReportActionClass as auditActionClass } from "./sandbox-report-header";

export function SandboxAuditHistory({ model }: { model: AuditModel }) {
  const { t } = useTranslation();
  return (
    <section className="min-w-0 space-y-3" data-testid="sandbox-audit-history">
      <h3 className="font-medium">{t("agents:auditHistory")}</h3>
      {model.history.length === 0 && !model.isPending && (
        <p className="text-muted-foreground">{t("agents:auditNoHistory")}</p>
      )}
      {model.history.map((view, index) => (
        <AuditRecord key={view.verification.id} view={view} model={model} announce={index === 0} />
      ))}
      {model.canLoadMore && (
        <Button
          type="button"
          variant="outline"
          className={auditActionClass}
          onClick={() => void model.loadMore()}
        >
          {t("agents:auditMore")}
        </Button>
      )}
      {model.isLoadingMore && <p role="status">{t("common:loading")}</p>}
    </section>
  );
}

function AuditRecord({
  view,
  model,
  announce,
}: {
  view: SandboxAuditView;
  model: AuditModel;
  announce: boolean;
}) {
  const { t } = useTranslation();
  const job = view.verification;
  const trusted = isAuditSignerTrustedNow(view, model.currentTrust);
  return (
    <article
      className="min-w-0 space-y-3 border-t pt-3"
      data-testid={`sandbox-audit-job-${job.id}`}
    >
      <div role={announce ? "status" : undefined}>
        <h4 className="font-medium">{t(`agents:auditJobState_${job.state}`)}</h4>
      </div>
      <p className="break-words text-muted-foreground">
        {t("agents:auditRequested", { operator: job.requested_by, time: job.requested_at })}
      </p>
      {job.completed_at && (
        <p className="break-words text-muted-foreground">
          {t("agents:auditCompleted", { time: job.completed_at })}
        </p>
      )}
      {job.failure_code && <p>{t(`agents:auditFailure_${job.failure_code}`)}</p>}
      {job.result && (
        <>
          <dl className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-x-3 gap-y-2">
            <dt>{t("agents:executionAuditIntegrity")}</dt>
            <dd>{t(`agents:executionFactState_${job.result.chain_state}`)}</dd>
            <dt>{t("agents:executionAuditFinalization")}</dt>
            <dd>{t(`agents:executionFactState_${job.result.finalization}`)}</dd>
            <dt>{t("agents:auditLedger")}</dt>
            <dd>{t(`agents:executionFactState_${job.result.ledger_state}`)}</dd>
            <dt>{t("agents:auditSignature")}</dt>
            <dd>
              {t(
                job.result.signature_state === "verified"
                  ? "agents:auditSignatureVerified"
                  : `agents:executionFactState_${job.result.signature_state}`,
              )}
            </dd>
            <dt>{t("agents:auditCurrentTrust")}</dt>
            <dd data-testid="sandbox-audit-current-trust">
              {t(trusted ? "agents:auditSignerTrusted" : "agents:auditSignerUntrusted")}
            </dd>
            <dt>{t("agents:auditEvents")}</dt>
            <dd className="tabular-nums">{job.result.event_count}</dd>
          </dl>
          <AuditRecordDetails job={job} />
        </>
      )}
    </article>
  );
}

function AuditRecordDetails({ job }: { job: SandboxAuditJob }) {
  const { t } = useTranslation();
  if (!job.result) return null;
  const details = [
    [t("agents:auditVerifier"), job.result.verifier_version],
    [t("agents:auditArtifactDigest"), job.artifact_set_digest],
    [t("agents:auditEventRoot"), job.result.event_root],
    [t("agents:auditSelectedKey"), job.result.expected_key_fingerprint],
    [t("agents:auditObservedKey"), job.result.signer_key_fingerprint],
    [t("agents:auditTrustRevision"), job.result.trust_revision],
    [t("agents:auditProviderSession"), job.provider_session_id],
  ];
  return (
    <details className="group min-w-0">
      <summary className="flex min-h-7 cursor-pointer items-center font-medium max-md:min-h-12 pointer-coarse:min-h-12">
        <IconChevronRight aria-hidden className="mr-1 size-3.5 shrink-0 group-open:rotate-90" />
        {t("agents:auditDetails")}
      </summary>
      <p className="py-2 text-muted-foreground">
        {t(
          job.trust.source === "operator"
            ? "agents:auditSelectedTrustOperator"
            : "agents:auditTrustUnconfigured",
          {
            version: job.trust.version,
            operator: job.trust.operator_id,
            time: job.trust.configured_at,
          },
        )}
      </p>
      <dl className="space-y-3 pt-2">
        {details
          .filter(([, value]) => value)
          .map(([label, value]) => (
            <div key={label} className="space-y-1">
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="break-all font-mono">{value}</dd>
            </div>
          ))}
      </dl>
    </details>
  );
}
