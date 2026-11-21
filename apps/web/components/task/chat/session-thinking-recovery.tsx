import { useTranslation } from "react-i18next";
import { useOptionalAppStore } from "@/components/state-provider";
import type { AgentErrorCause } from "@/lib/types/task-status-summary";

function thinkingGuidanceKey(source: AgentErrorCause["setting_source"]) {
  if (source === "agent_profile") return "task:thinkingProfileHelp";
  if (source === "session_override") return "task:thinkingSessionHelp";
  return "task:thinkingUnknownSourceHelp";
}

export function SessionThinkingRecovery({
  sessionId,
  error,
}: {
  sessionId: string;
  error?: { causes?: AgentErrorCause[] } | null;
}) {
  const { t } = useTranslation();
  const profileLink = useOptionalAppStore((state) => {
    const session = state.taskSessions.items[sessionId];
    const id = session?.execution_profile_id || session?.agent_profile_id;
    const profile = state.agentProfiles.items.find((item) => item.id === id);
    return profile
      ? `/settings/agents/${encodeURIComponent(profile.agent_name)}/profiles/${encodeURIComponent(profile.id)}`
      : undefined;
  }, undefined);
  const cause = error?.causes?.find((item) => item.code?.startsWith("thinking_level_"));
  if (!cause) return null;
  const isProfile = cause.setting_source === "agent_profile";
  return (
    <div className="mt-2 space-y-1" data-testid="thinking-recovery-guidance">
      <p className="text-xs text-muted-foreground">
        {t(thinkingGuidanceKey(cause.setting_source))}
      </p>
      {isProfile && profileLink ? (
        <a
          href={profileLink}
          className="inline-flex min-h-7 cursor-pointer items-center text-sm underline max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        >
          {t("agents:editProfile")}
        </a>
      ) : null}
    </div>
  );
}
