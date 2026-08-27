import type { SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import type { Theme } from "@/lib/settings/types";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import type { UserSettingsUpdatePayload } from "@/lib/types/http";

export type AppearanceState = {
  theme: Theme;
  settingsMenuMode: SettingsMenuMode;
  richOutputAnimationsEnabled: boolean;
  startupPage: UserSettingsState["startupPage"];
  changesPanelLayout: UserSettingsState["changesPanelLayout"];
  appStatusBarEnabled: boolean;
  showMetrics: boolean;
  simplifiedMetrics: boolean;
};

export function createAppearanceSavedState(
  theme: Theme,
  settingsMenuMode: SettingsMenuMode,
  richOutputAnimationsEnabled: boolean,
  userSettings: Pick<
    UserSettingsState,
    "appStatusBarEnabled" | "changesPanelLayout" | "startupPage" | "systemMetricsDisplay"
  >,
): AppearanceState {
  return {
    theme,
    appStatusBarEnabled: userSettings.appStatusBarEnabled,
    // Per-device, but drafted and saved with account settings under one control.
    settingsMenuMode,
    richOutputAnimationsEnabled,
    changesPanelLayout: userSettings.changesPanelLayout,
    startupPage: userSettings.startupPage,
    showMetrics: userSettings.systemMetricsDisplay.showInTopbar,
    simplifiedMetrics: userSettings.systemMetricsDisplay.simplified,
  };
}

export function buildAppearanceUserSettingsPatch(
  submitted: AppearanceState,
  saved: AppearanceState,
): UserSettingsUpdatePayload {
  const patch: UserSettingsUpdatePayload = {};
  if (submitted.startupPage !== saved.startupPage) {
    patch.startup_page = submitted.startupPage;
  }
  if (submitted.changesPanelLayout !== saved.changesPanelLayout) {
    patch.changes_panel_layout = submitted.changesPanelLayout;
  }
  if (submitted.appStatusBarEnabled !== saved.appStatusBarEnabled) {
    patch.app_status_bar_enabled = submitted.appStatusBarEnabled;
  }
  const metrics: NonNullable<UserSettingsUpdatePayload["system_metrics_display"]> = {};
  if (submitted.showMetrics !== saved.showMetrics) {
    metrics.show_in_topbar = submitted.showMetrics;
  }
  if (submitted.simplifiedMetrics !== saved.simplifiedMetrics) {
    metrics.simplified = submitted.simplifiedMetrics;
  }
  if (Object.keys(metrics).length > 0) {
    patch.system_metrics_display = metrics;
  }
  return patch;
}

export function rebaseAppearanceDraft(
  draft: AppearanceState,
  baseline: AppearanceState,
  nextSaved: AppearanceState,
  preserveDraftFields: ReadonlySet<keyof AppearanceState> = new Set(),
): AppearanceState {
  const rebase = <Field extends keyof AppearanceState>(field: Field): AppearanceState[Field] =>
    preserveDraftFields.has(field) || draft[field] !== baseline[field]
      ? draft[field]
      : nextSaved[field];
  return {
    theme: rebase("theme"),
    settingsMenuMode: rebase("settingsMenuMode"),
    richOutputAnimationsEnabled: rebase("richOutputAnimationsEnabled"),
    startupPage: rebase("startupPage"),
    changesPanelLayout: rebase("changesPanelLayout"),
    appStatusBarEnabled: rebase("appStatusBarEnabled"),
    showMetrics: rebase("showMetrics"),
    simplifiedMetrics: rebase("simplifiedMetrics"),
  };
}

export function appearanceRevision(state: AppearanceState): string {
  return JSON.stringify([
    state.theme,
    state.settingsMenuMode,
    state.richOutputAnimationsEnabled,
    state.startupPage,
    state.changesPanelLayout,
    state.appStatusBarEnabled,
    state.showMetrics,
    state.simplifiedMetrics,
  ]);
}
