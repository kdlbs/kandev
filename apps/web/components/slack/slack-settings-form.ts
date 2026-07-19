import type { SlackConfig } from "@/lib/types/slack";

export const DEFAULT_PREFIX = "!kandev";
export const DEFAULT_POLL_INTERVAL_SECONDS = 30;
export const MIN_POLL_INTERVAL_SECONDS = 5;
export const MAX_POLL_INTERVAL_SECONDS = 600;

export type SlackSettingsFormState = {
  utilityAgentId: string;
  commandPrefix: string;
  pollIntervalSeconds: number;
  token: string;
  cookie: string;
};

export const emptySlackSettingsForm: SlackSettingsFormState = {
  utilityAgentId: "",
  commandPrefix: DEFAULT_PREFIX,
  pollIntervalSeconds: DEFAULT_POLL_INTERVAL_SECONDS,
  token: "",
  cookie: "",
};

export function slackConfigToForm(cfg: SlackConfig | null): SlackSettingsFormState {
  if (!cfg) return emptySlackSettingsForm;
  return {
    utilityAgentId: cfg.utilityAgentId,
    commandPrefix: cfg.commandPrefix || DEFAULT_PREFIX,
    pollIntervalSeconds: cfg.pollIntervalSeconds || DEFAULT_POLL_INTERVAL_SECONDS,
    token: "",
    cookie: "",
  };
}
