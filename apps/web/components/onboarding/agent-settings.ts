import type { AgentProfile, AvailableAgent, ProfileLaunchSettingsRequest } from "@/lib/types/http";

export type OnboardingAgentDraft = {
  model: string;
  cli_passthrough: boolean;
};

export type AgentSetting = {
  profileId: string;
  draft: OnboardingAgentDraft;
  baseline: OnboardingAgentDraft;
  savedLaunchSettings: ProfileLaunchSettingsRequest;
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
        },
        baseline: {
          model,
          cli_passthrough,
        },
        savedLaunchSettings,
        dirty: false,
      };
    }
  }
  return settings;
}
