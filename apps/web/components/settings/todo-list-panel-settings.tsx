"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";

/**
 * Edits the per-user preference that controls whether the agent's live todo
 * checklist is available as a Todos tab in the desktop task workbench's
 * right panel. It participates in the shared Settings save/discard lifecycle
 * and commits the persisted preference to the app store on save.
 */
export function TodoListPanelSettings() {
  const { t } = useTranslation();
  const showTodoListPanel = useAppStore((state) => state.userSettings.showTodoListPanel);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(showTodoListPanel);
  const [draft, setDraft] = useState(showTodoListPanel);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(showTodoListPanel);
      return showTodoListPanel;
    });
  }, [showTodoListPanel]);

  useSettingsSaveContributor({
    id: "general-todo-list-panel",
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(revision);
      await updateUserSettings({ show_todo_list_panel: submitted });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, showTodoListPanel: submitted });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard isDirty={isDirty} data-testid="todo-list-panel-settings-card">
      <CardHeader>
        <CardTitle className="text-base">{t("settings:todoListPanel")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="show-todo-list-panel">{t("settings:showAgentTodoListPanel")}</Label>
            <p className="text-xs text-muted-foreground">
              {t("settings:pinTheAgentsLiveTodoChecklistAs")}
            </p>
          </div>
          <Switch
            id="show-todo-list-panel"
            checked={draft}
            data-settings-dirty={isDirty}
            onCheckedChange={setDraft}
            className="shrink-0 cursor-pointer"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
