import { useTranslation } from "react-i18next";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import {
  resolveRuntimeActiveVersion,
  resolveRuntimeEffectiveVersion,
  resolveRuntimeOperation,
  resolveRuntimeVersionPair,
  runtimeOperationLabelKey,
} from "@/lib/agent-runtime-update";
import { SettingsInfo } from "./settings-info";

function updateExplainerKey(preview: AgentUpdatePreview): string {
  if (preview.update_mode === "self_update") return "agents:harnessUpdateChannelNotice";
  if (preview.managed_fallback) return "agents:runtimeFallbackExplainer";
  return "agents:runtimeUpdateExplainer";
}

export function RuntimeVersionSummary({
  agentName,
  preview,
  job,
}: {
  agentName: string;
  preview: AgentUpdatePreview;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  if (preview.update_mode === "self_update") {
    const current = job?.current_version || preview.current_version || t("common:unknown");
    return (
      <div className="space-y-2" data-testid={`agent-update-version-summary-${agentName}`}>
        <p className="font-medium">
          {t(runtimeOperationLabelKey(resolveRuntimeOperation(preview, job)))}
        </p>
        <p className="text-xs text-muted-foreground">{t("agents:harnessUpdateChannelNotice")}</p>
        <div className="grid grid-cols-1 gap-2 text-xs sm:grid-cols-2">
          <p data-testid={`agent-update-current-${agentName}`}>
            {t("agents:installedRuntimeVersion", { version: current })}
          </p>
          <p data-testid={`agent-update-stable-reference-${agentName}`}>
            {t("agents:stableLatestReference", {
              version: preview.stable_latest_version || t("common:unknown"),
            })}
          </p>
        </div>
      </div>
    );
  }
  const { currentVersion, targetVersion } = resolveRuntimeVersionPair(preview, job);
  const activeVersion = resolveRuntimeActiveVersion(preview, job);
  const operation = resolveRuntimeOperation(preview, job);
  const isUpToDate = operation === "up_to_date";
  const effectiveVersion = resolveRuntimeEffectiveVersion(preview, job);

  return (
    <div className="space-y-0.5" data-testid={`agent-update-version-summary-${agentName}`}>
      <div className="flex items-center gap-1">
        <p className="font-medium" role={isUpToDate ? "status" : undefined}>
          {t(runtimeOperationLabelKey(operation))}
        </p>
        <SettingsInfo
          label={t(runtimeOperationLabelKey(operation))}
          testId={`agent-update-info-${agentName}`}
        >
          {operation === "migrate" && <p>{t("agents:openCodeMigrationScope")}</p>}
          <p>{t(updateExplainerKey(preview))}</p>
          <p>{t("agents:runtimeUpdateSessionsNote")}</p>
        </SettingsInfo>
      </div>
      <p className="break-words font-mono text-sm">
        {isUpToDate ? currentVersion : `${currentVersion} → ${targetVersion}`}
      </p>
      <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs text-muted-foreground sm:grid-cols-3">
        {activeVersion && <p>{t("agents:activeRuntimeVersion", { version: activeVersion })}</p>}
        <p>{t("agents:effectiveRuntimeVersion", { version: effectiveVersion })}</p>
        {preview.default_version && (
          <p>{t("agents:kandevDefaultVersion", { version: preview.default_version })}</p>
        )}
      </div>
    </div>
  );
}
