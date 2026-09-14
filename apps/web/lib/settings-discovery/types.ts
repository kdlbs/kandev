export type SettingsDiscoveryKind = "page" | "section" | "control";

export type SettingsDiscoveryDefinition = {
  id: string;
  kind: SettingsDiscoveryKind;
  labelKey?: string;
  label?: string;
  aliasesKey?: string;
  parentId?: string;
  groupId: string;
  href: string;
  targetId?: string;
  order: number;
  requires?: "account" | "users" | "workspace" | "organizations";
};

export type SettingsDiscoveryGroupDefinition = {
  id: string;
  labelKey: string;
  order: number;
};

export type ResolvedSettingsDiscoveryItem = {
  id: string;
  kind: SettingsDiscoveryKind;
  label: string;
  aliases: string[];
  breadcrumb: string[];
  groupId: string;
  groupLabel: string;
  href: string;
  targetId?: string;
  order: number;
};

export type SettingsDiscoveryContext = {
  t: (key: string) => string;
  showAccount: boolean;
  showUsers: boolean;
  showOrganizations: boolean;
  workspaces: Array<{ id: string; name: string }>;
  agents: Array<{
    name: string;
    profiles: Array<{ id: string; name: string; agentDisplayName?: string }>;
  }>;
  executors: Array<{
    type: string;
    profiles?: Array<{ id: string; name: string }>;
  }>;
};
