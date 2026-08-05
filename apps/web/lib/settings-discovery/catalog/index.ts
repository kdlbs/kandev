import { ACCOUNT_DISCOVERY_DEFINITIONS } from "./account";
import { AGENT_DISCOVERY_DEFINITIONS } from "./agents";
import { EXECUTOR_DISCOVERY_DEFINITIONS } from "./executors";
import { GENERAL_DISCOVERY_DEFINITIONS } from "./general";
import { INTEGRATION_DISCOVERY_DEFINITIONS } from "./integrations";
import { STANDALONE_DISCOVERY_DEFINITIONS } from "./standalone";
import { SYSTEM_DISCOVERY_DEFINITIONS } from "./system";
import { WORKSPACE_DISCOVERY_DEFINITIONS } from "./workspaces";

export const SETTINGS_DISCOVERY_DEFINITIONS = [
  ...GENERAL_DISCOVERY_DEFINITIONS,
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
  "/settings/general/changes-panel": "Redirects to the canonical Appearance page.",
  "/settings/general/chat-input": "Redirects to the canonical Keyboard Shortcuts page.",
  "/settings/general/resource-metrics": "Redirects to the canonical Appearance page.",
  "/settings/general/shell": "Redirects to the canonical Terminal page.",
  "/settings/executor/new": "Transient executor creation flow, not a stable setting.",
  "/settings/system": "Redirects to the canonical System Status page.",
  "/settings/system/message-queue": "Redirects to the canonical General Message Queue page.",
  "/settings/changelog": "Redirects to the canonical System Updates page.",
};

export * from "./account";
export * from "./agents";
export * from "./executors";
export * from "./general";
export * from "./groups";
export * from "./integrations";
export * from "./standalone";
export * from "./system";
export * from "./workspaces";
