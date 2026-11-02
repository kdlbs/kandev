"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import type { ExecutionFacts, RuntimeObservation } from "@/lib/types/execution";
import { unavailableExecutionFacts } from "@/lib/execution-facts";

function FactGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="min-w-0 space-y-2">
      <h3 className="font-medium">{title}</h3>
      {children}
    </section>
  );
}

function FactTime({ value }: { value?: string }) {
  const { t } = useTranslation();
  if (!value || parseTurnTimestamp(value) === null) return null;
  return (
    <p className="break-words text-muted-foreground">
      {t("agents:executionObservedAt", { time: value })}
    </p>
  );
}

export function ExecutionFactsDetails({
  facts: supplied,
}: {
  facts: ExecutionFacts | null | undefined;
}) {
  const { t } = useTranslation();
  const facts = supplied ?? unavailableExecutionFacts();
  const receipt = facts.launch_checks.receipt;
  return (
    <div className="space-y-5" data-testid="execution-facts">
      <FactGroup title={t("agents:executionFactProcess")}>
        <p data-testid="execution-process-state">
          {t(`agents:executionFactState_${facts.process.state}`)}
        </p>
        <FactTime value={facts.process.checked_at} />
      </FactGroup>
      <FactGroup title={t("agents:executionFactLaunch")}>
        <p>{t(`agents:executionFactState_${facts.launch_checks.state}`)}</p>
        {receipt && (
          <>
            <FactTime value={receipt.checked_at} />
            <ul className="list-disc space-y-1 pl-4">
              {receipt.checks.map((check) => (
                <li key={check}>{t(`agents:executionLaunchCheck_${check}`)}</li>
              ))}
            </ul>
            <p className="text-muted-foreground">{t("agents:executionLaunchChecksHelp")}</p>
          </>
        )}
      </FactGroup>
      <FactGroup title={t("agents:executionFactRuntime")}>
        <p>{t(`agents:executionFactState_${facts.runtime.state}`)}</p>
        {facts.runtime.observations?.map((fact) => (
          <RuntimeFact
            key={`${fact.source_id}:${fact.evidence_digest}:${fact.observed_at}`}
            fact={fact}
          />
        ))}
      </FactGroup>
      <FactGroup title={t("agents:executionFactAudit")}>
        <dl className="space-y-1">
          <div>
            <dt className="inline text-muted-foreground">{t("agents:executionAuditIntegrity")}</dt>{" "}
            <dd className="inline">{t(`agents:executionFactState_${facts.audit.integrity}`)}</dd>
          </div>
          <div>
            <dt className="inline text-muted-foreground">
              {t("agents:executionAuditFinalization")}
            </dt>{" "}
            <dd className="inline">{t(`agents:executionFactState_${facts.audit.finalization}`)}</dd>
          </div>
          <div>
            <dt className="inline text-muted-foreground">{t("agents:executionAuditSigner")}</dt>{" "}
            <dd className="inline">{t(`agents:executionFactState_${facts.audit.signer}`)}</dd>
          </div>
        </dl>
      </FactGroup>
    </div>
  );
}

function RuntimeFact({ fact }: { fact: RuntimeObservation }) {
  const { t } = useTranslation();
  const booleanValue = (value?: boolean) => {
    if (value === undefined) return t("agents:executionFactState_unavailable");
    return t(value ? "agents:executionFactState_yes" : "agents:executionFactState_no");
  };
  return (
    <div className="min-w-0 space-y-2 border-l-2 pl-3">
      <p className="break-all">{t("agents:executionRuntimeSource", { source: fact.source_id })}</p>
      <FactTime value={fact.observed_at} />
      <dl className="space-y-1">
        <div>
          <dt className="text-muted-foreground">{t("agents:executionRuntimePlatform")}</dt>
          <dd>{fact.platform}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("agents:executionRuntimeABI")}</dt>
          <dd>{fact.landlock_abi ?? t("agents:executionFactState_unavailable")}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("agents:executionRuntimeExecute")}</dt>
          <dd>{booleanValue(fact.execute_enforced)}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("agents:executionRuntimeMediation")}</dt>
          <dd>{booleanValue(fact.command_mediation_active)}</dd>
        </div>
      </dl>
      <p className="text-muted-foreground">{t("agents:executionRuntimeHistorical")}</p>
    </div>
  );
}
