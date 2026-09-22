import { GENERAL_SETTINGS_TARGETS as targets } from "@/lib/settings-discovery/catalog/preferences";

export const TASK_BEHAVIOR_TABS = ["tasks", "conversation", "runtime"] as const;
export type TaskBehaviorTab = (typeof TASK_BEHAVIOR_TABS)[number];
export const TASK_BEHAVIOR_TARGET_TABS: Readonly<Record<string, TaskBehaviorTab>> = {
  [targets.creationAutoFocus]: "tasks",
  [targets.agentGeneratedTitles]: "tasks",
  [targets.agentTaskProfile]: "tasks",
  [targets.preventAutoStartOnOpen]: "tasks",
  [targets.archiveConfirmation]: "tasks",
  [targets.unreadMessages]: "conversation",
  [targets.transcriptNavigation]: "conversation",
  [targets.sessionCapacity]: "runtime",
  [targets.messageQueue]: "runtime",
};
const CONTRIBUTOR_TABS: Readonly<Record<string, TaskBehaviorTab>> = {
  "general-creation-auto-focus": "tasks",
  "general-agent-generated-task-titles": "tasks",
  "general-mcp-task-agent-profile-default": "tasks",
  "general-prevent-auto-start-on-open": "tasks",
  "general-task-actions": "tasks",
  "general-unread-divider": "conversation",
  "general-transcript-navigation": "conversation",
  "general-todo-list-panel": "conversation",
  "system-session-capacity": "runtime",
  "system-message-queue": "runtime",
  "general-task-sleep-inhibition": "runtime",
};
export function taskBehaviorTab(id: string): TaskBehaviorTab | undefined {
  return CONTRIBUTOR_TABS[id];
}
export function firstNewAttentionTab(
  current: readonly string[],
  previous: readonly string[],
): TaskBehaviorTab | undefined {
  return TASK_BEHAVIOR_TABS.find((tab) => current.includes(tab) && !previous.includes(tab));
}
