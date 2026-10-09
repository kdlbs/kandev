"use client";

import { useEffect, useRef, useState } from "react";
import { SettingsInfo } from "./settings-info";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { MCPTaskAgentProfileDefault } from "@/lib/types/http";
import { SettingsCard } from "./settings-card";
import { SettingsRow, type SettingsPresentation } from "./settings-group";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { Trans, useTranslation } from "react-i18next";

/**
 * `value` is the persisted enum the API compares — never translated. The label
 * and description hold catalog KEYS, resolved at render: a module-scope `t()`
 * would freeze this copy at the boot locale (see docs/i18n.md).
 */
const PROFILE_LABEL_KEY = "settings:profileForTasksCreatedByAgents";

const OPTIONS: Array<{
  value: MCPTaskAgentProfileDefault;
  labelKey: string;
  descriptionKey: string;
}> = [
  {
    value: "current_task",
    labelKey: "settings:mcpTaskProfileCurrentTask",
    descriptionKey: "settings:mcpTaskProfileCurrentTaskDescription",
  },
  {
    value: "workspace_default",
    labelKey: "settings:mcpTaskProfileWorkspaceDefault",
    descriptionKey: "settings:mcpTaskProfileWorkspaceDefaultDescription",
  },
];

function MCPTaskProfileScopeDescription() {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <p>{t("settings:useThisSettingWhenAnAgent")}</p>
      <p>
        <Trans i18nKey="settings:mcpCreateTaskScope">
          <code className="rounded-sm bg-muted px-1 py-0.5 text-foreground">
            create_task_kandev
          </code>{" "}
          creates new tasks and subtasks. This setting applies only when the call omits{" "}
          <code className="rounded-sm bg-muted px-1 py-0.5 text-foreground">agent_profile_id</code>.
        </Trans>
      </p>
      <p>
        <Trans i18nKey="settings:mcpSpawnSessionScope">
          <code className="rounded-sm bg-muted px-1 py-0.5 text-foreground">
            spawn_session_kandev
          </code>{" "}
          and tasks you create yourself are not affected. An explicitly selected profile always
          wins.
        </Trans>
      </p>
      <p>
        <Trans i18nKey="settings:mcpAffectedToolsHelp">
          <code>create_task_kandev</code> creates a separate task. <code>spawn_session_kandev</code>{" "}
          adds a session to the current task, so it does not use this preference.
        </Trans>
      </p>
      {OPTIONS.map((option) => (
        <p key={option.value}>
          <strong>{t(option.labelKey)}: </strong>
          {t(option.descriptionKey)}
        </p>
      ))}
    </div>
  );
}

function MCPTaskProfileRadioGroup({
  value,
  isDirty,
  onValueChange,
  ariaDescribedBy,
}: {
  value: MCPTaskAgentProfileDefault;
  isDirty: boolean;
  onValueChange: (value: MCPTaskAgentProfileDefault) => void;
  ariaDescribedBy?: string;
}) {
  const { t } = useTranslation();
  return (
    <RadioGroup
      aria-label={t(PROFILE_LABEL_KEY)}
      aria-describedby={ariaDescribedBy}
      value={value}
      onValueChange={(nextValue) => onValueChange(nextValue as MCPTaskAgentProfileDefault)}
      data-settings-dirty={isDirty}
      className="gap-3"
    >
      {OPTIONS.map((option) => {
        const labelId = `mcp-task-profile-${option.value}-label`;
        const descriptionId = `mcp-task-profile-${option.value}-description`;
        return (
          <Label
            key={option.value}
            htmlFor={`mcp-task-profile-${option.value}`}
            className="flex min-h-11 w-full min-w-0 cursor-pointer items-start gap-3 rounded-md border p-3 hover:bg-muted/30"
          >
            <RadioGroupItem
              id={`mcp-task-profile-${option.value}`}
              value={option.value}
              aria-labelledby={labelId}
              aria-describedby={descriptionId}
              className="mt-0.5"
            />
            <span className="min-w-0 space-y-1">
              <span id={labelId} className="block text-sm font-medium">
                {t(option.labelKey)}
              </span>
              <span
                id={descriptionId}
                className="block whitespace-normal break-words text-xs text-muted-foreground"
              >
                {t(
                  option.value === "current_task"
                    ? "settings:profileSessionShort"
                    : "settings:profileWorkspaceShort",
                )}
              </span>
            </span>
          </Label>
        );
      })}
    </RadioGroup>
  );
}

export function MCPTaskAgentProfileDefaultSettings({
  presentation = "card",
}: {
  presentation?: SettingsPresentation;
}) {
  const { t } = useTranslation();
  const preference = useAppStore((state) => state.userSettings.mcpTaskAgentProfileDefault);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(preference);
  const [draft, setDraft] = useState(preference);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(preference);
      return preference;
    });
  }, [preference]);

  useSettingsSaveContributor({
    id: "general-mcp-task-agent-profile-default",
    order: 10,
    revision: draft,
    isDirty,
    save: async (revision) => {
      const submitted = revision as MCPTaskAgentProfileDefault;
      await updateUserSettings({ mcp_task_agent_profile_default: submitted });
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        mcpTaskAgentProfileDefault: submitted,
      });
    },
    discard: () => setDraft(saved),
  });

  if (presentation === "row") {
    return (
      <SettingsRow
        label={t(PROFILE_LABEL_KEY)}
        description={t("settings:agentProfileShort")}
        info={
          <SettingsInfo label={t(PROFILE_LABEL_KEY)}>
            <MCPTaskProfileScopeDescription />
          </SettingsInfo>
        }
        descriptionId="mcp-task-profile-description"
        discoveryTargetId={GENERAL_SETTINGS_TARGETS.agentTaskProfile}
        isDirty={isDirty}
        control={
          <MCPTaskProfileRadioGroup
            value={draft}
            isDirty={isDirty}
            onValueChange={setDraft}
            ariaDescribedBy="mcp-task-profile-description"
          />
        }
      />
    );
  }

  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.agentTaskProfile}
      data-testid="mcp-task-profile-default-card"
    >
      <CardHeader>
        <CardTitle className="text-base">
          <h3>{t(PROFILE_LABEL_KEY)}</h3>
        </CardTitle>
        <SettingsInfo label={t(PROFILE_LABEL_KEY)}>
          <MCPTaskProfileScopeDescription />
        </SettingsInfo>
      </CardHeader>
      <CardContent>
        <MCPTaskProfileRadioGroup value={draft} isDirty={isDirty} onValueChange={setDraft} />
      </CardContent>
    </SettingsCard>
  );
}
