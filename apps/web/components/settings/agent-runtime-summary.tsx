"use client";

import { useTranslation } from "react-i18next";
import type { AgentUpdateStatus } from "@/lib/api";
import type { AgentDiscovery, RuntimeUpdate } from "@/lib/types/http";
import { presentAgentRuntimeIdentity } from "@/lib/agent-runtime-identity";
import { presentHostCLIVersion } from "@/lib/agent-host-cli";

export function AgentRuntimeSummary({
  agent,
  displayName,
  runtime,
  status,
}: {
  agent: AgentDiscovery;
  displayName: string;
  runtime?: RuntimeUpdate;
  status?: AgentUpdateStatus;
}) {
  const { t } = useTranslation();
  const identity = presentAgentRuntimeIdentity(agent.name, runtime, status);
  const labels = {
    managed: t("agents:sessionRuntimeManaged"),
    native: t("agents:sessionRuntimeNative"),
    harness: t("agents:sessionRuntimeHarness"),
    custom: t("agents:sessionRuntimeCustom"),
    unknown: t("agents:sessionRuntimeUnknown"),
  };
  return (
    <div className="min-w-0 space-y-1 text-xs text-muted-foreground">
      <div data-testid={`session-runtime-${agent.name}`}>
        <p>{labels[identity.kind]}</p>
        {identity.kind === "managed" && (
          <p className="break-all" data-testid={`session-runtime-package-${agent.name}`}>
            {t("agents:sessionRuntimePackage", {
              package: identity.package,
              version: identity.version || t("agents:sessionRuntimeVersionUnknown"),
            })}
          </p>
        )}
        <p>{t("agents:sessionRuntimeNewSessions")}</p>
      </div>
      <AgentCLIDiagnostics agent={agent} displayName={displayName} />
    </div>
  );
}

function AgentCLIDiagnostics({
  agent,
  displayName,
}: {
  agent: AgentDiscovery;
  displayName: string;
}) {
  const { t } = useTranslation();
  const version = presentHostCLIVersion(agent);
  if (!agent.matched_path && !version) return null;
  return (
    <details data-testid={`agent-diagnostics-${agent.name}`}>
      <summary className="min-h-11 cursor-pointer content-center md:min-h-7 [@media(pointer:coarse)]:min-h-11">
        {t("agents:runtimeDiagnostics")}
      </summary>
      <div className="space-y-1 break-all pb-1">
        {agent.matched_path && (
          <p>{t("agents:runtimeDetectedLocalCLI", { path: agent.matched_path })}</p>
        )}
        {version && (
          <p data-testid={`cli-version-${agent.name}`}>
            {version.kind === "known"
              ? t("agents:runtimeLocalCLIVersion", { name: displayName, version: version.version })
              : t("agents:runtimeLocalCLIVersionUnknown", {
                  name: displayName,
                  reason: version.reason,
                })}
          </p>
        )}
      </div>
    </details>
  );
}
