"use client";

import { useTranslation } from "react-i18next";
import { useEffect, useRef, useState } from "react";
import { AgentGeneratedTaskTitleSettings } from "@/components/settings/agent-generated-task-title-settings";
import { AnchoredPromptBarSettings } from "@/components/settings/anchored-prompt-bar-settings";
import { ArchiveConfirmationSettings } from "@/components/settings/archive-confirmation-settings";
import { CreationAutoFocusSettings } from "@/components/settings/creation-auto-focus-settings";
import { MCPTaskAgentProfileDefaultSettings } from "@/components/settings/mcp-task-agent-profile-default-settings";
import { PreventAutoStartAgentSettings } from "@/components/settings/prevent-auto-start-agent-settings";
import { SettingsGroup } from "@/components/settings/settings-group";
import { SettingsTarget } from "@/components/settings/settings-target";
import { SleepInhibitionSettings } from "@/components/settings/sleep-inhibition-settings";
import { TodoListPanelSettings } from "@/components/settings/todo-list-panel-settings";
import { UnreadDividerSettings } from "@/components/settings/unread-divider-settings";
import {
  MessageQueueSettingsContent,
  useMessageQueueSettingsDraft,
} from "@/components/settings/system/message-queue-settings";
import { SessionCapacitySettingsContent } from "@/components/settings/system/session-capacity-settings";
import { useSessionCapacitySettings } from "@/components/settings/system/use-session-capacity-settings";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsPageHeader } from "./settings-typography";
import { SettingsTabs, SettingsTabsList, SettingsTabsPanel } from "./settings-tabs";
import { useSettingsTab } from "@/hooks/domains/settings/use-settings-tab";
import { useSettingsSaveCoordinator } from "./settings-save-provider";
import { UnsavedChangesBadge } from "./unsaved-indicator";
import {
  TASK_BEHAVIOR_TABS,
  TASK_BEHAVIOR_TARGET_TABS,
  taskBehaviorTab,
  firstNewAttentionTab,
} from "./task-behavior-tabs";

function useTabAttention(value: string, selectTab: (tab: string) => void) {
  const { contributorStates } = useSettingsSaveCoordinator();
  const previousAttention = useRef<string[]>([]);
  const attention = contributorStates.filter((state) => state.invalid || state.saveFailed);
  const attentionKey = attention
    .map((state) => `${state.id}:${state.invalid}:${state.saveFailed}`)
    .join("|");
  useEffect(() => {
    const tokens = attentionKey ? attentionKey.split("|") : [];
    const newTabs = tokens
      .filter((token) => !previousAttention.current.includes(token))
      .map((token) => taskBehaviorTab(token.split(":")[0]))
      .filter((tab): tab is NonNullable<typeof tab> => !!tab);
    previousAttention.current = tokens;
    const next = firstNewAttentionTab(newTabs, []);
    if (next && next !== value) selectTab(next);
  }, [attentionKey, selectTab, value]);
  return { contributorStates, attention };
}

export function TaskBehaviorSettings() {
  const { t } = useTranslation();
  const queueState = useMessageQueueSettingsDraft();
  const sessionState = useSessionCapacitySettings();
  const [sleepAttention, setSleepAttention] = useState(false);
  const { value, selectTab } = useSettingsTab({
    tabs: TASK_BEHAVIOR_TABS,
    defaultTab: "tasks",
    targetToTab: TASK_BEHAVIOR_TARGET_TABS,
  });
  const { contributorStates, attention } = useTabAttention(value, selectTab);
  const runtimeLoadError = queueState.loadFailed || sessionState.loadFailed || sleepAttention;
  const tabLabels = {
    tasks: t("settings:taskBehaviorTabTasks"),
    conversation: t("settings:taskBehaviorTabConversation"),
    runtime: t("settings:taskBehaviorTabRuntime"),
  };
  const tabs = TASK_BEHAVIOR_TABS.map((id) => ({
    id,
    ariaLabel: tabLabels[id],
    label: (
      <span className="flex items-center gap-2">
        {tabLabels[id]}
        {contributorStates.some((state) => state.isDirty && taskBehaviorTab(state.id) === id) && (
          <UnsavedChangesBadge />
        )}
        {(attention.some((state) => taskBehaviorTab(state.id) === id) ||
          (id === "runtime" && runtimeLoadError)) && (
          <span role="status" className="text-xs text-destructive">
            {t("settings:settingsNeedAttention")}
          </span>
        )}
      </span>
    ),
  }));
  return (
    <div className="space-y-6" data-testid="task-behavior-settings">
      <SettingsTabs tabs={tabs} value={value} onValueChange={selectTab}>
        <SettingsPageHeader
          title={t("settings:taskBehavior")}
          tabs={<SettingsTabsList ariaLabel={t("settings:taskBehavior")} />}
        />
        <SettingsTabsPanel value="tasks" className="space-y-6 pt-4">
          <SettingsGroup
            title={t("settings:taskBehaviorCreating")}
            titleTestId="task-behavior-creating-title"
            data-testid="task-behavior-group"
          >
            <CreationAutoFocusSettings presentation="row" />
            <AgentGeneratedTaskTitleSettings presentation="row" />
            <MCPTaskAgentProfileDefaultSettings presentation="row" />
            <PreventAutoStartAgentSettings presentation="row" />
          </SettingsGroup>
          <SettingsGroup
            title={t("settings:taskBehaviorArchiving")}
            titleTestId="task-behavior-archiving-title"
            data-testid="task-behavior-group"
          >
            <ArchiveConfirmationSettings presentation="row" />
          </SettingsGroup>
        </SettingsTabsPanel>
        <SettingsTabsPanel value="conversation" className="pt-4">
          <SettingsGroup
            title={t("settings:taskBehaviorConversation")}
            titleTestId="task-behavior-conversation-title"
            data-testid="task-behavior-group"
          >
            <UnreadDividerSettings presentation="row" />
            <AnchoredPromptBarSettings presentation="row" />
            <TodoListPanelSettings presentation="row" />
          </SettingsGroup>
        </SettingsTabsPanel>
        <SettingsTabsPanel value="runtime" className="pt-4">
          <SettingsGroup
            title={t("settings:taskBehaviorRuntime")}
            description={t("settings:taskBehaviorRuntimeScopeShort")}
            titleTestId="task-behavior-runtime-title"
            isDirty={queueState.isDirty || sessionState.isDirty}
            data-testid="task-behavior-runtime"
          >
            <SettingsTarget targetId={GENERAL_SETTINGS_TARGETS.sessionCapacity}>
              <SessionCapacitySettingsContent state={sessionState} withinGroup />
            </SettingsTarget>
            <SettingsTarget targetId={GENERAL_SETTINGS_TARGETS.messageQueue}>
              <MessageQueueSettingsContent state={queueState} withinGroup />
            </SettingsTarget>
            <SleepInhibitionSettings withinGroup onAttentionChange={setSleepAttention} />
          </SettingsGroup>
        </SettingsTabsPanel>
      </SettingsTabs>
    </div>
  );
}
