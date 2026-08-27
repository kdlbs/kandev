import type {
  AgentUpdateJob,
  AgentUpdateOperation,
  AgentUpdatePreview,
  AgentUpdateVersion,
} from "@/lib/api";
import { t } from "@/lib/i18n";

export function latestRuntimeVersions(versions: AgentUpdateVersion[]): AgentUpdateVersion[] {
  return versions;
}

export type RuntimeVersionPair = {
  /** Display copy — localized, and therefore never compared. */
  currentVersion: string;
  /**
   * Whether a real version was reported. The predicate this replaced was
   * `currentVersion !== "Unknown"`, which compared against the English
   * placeholder; once `currentVersion` became localized that test was true in
   * every other locale, so a missing version read as known and the update
   * gate opened. docs/i18n.md calls this out under "Do not translate" — the
   * flag is the state, the string is only for the user.
   */
  hasCurrentVersion: boolean;
  targetVersion?: string;
  versionsMatch: boolean;
};

export function resolveRuntimeVersionPair(
  preview: AgentUpdatePreview | null,
  job?: AgentUpdateJob,
): RuntimeVersionPair {
  const reported = job?.current_version || preview?.current_version;
  const currentVersion = reported || t("common:unknown");
  const targetVersion = job?.target_version || preview?.target_version;
  const versionsMatch = Boolean(targetVersion && reported && reported === targetVersion);
  return { currentVersion, hasCurrentVersion: Boolean(reported), targetVersion, versionsMatch };
}

export function resolveRuntimeActiveVersion(
  preview: AgentUpdatePreview,
  job?: AgentUpdateJob,
): string | undefined {
  if (job?.status === "succeeded" && job.operation === "use_default" && !job.active_version) {
    return undefined;
  }
  return job?.active_version ?? preview.active_version;
}

export function resolveRuntimeEffectiveVersion(
  preview: AgentUpdatePreview,
  job?: AgentUpdateJob,
): string {
  return (
    job?.effective_version ??
    preview.effective_version ??
    resolveRuntimeActiveVersion(preview, job) ??
    resolveRuntimeVersionPair(preview, job).currentVersion
  );
}

export function resolveRuntimeOperation(
  preview: AgentUpdatePreview | null,
  job?: AgentUpdateJob,
): AgentUpdateOperation | undefined {
  return job?.operation ?? preview?.operation;
}

export function runtimeOperationLabelKey(operation: AgentUpdateOperation | undefined): string {
  switch (operation) {
    case "rollback":
      return "agents:rollBackRuntime";
    case "repair":
      return "agents:repairRuntime";
    case "up_to_date":
      return "agents:upToDateRuntime";
    case "use_default":
      return "agents:useKandevDefault";
    case "update":
    default:
      return "agents:updateRuntime";
  }
}

export function canApproveAgentRuntimeUpdate({
  preview,
  job,
  previewError,
  loading,
  updateInFlight,
  starting,
  installInFlight,
}: {
  preview: AgentUpdatePreview | null;
  job?: AgentUpdateJob;
  previewError: string | null;
  loading: boolean;
  updateInFlight: boolean;
  starting: boolean;
  installInFlight: boolean;
}): boolean {
  const { hasCurrentVersion, targetVersion, versionsMatch } = resolveRuntimeVersionPair(
    preview,
    job,
  );
  const operation = resolveRuntimeOperation(preview, job);
  const operationAllowsApproval = operation
    ? operation !== "up_to_date"
    : hasCurrentVersion && !versionsMatch;
  return (
    Boolean(preview && targetVersion && operationAllowsApproval) &&
    !previewError &&
    !loading &&
    !updateInFlight &&
    !starting &&
    !installInFlight
  );
}
