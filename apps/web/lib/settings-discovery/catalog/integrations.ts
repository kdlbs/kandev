import type { SettingsDiscoveryDefinition } from "../types";

const INTEGRATIONS = [
  ["github", "GitHub"],
  ["gitlab", "GitLab"],
  ["jira", "Jira"],
  ["linear", "Linear"],
  ["sentry", "Sentry"],
] as const;

export const INTEGRATION_SETTINGS_TARGETS = {
  github: "setting-integration-github-connection",
  gitlab: "setting-integration-gitlab-connection",
  jira: "setting-integration-jira-connection",
  linear: "setting-integration-linear-connection",
  sentry: "setting-integration-sentry-connection",
} as const;

// Integrations are workspace-scoped: their discovery entries are generated per
// workspace in resolve.ts, so search results read "<workspace> › Integrations › …".
export const INTEGRATION_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [];

export const WORKSPACE_INTEGRATIONS = INTEGRATIONS;
