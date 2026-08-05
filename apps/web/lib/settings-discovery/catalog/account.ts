import type { SettingsDiscoveryDefinition } from "../types";

export const ACCOUNT_SETTINGS_HREF = "/settings/account";
export const ACCOUNT_SECURITY_SETTINGS_HREF = `${ACCOUNT_SETTINGS_HREF}/security`;
export const ACCOUNT_SETTINGS_TARGETS = {
  password: "setting-account-password",
  sessions: "setting-account-sessions",
} as const;

export const ACCOUNT_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [
  {
    id: "account-security",
    kind: "page",
    labelKey: "sidebar:profileAndPassword",
    groupId: "access",
    href: ACCOUNT_SECURITY_SETTINGS_HREF,
    order: 810,
    requires: "account",
  },
  {
    id: "account-tokens",
    kind: "page",
    labelKey: "sidebar:apiTokens",
    groupId: "access",
    href: "/settings/account/tokens",
    order: 820,
    requires: "account",
  },
  {
    id: "account-password",
    kind: "control",
    labelKey: "settings:password",
    parentId: "account-security",
    groupId: "access",
    href: ACCOUNT_SECURITY_SETTINGS_HREF,
    targetId: ACCOUNT_SETTINGS_TARGETS.password,
    order: 811,
    requires: "account",
  },
  {
    id: "account-sessions",
    kind: "section",
    labelKey: "settings:activeSessions",
    parentId: "account-security",
    groupId: "access",
    href: ACCOUNT_SECURITY_SETTINGS_HREF,
    targetId: ACCOUNT_SETTINGS_TARGETS.sessions,
    order: 812,
    requires: "account",
  },
];
