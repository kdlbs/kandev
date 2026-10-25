import type { SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import type { Theme } from "@/lib/settings/types";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import type { UserSettingsUpdatePayload } from "@/lib/types/http";

export type AppearanceState = {
  theme: Theme;
  settingsMenuMode: SettingsMenuMode;
  richOutputAnimationsEnabled: boolean;
  chatAnimationsEnabled: boolean;
  startupPage: UserSettingsState["startupPage"];
  changesPanelLayout: UserSettingsState["changesPanelLayout"];
  appStatusBarEnabled: boolean;
  sidebarHoverEnabled: boolean;
  sidebarHoverDelayMs: string;
  showMetrics: boolean;
  simplifiedMetrics: boolean;
};

export function createAppearanceSavedState(
  theme: Theme,
  settingsMenuMode: SettingsMenuMode,
  richOutputAnimationsEnabled: boolean,
  userSettings: Pick<
    UserSettingsState,
    | "appStatusBarEnabled"
    | "changesPanelLayout"
    | "startupPage"
    | "systemMetricsDisplay"
    | "sidebarHoverEnabled"
    | "sidebarHoverDelayMs"
  >,
  chatAnimationsEnabled = true,
): AppearanceState {
  return {
    theme,
    sidebarHoverEnabled: userSettings.sidebarHoverEnabled,
    sidebarHoverDelayMs: String(userSettings.sidebarHoverDelayMs),
    appStatusBarEnabled: userSettings.appStatusBarEnabled,
    // Per-device, but drafted and saved with account settings under one control.
    settingsMenuMode,
    richOutputAnimationsEnabled,
    chatAnimationsEnabled,
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
  const delay = parseSidebarHoverDelay(submitted.sidebarHoverDelayMs);
  // i18n-exempt: unreachable invariant guard; the contributor presents localized validation.
  if (delay === null) throw new Error("Invalid sidebar hover delay");
  const patch: UserSettingsUpdatePayload = {};
  if (submitted.sidebarHoverEnabled !== saved.sidebarHoverEnabled) {
    patch.sidebar_hover_enabled = submitted.sidebarHoverEnabled;
  }
  if (delay !== Number(saved.sidebarHoverDelayMs)) {
    patch.sidebar_hover_delay_ms = delay;
  }
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
    sidebarHoverEnabled: rebase("sidebarHoverEnabled"),
    sidebarHoverDelayMs: rebase("sidebarHoverDelayMs"),
    settingsMenuMode: rebase("settingsMenuMode"),
    richOutputAnimationsEnabled: rebase("richOutputAnimationsEnabled"),
    chatAnimationsEnabled: rebase("chatAnimationsEnabled"),
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
    state.sidebarHoverEnabled,
    state.sidebarHoverDelayMs,
    state.settingsMenuMode,
    state.richOutputAnimationsEnabled,
    state.chatAnimationsEnabled,
    state.startupPage,
    state.changesPanelLayout,
    state.appStatusBarEnabled,
    state.showMetrics,
    state.simplifiedMetrics,
  ]);
}

export function parseSidebarHoverDelay(value: string): number | null {
  if (!/^\d+$/.test(value)) return null;
  const delay = Number(value);
  return Number.isInteger(delay) && delay >= 0 && delay <= 5000 ? delay : null;
}
