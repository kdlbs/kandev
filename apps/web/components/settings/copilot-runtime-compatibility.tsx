import { useTranslation } from "react-i18next";
import type { CopilotPluginCompatibility } from "@/lib/api";

export const COPILOT_AGENT_NAME = "copilot-acp";

function affectedProfilesText(
  t: ReturnType<typeof useTranslation>["t"],
  count: number | null,
): string {
  if (count === null) return t("agents:copilotRuntimeProfilesUnknown");
  if (count === 0) return t("agents:copilotRuntimeProfilesNone");
  return t("agents:copilotRuntimeProfilesAffected", { count });
}

/**
 * Render the Copilot runtime plugin-preparation projection before the existing
 * approval action. The notice is advisory: it does not add a gate, change
 * selections, or claim native policy or authentication compatibility.
 */
export function CopilotRuntimeCompatibilityNotice({
  agentName,
  compatibility,
}: {
  agentName: string;
  compatibility?: CopilotPluginCompatibility;
}) {
  const { t } = useTranslation();
  if (agentName !== COPILOT_AGENT_NAME) return null;
  const testId = `agent-update-copilot-compatibility-${agentName}`;
  if (!compatibility) {
    return (
      <p className="break-words text-xs text-muted-foreground" role="status" data-testid={testId}>
        {t("agents:copilotRuntimeCompatibilityUnavailable")}
      </p>
    );
  }
  const versions =
    compatibility.supported_versions.length > 0
      ? compatibility.supported_versions.join(", ")
      : t("common:unknown");
  return (
    <div
      className="space-y-1 rounded-md border bg-muted/30 p-2 text-xs"
      role="status"
      data-testid={testId}
      data-supported={compatibility.supported}
    >
      {compatibility.supported ? (
        <p>
          {t("agents:copilotRuntimePluginSupported", { version: compatibility.runtime_version })}
        </p>
      ) : (
        <>
          <p className="font-medium">
            {t("agents:copilotPluginsRuntimeUnavailableTitle", {
              version: compatibility.runtime_version,
            })}
          </p>
          <p className="text-muted-foreground">
            {t("agents:copilotRuntimePluginUnavailableReason")}
          </p>
          <p data-testid={`${testId}-profiles`}>
            {affectedProfilesText(t, compatibility.selected_profile_count)}
          </p>
          <p>{t("agents:copilotPluginsRuntimeQualified", { versions })}</p>
          <p className="text-muted-foreground">{t("agents:copilotPluginsRuntimeRecovery")}</p>
        </>
      )}
      {compatibility.supported ? (
        <p>{t("agents:copilotPluginsRuntimeQualified", { versions })}</p>
      ) : null}
    </div>
  );
}
