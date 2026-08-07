import type { SettingsDiscoveryDefinition } from "../types";

const INTEGRATIONS = [
  ["azure-devops", "Azure DevOps"],
  ["github", "GitHub"],
  ["gitlab", "GitLab"],
  ["jira", "Jira"],
  ["linear", "Linear"],
  ["sentry", "Sentry"],
] as const;

export const INTEGRATION_SETTINGS_TARGETS = {
  "azure-devops": "setting-integration-azure-devops-connection",
  github: "setting-integration-github-connection",
  gitlab: "setting-integration-gitlab-connection",
  jira: "setting-integration-jira-connection",
  linear: "setting-integration-linear-connection",
  sentry: "setting-integration-sentry-connection",
} as const;

export const INTEGRATION_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [
  {
    id: "integrations",
    kind: "page",
    labelKey: "common:integrations",
    parentId: "workspaces",
    groupId: "workspaces",
    href: "/settings/integrations",
    order: 220,
  },
  ...INTEGRATIONS.flatMap(([slug, label], index): SettingsDiscoveryDefinition[] => {
    const id = `integration-${slug}`;
    const href = `/settings/integrations/${slug}`;
    const order = 221 + index * 10;
    return [
      {
        id,
        kind: "page",
        label,
        parentId: "integrations",
        groupId: "workspaces",
        href,
        order,
      },
      {
        id: `${id}-connection`,
        kind: "section",
        labelKey: "settings:connection",
        parentId: id,
        groupId: "workspaces",
        href,
        targetId: INTEGRATION_SETTINGS_TARGETS[slug],
        order: order + 1,
        requires: "workspace",
      },
    ];
  }),
];

export const WORKSPACE_INTEGRATIONS = INTEGRATIONS;
