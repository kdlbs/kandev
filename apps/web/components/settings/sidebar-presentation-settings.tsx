"use client";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { mapLatestUserSettingsResponse } from "@/lib/ssr/user-settings";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsGroup, SettingsRow } from "./settings-group";
import { useSettingsSaveContributor } from "./settings-save-provider";
import {
  presentationPatch,
  rebasePresentation,
  type SidebarPresentation,
} from "./sidebar-presentation-state";

export function SidebarPresentationSettings() {
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const fast = useAppStore((s) => s.userSettings.sidebarFastActionsEnabled);
  const style = useAppStore((s) => s.userSettings.sidebarNewTaskStyle);
  const saved = { fast, style };
  const [state, setState] = useState(() => ({ baseline: saved, draft: saved }));
  const latest = useRef(state);
  latest.current = state;
  useEffect(() => {
    setState((current) => ({
      baseline: { fast, style },
      draft: rebasePresentation(current.draft, current.baseline, { fast, style }),
    }));
  }, [fast, style]);
  const dirty = Object.keys(presentationPatch(state.draft, saved)).length > 0;
  useSettingsSaveContributor({
    id: "sidebar-presentation",
    order: 10,
    revision: JSON.stringify(state.draft),
    isDirty: dirty,
    save: async () => {
      const submitted = latest.current.draft;
      const settings = store.getState().userSettings;
      const response = await updateUserSettings(
        presentationPatch(submitted, {
          fast: settings.sidebarFastActionsEnabled,
          style: settings.sidebarNewTaskStyle,
        }),
      );
      const next = mapLatestUserSettingsResponse(response, store.getState().userSettings);
      store.getState().setUserSettings(next);
      const accepted = { fast: next.sidebarFastActionsEnabled, style: next.sidebarNewTaskStyle };
      setState((current) => ({
        baseline: accepted,
        draft: rebasePresentation(current.draft, submitted, accepted),
      }));
    },
    discard: () => {
      const settings = store.getState().userSettings;
      const next = {
        fast: settings.sidebarFastActionsEnabled,
        style: settings.sidebarNewTaskStyle,
      };
      setState({ baseline: next, draft: next });
    },
  });
  const update = (patch: Partial<SidebarPresentation>) =>
    setState((current) => ({ ...current, draft: { ...current.draft, ...patch } }));
  return (
    <SettingsGroup title={t("settings:sidebarPresentation")} isDirty={dirty}>
      <SettingsRow
        label={t("settings:sidebarFastActions")}
        description={t("settings:sidebarFastActionsDescription")}
        discoveryTargetId={GENERAL_SETTINGS_TARGETS.sidebarFastActions}
        controlId="sidebar-fast-actions"
        isDirty={state.draft.fast !== fast}
        control={
          <Switch
            id="sidebar-fast-actions"
            data-testid="sidebar-fast-actions-setting"
            checked={state.draft.fast}
            onCheckedChange={(value) => update({ fast: value })}
          />
        }
      />
      <SettingsRow
        label={t("settings:sidebarNewTaskStyle")}
        discoveryTargetId={GENERAL_SETTINGS_TARGETS.sidebarNewTaskStyle}
        controlId="sidebar-new-task-style"
        isDirty={state.draft.style !== style}
        control={
          <Select
            value={state.draft.style}
            onValueChange={(value) => update({ style: value as SidebarPresentation["style"] })}
          >
            <SelectTrigger id="sidebar-new-task-style" data-testid="sidebar-new-task-style-setting">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="simple">{t("settings:sidebarStyleSimple")}</SelectItem>
              <SelectItem value="compact">{t("settings:sidebarStyleCompact")}</SelectItem>
            </SelectContent>
          </Select>
        }
      />
    </SettingsGroup>
  );
}
