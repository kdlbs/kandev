import { ACCOUNT_DISCOVERY_DEFINITIONS } from "./account";
import { AGENT_DISCOVERY_DEFINITIONS } from "./agents";
import { EXECUTOR_DISCOVERY_DEFINITIONS } from "./executors";
import { INTEGRATION_DISCOVERY_DEFINITIONS } from "./integrations";
import { PREFERENCES_DISCOVERY_DEFINITIONS } from "./preferences";
import { STANDALONE_DISCOVERY_DEFINITIONS } from "./standalone";
import { SYSTEM_DISCOVERY_DEFINITIONS } from "./system";
import { WORKSPACE_DISCOVERY_DEFINITIONS } from "./workspaces";

export const SETTINGS_DISCOVERY_DEFINITIONS = [
  ...PREFERENCES_DISCOVERY_DEFINITIONS,
  ...WORKSPACE_DISCOVERY_DEFINITIONS,
  ...AGENT_DISCOVERY_DEFINITIONS,
  ...EXECUTOR_DISCOVERY_DEFINITIONS,
  ...STANDALONE_DISCOVERY_DEFINITIONS,
  ...INTEGRATION_DISCOVERY_DEFINITIONS,
  ...SYSTEM_DISCOVERY_DEFINITIONS,
  ...ACCOUNT_DISCOVERY_DEFINITIONS,
];

export const SETTINGS_DISCOVERY_ROUTE_EXCLUSIONS: Record<string, string> = {
  "/settings": "Owned by the top-level Go to Settings destination.",
  "/settings/preferences": "Redirects to the canonical Appearance page.",
  "/settings/general": "Legacy prefix; redirects to Preferences pages.",
  "/settings/general/appearance": "Redirects to the canonical Appearance page.",
  "/settings/general/changes-panel": "Redirects to the canonical Appearance page.",
  "/settings/general/chat-input": "Redirects to the canonical Keyboard Shortcuts page.",
  "/settings/general/editors": "Redirects to the canonical Terminal & Editors page.",
  "/settings/general/keyboard-shortcuts": "Redirects to the canonical Keyboard Shortcuts page.",
  "/settings/general/layouts": "Redirects to the canonical Layouts page.",
  "/settings/general/message-queue": "Redirects to the canonical Task behavior page.",
  "/settings/general/notifications": "Redirects to the canonical Notifications page.",
  "/settings/general/resource-metrics": "Redirects to the canonical Appearance page.",
  "/settings/general/secrets": "Redirects to the canonical Secrets page.",
  "/settings/general/shell": "Redirects to the canonical Terminal & Editors page.",
  "/settings/general/sprites": "Redirects to the Executors page, which owns Sprites.",
  "/settings/general/task-actions": "Redirects to the canonical Task behavior page.",
  "/settings/general/terminal": "Redirects to the canonical Terminal & Editors page.",
  "/settings/workspace": "Legacy path; redirects to the canonical Workspaces page.",
  "/settings/automations": "Redirects into the active workspace's Automations tab.",
  "/settings/integrations": "Redirects into the active workspace's Integrations tab.",
  "/settings/integrations/azure-devops": "Redirects into the active workspace's Integrations tab.",
  "/settings/integrations/github": "Redirects into the active workspace's Integrations tab.",
  "/settings/integrations/gitlab": "Redirects into the active workspace's Integrations tab.",
  "/settings/integrations/jira": "Redirects into the active workspace's Integrations tab.",
  "/settings/integrations/linear": "Redirects into the active workspace's Integrations tab.",
  "/settings/integrations/sentry": "Redirects into the active workspace's Integrations tab.",
  "/settings/integrations/slack": "Redirects into the active workspace's Integrations tab.",
  "/settings/executor/new": "Transient executor creation flow, not a stable setting.",
  "/settings/system": "Redirects to the canonical System Status page.",
  "/settings/system/database": "Redirects to the canonical Data & storage page.",
  "/settings/system/backups": "Redirects to the canonical Data & storage page.",
  "/settings/system/storage": "Redirects to the canonical Data & storage page.",
  "/settings/system/logs": "Redirects to the canonical Data & storage page.",
  "/settings/system/licenses": "Redirects to the canonical About page.",
  "/settings/system/message-queue": "Redirects to the canonical Task behavior page.",
  "/settings/changelog": "Redirects to the canonical System Updates page.",
};

export * from "./account";
export * from "./agents";
export * from "./executors";
export * from "./groups";
export * from "./integrations";
export * from "./preferences";
export * from "./standalone";
export * from "./system";
export * from "./workspaces";
