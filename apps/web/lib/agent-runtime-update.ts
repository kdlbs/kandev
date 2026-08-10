import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import { t } from "@/lib/i18n";

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
  const { currentVersion, hasCurrentVersion, targetVersion } = resolveRuntimeVersionPair(
    preview,
    job,
  );
  return (
    Boolean(preview && hasCurrentVersion && targetVersion && currentVersion !== targetVersion) &&
    !previewError &&
    !loading &&
    !updateInFlight &&
    !starting &&
    !installInFlight
  );
}
