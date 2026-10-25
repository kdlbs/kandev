"use client";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useSessionExecution } from "@/hooks/domains/session/use-session-execution";
import { useExecutionPolicyDetails } from "@/hooks/domains/session/use-execution-policy-details";
import { isExecutionObservationBound } from "@/lib/execution-status";
import { sandboxMessageKey } from "@/lib/sandbox-messages";
import type { ExecutionSnapshot } from "@/lib/types/execution";
import { SandboxPermissions } from "@/components/settings/sandbox-permissions";
import { CommandDetails } from "./execution-command-details";
import { NextExecutionCommand } from "./next-execution-command";
import { SandboxApprovals } from "./sandbox-approvals";
import { SandboxDenialReport } from "./sandbox-denial-report";
import { ExecutionStatusIcon, executionStatusKeys } from "./execution-status-presentation";

function ExecutionDetailSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="min-w-0 space-y-3">
      <h3 className="font-medium">{title}</h3>
      {children}
    </section>
  );
}

function CapturedLaunch({ snapshot }: { snapshot: ExecutionSnapshot }) {
  const { t } = useTranslation();
  const sandbox = snapshot.sandbox;
  return (
    <div className="space-y-4">
      <dl className="space-y-3">
        <div>
          <dt className="text-muted-foreground">{t("agents:agentCommand")}</dt>
          <dd>
            <CommandDetails command={snapshot.agent} />
          </dd>
        </div>
        {snapshot.wrapper && (
          <div>
            <dt className="text-muted-foreground">{t("agents:commandPrefix")}</dt>
            <dd>
              <CommandDetails command={snapshot.wrapper.command} />
            </dd>
          </div>
        )}
        {sandbox && (
          <>
            <div>
              <dt className="text-muted-foreground">{t("agents:sandboxProvider")}</dt>
              <dd className="break-all">
                {sandbox.provider_key} ({sandbox.provider_version})
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t("agents:executionRuntime")}</dt>
              <dd className="break-all">{sandbox.runtime_version}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t("agents:executionPolicy")}</dt>
              <dd className="break-words">
                {sandbox.policy_name} ({sandbox.revision_number})
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t("agents:executionSelectionSource")}</dt>
              <dd>{t(`agents:sandboxSource_${sandbox.selection_source}`)}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t("agents:executionCapturedCeiling")}</dt>
              <dd className="break-all font-mono">{sandbox.ceiling_digest}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t("agents:executionPolicyDigest")}</dt>
              <dd className="break-all font-mono">{sandbox.policy_digest}</dd>
            </div>
          </>
        )}
      </dl>
      <p className="text-muted-foreground">{t("agents:commandCompositionHelp")}</p>
    </div>
  );
}

function useExecutionDetails({ sessionId }: { sessionId: string }) {
  const { session, status } = useSessionExecution(sessionId);
  const candidate = session?.execution_snapshot;
  const snapshot =
    candidate?.schema_version === 1 &&
    candidate.session_id === session?.id &&
    candidate.task_id === session?.task_id
      ? candidate
      : null;
  const observation = isExecutionObservationBound(session?.execution_observation, snapshot)
    ? session.execution_observation
    : null;
  const policies = useExecutionPolicyDetails(snapshot);
  return { status, snapshot, observation, policies };
}

export function SessionExecutionBody({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation();
  const { status, snapshot, observation, policies } = useExecutionDetails({ sessionId });
  return (
    <>
      <div className="space-y-2">
        <p role="status" className="flex items-center gap-2 font-medium">
          <ExecutionStatusIcon status={status} />
          {t(executionStatusKeys[status])}
        </p>
        {observation && (
          <div className="text-muted-foreground">
            <p>{t("agents:executionObservedAt", { time: observation.checked_at })}</p>
            <p>
              {t("agents:executionProviderHealth", {
                health: t(`agents:executionHealth_${observation.provider_health}`),
              })}
            </p>
          </div>
        )}
      </div>
      <ExecutionDetailSection title={t("agents:executionThisAttempt")}>
        {snapshot ? (
          <CapturedLaunch snapshot={snapshot} />
        ) : (
          <p className="text-muted-foreground">{t("agents:executionNoRecord")}</p>
        )}
        {policies.captured && <SandboxPermissions grants={policies.captured.policy.grants} />}
        {snapshot?.sandbox && !policies.captured && (
          <p className="text-muted-foreground">{t("agents:executionPermissionsUnavailable")}</p>
        )}
      </ExecutionDetailSection>
      <ExecutionDetailSection title={t("agents:executionScope")}>
        <p>{t("agents:executionScopeHelp")}</p>
        {status === "active" && (
          <p>{t("agents:executionMechanism", { mechanism: observation?.mechanism })}</p>
        )}
        <p className="text-muted-foreground">{t("agents:executionOutsideScope")}</p>
      </ExecutionDetailSection>
      <ExecutionDetailSection title={t("agents:executionNextLaunch")}>
        <p className="text-muted-foreground">{t("agents:executionNextLaunchHelp")}</p>
        <NextExecutionCommand snapshot={snapshot} />
        <NextPolicy policies={policies} snapshot={snapshot} />
      </ExecutionDetailSection>
      <ExecutionDetailSection title={t("agents:executionApprovals")}>
        <SandboxApprovals sessionId={sessionId} />
      </ExecutionDetailSection>
      <ExecutionDetailSection title={t("agents:executionDenials")}>
        <SandboxDenialReport sessionId={sessionId} />
      </ExecutionDetailSection>
    </>
  );
}

function NextPolicy({
  policies,
  snapshot,
}: {
  policies: ReturnType<typeof useExecutionPolicyDetails>;
  snapshot: ExecutionSnapshot | null;
}) {
  const { t } = useTranslation();
  if (!policies.canPreview || policies.preview.isError)
    return <p>{t("agents:executionPreviewUnavailable")}</p>;
  if (policies.preview.isPending) return <p role="status">{t("common:loading")}</p>;
  const preview = policies.preview.data;
  if (preview?.resolution_error)
    return <p role="alert">{t(sandboxMessageKey(preview.resolution_error))}</p>;
  if (!preview?.resolved) return <p>{t("agents:executionNotConfigured")}</p>;
  const selected = preview.resolved.revision;
  const changed = snapshot?.sandbox?.revision_id !== selected.id;
  return (
    <div className="space-y-2">
      <p className="break-words">
        {selected.policy.name} ({selected.number})
      </p>
      <p>{t(changed ? "agents:executionPolicyChanged" : "agents:executionPolicyUnchanged")}</p>
      <p>
        {t("agents:sandboxInheritedSource", {
          source: t(`agents:sandboxSource_${preview.resolved.selection_source}`),
          ceiling: preview.resolved.ceiling.policy.name,
          revision: preview.resolved.ceiling.number,
        })}
      </p>
      <SandboxPermissions grants={selected.policy.grants} />
    </div>
  );
}
