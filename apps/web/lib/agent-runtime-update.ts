import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";

export type RuntimeVersionPair = {
  currentVersion: string;
  targetVersion?: string;
  versionsMatch: boolean;
};

export function resolveRuntimeVersionPair(
  preview: AgentUpdatePreview | null,
  job?: AgentUpdateJob,
): RuntimeVersionPair {
  const currentVersion = job?.current_version || preview?.current_version || "Unknown";
  const targetVersion = job?.target_version || preview?.target_version;
  const versionsMatch = Boolean(
    targetVersion && currentVersion !== "Unknown" && currentVersion === targetVersion,
  );
  return { currentVersion, targetVersion, versionsMatch };
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
  const { currentVersion, targetVersion } = resolveRuntimeVersionPair(preview, job);
  return (
    Boolean(
      preview && currentVersion !== "Unknown" && targetVersion && currentVersion !== targetVersion,
    ) &&
    !previewError &&
    !loading &&
    !updateInFlight &&
    !starting &&
    !installInFlight
  );
}
