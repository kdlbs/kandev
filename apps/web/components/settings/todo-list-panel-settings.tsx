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

type TodoListPanelDraft = {
  show: boolean;
  onlyWhenNotEmpty: boolean;
};

/**
 * Edits the per-user preferences that control whether the agent's live todo
 * checklist is auto-pinned as a Todos tab in the desktop task workbench's
 * right panel. The main preference is the master gate; the "only when not
 * empty" sub-option is inhibited (hidden, never disabled) while the master
 * gate is off, so its saved value survives and reappears once the master gate
 * is re-enabled. Manual adds from the workbench's "+" menu are never gated by
 * either preference.
 */
export function TodoListPanelSettings() {
  const { t } = useTranslation();
  const showTodoListPanel = useAppStore((state) => state.userSettings.showTodoListPanel);
  const onlyPinWhenNotEmpty = useAppStore(
    (state) => state.userSettings.showTodoListPanelOnlyWhenNotEmpty,
  );
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState<TodoListPanelDraft>({
    show: showTodoListPanel,
    onlyWhenNotEmpty: onlyPinWhenNotEmpty,
  });
  const [draft, setDraft] = useState<TodoListPanelDraft>(saved);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft.show !== saved.show || draft.onlyWhenNotEmpty !== saved.onlyWhenNotEmpty;

  useEffect(() => {
    setSaved((previous) => {
      if (
        draftRef.current.show === previous.show &&
        draftRef.current.onlyWhenNotEmpty === previous.onlyWhenNotEmpty
      ) {
        setDraft({ show: showTodoListPanel, onlyWhenNotEmpty: onlyPinWhenNotEmpty });
      }
      return { show: showTodoListPanel, onlyWhenNotEmpty: onlyPinWhenNotEmpty };
    });
  }, [showTodoListPanel, onlyPinWhenNotEmpty]);

  useSettingsSaveContributor({
    id: "general-todo-list-panel",
    revision: JSON.stringify(draft),
    isDirty,
    save: async (revision) => {
      const submitted = JSON.parse(String(revision)) as TodoListPanelDraft;
      const before = storeApi.getState().userSettings;
      await updateUserSettings({
        show_todo_list_panel: submitted.show,
        show_todo_list_panel_only_when_not_empty: submitted.onlyWhenNotEmpty,
      });
      // Only adopt our submission as the saved baseline when nothing newer
      // landed mid-flight (e.g. a WS settings push from another tab): the
      // hydration effect has already moved `saved` to the newer values, so
      // overwriting them here would leave the UI falsely clean against a
      // stale submission. On drift, keep the newer baseline so the draft
      // stays dirty and the user can reconcile.
      const current = storeApi.getState().userSettings;
      const drifted =
        current.showTodoListPanel !== before.showTodoListPanel ||
        current.showTodoListPanelOnlyWhenNotEmpty !== before.showTodoListPanelOnlyWhenNotEmpty;
      if (drifted) return;
      setSaved(submitted);
      setUserSettings({
        ...current,
        showTodoListPanel: submitted.show,
        showTodoListPanelOnlyWhenNotEmpty: submitted.onlyWhenNotEmpty,
      });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard isDirty={isDirty} data-testid="todo-list-panel-settings-card">
      <CardHeader>
        <CardTitle className="text-base">{t("settings:todoListPanel")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div className="flex min-h-11 items-center justify-between gap-4">
            <div className="min-w-0 space-y-0.5">
              <Label htmlFor="show-todo-list-panel">{t("settings:showAgentTodoListPanel")}</Label>
              <p className="text-xs text-muted-foreground">
                {t("settings:pinTheAgentsLiveTodoChecklistAs")}
              </p>
            </div>
            <Switch
              id="show-todo-list-panel"
              checked={draft.show}
              data-settings-dirty={draft.show !== saved.show}
              onCheckedChange={(show) => setDraft((current) => ({ ...current, show }))}
              className="shrink-0 cursor-pointer"
            />
          </div>
          {draft.show && (
            <div className="flex min-h-11 items-center justify-between gap-4 border-t pt-4">
              <div className="min-w-0 space-y-0.5">
                <Label htmlFor="todo-list-panel-only-when-not-empty">
                  {t("settings:onlyPinWhenTodoListIsNotEmpty")}
                </Label>
                <p className="text-xs text-muted-foreground">
                  {t("settings:onlyPinWhenTodoListIsNotEmptyDescription")}
                </p>
              </div>
              <Switch
                id="todo-list-panel-only-when-not-empty"
                checked={draft.onlyWhenNotEmpty}
                data-settings-dirty={draft.onlyWhenNotEmpty !== saved.onlyWhenNotEmpty}
                onCheckedChange={(onlyWhenNotEmpty) =>
                  setDraft((current) => ({ ...current, onlyWhenNotEmpty }))
                }
                className="shrink-0 cursor-pointer"
              />
            </div>
          )}
        </div>
      </CardContent>
    </SettingsCard>
  );
}
