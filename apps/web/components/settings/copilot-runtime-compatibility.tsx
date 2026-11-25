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
  const state = compatibility?.state ?? "not_checked";
  const version = compatibility?.runtime_version ?? t("common:unknown");
  const profileCount = compatibility?.selected_profile_count ?? null;
  return (
    <div
      className="space-y-1 rounded-md border bg-muted/30 p-2 text-xs"
      role="status"
      data-testid={testId}
      data-state={state}
    >
      <p className={state === "not_checked" ? "font-medium" : undefined}>
        {compatibilityMessage(t, state, version)}
      </p>
      <p data-testid={`${testId}-profiles`}>{affectedProfilesText(t, profileCount)}</p>
    </div>
  );
}

function compatibilityMessage(
  t: ReturnType<typeof useTranslation>["t"],
  state: NonNullable<CopilotPluginCompatibility["state"]>,
  version: string,
): string {
  switch (state) {
    case "compatible":
      return t("agents:copilotRuntimePluginAdvertised", { version });
    case "incompatible":
      return t("agents:copilotRuntimePluginNotAdvertised", { version });
    case "not_checked":
      return t("agents:copilotRuntimeCompatibilityNotChecked", { version });
  }
}
