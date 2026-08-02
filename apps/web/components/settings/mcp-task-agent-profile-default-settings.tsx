"use client";

import { useEffect, useRef, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { MCPTaskAgentProfileDefault } from "@/lib/types/http";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { Trans, useTranslation } from "react-i18next";

/**
 * `value` is the persisted enum the API compares — never translated. The label
 * and description hold catalog KEYS, resolved at render: a module-scope `t()`
 * would freeze this copy at the boot locale (see docs/i18n.md).
 */
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
    <CardDescription className="space-y-3">
      <p>{t("settings:useThisSettingWhenAnAgent")}</p>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1 text-foreground">
          <span className="font-medium">{t("settings:affectedKandevMcpTool")}</span>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={t("settings:aboutAffectedKandevMcpTools")}
                className="relative inline-flex size-6 shrink-0 cursor-pointer items-center justify-center text-muted-foreground outline-none after:absolute after:-inset-2.5 hover:text-foreground focus-visible:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
              >
                <IconInfoCircle className="size-4" aria-hidden="true" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="top" className="max-w-xs text-xs">
              <Trans i18nKey="settings:mcpAffectedToolsHelp">
                <code>create_task_kandev</code> creates a separate task.{" "}
                <code>spawn_session_kandev</code> adds a session to the current task, so it does not
                use this preference.
              </Trans>
            </TooltipContent>
          </Tooltip>
        </div>
        <p>
          <Trans i18nKey="settings:mcpCreateTaskScope">
            <code className="rounded-sm bg-muted px-1 py-0.5 text-foreground">
              create_task_kandev
            </code>{" "}
            creates new tasks and subtasks. This setting applies only when the call omits{" "}
            <code className="rounded-sm bg-muted px-1 py-0.5 text-foreground">
              agent_profile_id
            </code>
            .
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
      </div>
    </CardDescription>
  );
}

export function MCPTaskAgentProfileDefaultSettings() {
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

  return (
    <SettingsCard isDirty={isDirty} data-testid="mcp-task-profile-default-card">
      <CardHeader>
        <CardTitle className="text-base">
          <h3>{t("settings:profileForTasksCreatedByAgents")}</h3>
        </CardTitle>
        <MCPTaskProfileScopeDescription />
      </CardHeader>
      <CardContent>
        <RadioGroup
          aria-label={t("settings:profileForTasksCreatedByAgents")}
          value={draft}
          onValueChange={(value) => setDraft(value as MCPTaskAgentProfileDefault)}
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
                    {t(option.descriptionKey)}
                  </span>
                </span>
              </Label>
            );
          })}
        </RadioGroup>
      </CardContent>
    </SettingsCard>
  );
}
