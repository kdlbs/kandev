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

// i18n-exempt: maintainer-facing notes explaining why a route is out of the
// discovery catalog. Read only by catalog.test.ts and settings-routes.test.ts;
// never rendered.
const TO_APPEARANCE = "Redirects to the canonical Appearance page.";
// i18n-exempt: maintainer-facing exclusion note; read by tests only.
const TO_DATA_STORAGE = "Redirects to the canonical Data & storage page.";
// i18n-exempt: maintainer-facing exclusion note; read by tests only.
const TO_KEYBOARD_SHORTCUTS = "Redirects to the canonical Keyboard Shortcuts page.";
// i18n-exempt: maintainer-facing exclusion note; read by tests only.
const TO_TASK_BEHAVIOR = "Redirects to the canonical Task behavior page.";
// i18n-exempt: maintainer-facing exclusion note; read by tests only.
const TO_TERMINAL_EDITORS = "Redirects to the canonical Terminal & Editors page.";
// i18n-exempt: maintainer-facing exclusion note; read by tests only.
const TO_WORKSPACE_INTEGRATIONS = "Redirects into the active workspace's Integrations tab.";

// i18n-exempt: maintainer-facing exclusion notes, asserted by tests only.
export const SETTINGS_DISCOVERY_ROUTE_EXCLUSIONS: Record<string, string> = {
  "/settings": "Owned by the top-level Go to Settings destination.",
  "/settings/preferences": TO_APPEARANCE,
  "/settings/general": "Legacy prefix; redirects to Preferences pages.",
  "/settings/general/appearance": TO_APPEARANCE,
  "/settings/general/changes-panel": TO_APPEARANCE,
  "/settings/general/chat-input": TO_KEYBOARD_SHORTCUTS,
  "/settings/general/editors": TO_TERMINAL_EDITORS,
  "/settings/general/keyboard-shortcuts": TO_KEYBOARD_SHORTCUTS,
  "/settings/general/layouts": "Redirects to the canonical Layouts page.",
  "/settings/general/message-queue": TO_TASK_BEHAVIOR,
  "/settings/general/notifications": "Redirects to the canonical Notifications page.",
  "/settings/general/resource-metrics": TO_APPEARANCE,
  "/settings/general/secrets": "Redirects to the canonical Secrets page.",
  "/settings/general/shell": TO_TERMINAL_EDITORS,
  "/settings/general/sprites": "Redirects to the Executors page, which owns Sprites.",
  "/settings/general/task-actions": TO_TASK_BEHAVIOR,
  "/settings/general/terminal": TO_TERMINAL_EDITORS,
  "/settings/workspace": "Legacy path; redirects to the canonical Workspaces page.",
  "/settings/automations": "Redirects into the active workspace's Automations tab.",
  "/settings/integrations": TO_WORKSPACE_INTEGRATIONS,
  "/settings/integrations/azure-devops": TO_WORKSPACE_INTEGRATIONS,
  "/settings/integrations/github": TO_WORKSPACE_INTEGRATIONS,
  "/settings/integrations/gitlab": TO_WORKSPACE_INTEGRATIONS,
  "/settings/integrations/jira": TO_WORKSPACE_INTEGRATIONS,
  "/settings/integrations/linear": TO_WORKSPACE_INTEGRATIONS,
  "/settings/integrations/sentry": TO_WORKSPACE_INTEGRATIONS,
  "/settings/integrations/slack": TO_WORKSPACE_INTEGRATIONS,
  "/settings/executor/new": "Transient executor creation flow, not a stable setting.",
  "/settings/system": "Redirects to the canonical System Status page.",
  "/settings/system/database": TO_DATA_STORAGE,
  "/settings/system/backups": TO_DATA_STORAGE,
  "/settings/system/storage": TO_DATA_STORAGE,
  "/settings/system/logs": TO_DATA_STORAGE,
  "/settings/system/licenses": "Redirects to the canonical About page.",
  "/settings/system/message-queue": TO_TASK_BEHAVIOR,
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
