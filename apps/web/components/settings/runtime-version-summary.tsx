import type { ReactNode } from "react";
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

export function RuntimeVersionSummary({
  agentName,
  preview,
  job,
  changeVersion,
}: {
  agentName: string;
  preview: AgentUpdatePreview;
  job?: AgentUpdateJob;
  changeVersion: ReactNode;
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
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-3"
      data-testid={`agent-update-version-summary-${agentName}`}
    >
      <PinnedRuntimeVersion agentName={agentName} preview={preview} job={job} />
      {changeVersion}
    </div>
  );
}

function PinnedRuntimeVersion({
  agentName,
  preview,
  job,
}: {
  agentName: string;
  preview: AgentUpdatePreview;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  const { currentVersion, targetVersion, versionsMatch } = resolveRuntimeVersionPair(preview, job);
  const operation = resolveRuntimeOperation(preview, job);
  const isUpToDate = operation === "up_to_date";
  const latestVersion =
    preview.available_versions?.find((version) => version.latest)?.version ??
    preview.stable_latest_version;
  const isLatest = targetVersion && targetVersion === latestVersion && !job;
  let versionLabel = "agents:runtimeTargetVersionLabel";
  if (isUpToDate) versionLabel = "agents:upToDateRuntime";
  else if (isLatest) versionLabel = "agents:runtimeLatestVersionLabel";

  return (
    <div className="min-w-0 space-y-0.5">
      <div className="flex items-center gap-1">
        <p className="text-muted-foreground" role={isUpToDate ? "status" : undefined}>
          {t(versionLabel)}
        </p>
        <RuntimeVersionScope agentName={agentName} preview={preview} job={job} />
      </div>
      <p className="wrap-anywhere font-mono text-xl font-semibold">
        {isUpToDate ? currentVersion : targetVersion || currentVersion}
      </p>
      {targetVersion && !versionsMatch && !isUpToDate && (
        <p className="wrap-anywhere text-muted-foreground">
          {t("agents:runtimeCurrentVersion", { version: currentVersion })}
        </p>
      )}
    </div>
  );
}

function RuntimeVersionScope({
  agentName,
  preview,
  job,
}: {
  agentName: string;
  preview: AgentUpdatePreview;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  const operation = resolveRuntimeOperation(preview, job);
  const migration = operation === "migrate";
  if (!migration && !preview.managed_fallback) return null;
  return (
    <SettingsInfo
      label={t(runtimeOperationLabelKey(operation))}
      testId={`agent-update-info-${agentName}`}
    >
      {migration && <p>{t("agents:openCodeMigrationScope")}</p>}
      <p>
        {t(
          preview.managed_fallback
            ? "agents:runtimeFallbackExplainer"
            : "agents:runtimeUpdateExplainer",
        )}
      </p>
      <p>{t("agents:runtimeUpdateSessionsNote")}</p>
    </SettingsInfo>
  );
}

export function RuntimeVersionFacts({
  preview,
  job,
}: {
  preview: AgentUpdatePreview;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  const currentVersion = job?.current_version || preview.current_version;
  const activeVersion = resolveRuntimeActiveVersion(preview, job);
  const effectiveVersion = resolveRuntimeEffectiveVersion(preview, job);
  return (
    <>
      {activeVersion && activeVersion !== currentVersion && (
        <p>{t("agents:activeRuntimeVersion", { version: activeVersion })}</p>
      )}
      {effectiveVersion && effectiveVersion !== currentVersion && (
        <p>{t("agents:effectiveRuntimeVersion", { version: effectiveVersion })}</p>
      )}
    </>
  );
}
