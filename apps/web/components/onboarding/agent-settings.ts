import type { AgentProfile, AvailableAgent, ProfileLaunchSettingsRequest } from "@/lib/types/http";

export type OnboardingAgentDraft = {
  model: string;
  cli_passthrough: boolean;
  config_options?: Record<string, string>;
};

export type AgentSetting = {
  profileId: string;
  draft: OnboardingAgentDraft;
  baseline: OnboardingAgentDraft;
  savedLaunchSettings: ProfileLaunchSettingsRequest;
  savedMode?: string;
  dirty: boolean;
};

export function buildAgentSettings(
  avail: AvailableAgent[],
  saved: { name: string; profiles?: AgentProfile[] }[],
): Record<string, AgentSetting> {
  const settings: Record<string, AgentSetting> = {};
  for (const aa of avail) {
    const dbAgent = saved.find((a) => a.name === aa.name);
    const profile = dbAgent?.profiles?.[0];
    if (profile) {
      const savedLaunchSettings: ProfileLaunchSettingsRequest = {
        env_vars: profile.envVars ?? [],
        cli_flags: profile.cliFlags ?? [],
        command_prefix: profile.commandPrefix ?? "",
      };
      const model = profile.model || "";
      const cli_passthrough = profile.cliPassthrough ?? false;
      settings[aa.name] = {
        profileId: profile.id,
        draft: {
          model,
          cli_passthrough,
          config_options: { ...(profile.configOptions ?? {}) },
        },
        baseline: {
          model,
          cli_passthrough,
          config_options: { ...(profile.configOptions ?? {}) },
        },
        savedLaunchSettings,
        savedMode: profile.mode ?? "",
        dirty: false,
      };
    }
  }
  return settings;
}

export function modelOptionsEqual(
  left?: Record<string, string>,
  right?: Record<string, string>,
): boolean {
  const entries = (values?: Record<string, string>) =>
    Object.entries(values ?? {}).sort(([a], [b]) => a.localeCompare(b));
  return JSON.stringify(entries(left)) === JSON.stringify(entries(right));
}

export function onboardingDraftIsDirty(
  draft: OnboardingAgentDraft,
  baseline: OnboardingAgentDraft,
): boolean {
  return (
    draft.model !== baseline.model ||
    draft.cli_passthrough !== baseline.cli_passthrough ||
    !modelOptionsEqual(draft.config_options, baseline.config_options)
  );
}
