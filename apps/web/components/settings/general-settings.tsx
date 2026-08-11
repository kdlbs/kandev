"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useTheme } from "@/components/theme/app-theme";
import {
  IconActivity,
  IconBinaryTree,
  IconCommand,
  IconPalette,
  IconKeyboard,
  IconGitBranch,
  IconArchive,
  IconArrowBackUp,
  IconListCheck,
  IconHome,
} from "@tabler/icons-react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { KeyboardShortcutsCard } from "@/components/settings/keyboard-shortcuts-card";
import { SystemMetricsSettingsCard } from "@/components/settings/system-metrics-settings-card";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { Theme } from "@/lib/settings/types";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { ArchiveConfirmationSettings } from "@/components/settings/archive-confirmation-settings";
import { LanguageSettings } from "@/components/settings/language-settings";
import { MCPTaskAgentProfileDefaultSettings } from "@/components/settings/mcp-task-agent-profile-default-settings";
import { UnreadDividerSettings } from "@/components/settings/unread-divider-settings";
import { AgentGeneratedTaskTitleSettings } from "@/components/settings/agent-generated-task-title-settings";
import { AnchoredPromptBarSettings } from "@/components/settings/anchored-prompt-bar-settings";
import { TodoListPanelSettings } from "@/components/settings/todo-list-panel-settings";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import { buildPluginShortcutEntries } from "@/lib/keyboard/plugin-shortcuts";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { StartupPageSettingsCard } from "@/components/settings/startup-page-settings-card";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SleepInhibitionSettings } from "@/components/settings/sleep-inhibition-settings";
import { AppStatusBarSettingsCard } from "@/components/settings/app-status-bar-settings-card";
import { SettingsMenuModeCard } from "@/components/settings/settings-menu-mode-card";
import type { SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import {
  appearanceRevision,
  buildAppearanceUserSettingsPatch,
  createAppearanceSavedState,
  rebaseAppearanceDraft,
  type AppearanceState,
} from "@/components/settings/appearance-settings-state";

function ThemeSettingsCard({
  theme,
  isDirty,
  onChange,
}: {
  theme: Theme;
  isDirty: boolean;
  onChange: (theme: Theme) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.colorTheme}
      data-testid="theme-settings-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:colorTheme")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Select value={theme} onValueChange={(value) => onChange(value as Theme)}>
            <SelectTrigger id="theme" data-settings-dirty={isDirty}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="system">{t("common:system")}</SelectItem>
              <SelectItem value="light">{t("settings:light")}</SelectItem>
              <SelectItem value="dark">{t("settings:dark")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </CardContent>
    </SettingsCard>
  );
}

function ChatSubmitKeyCard({
  value,
  isDirty,
  onChange,
}: {
  value: "enter" | "cmd_enter";
  isDirty: boolean;
  onChange: (value: "enter" | "cmd_enter") => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.submitShortcut}
      data-testid="chat-submit-key-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:submitShortcut")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="chat-submit-key">{t("settings:messageSubmitKey")}</Label>
          <Select value={value} onValueChange={(next) => onChange(next as "enter" | "cmd_enter")}>
            <SelectTrigger id="chat-submit-key" data-settings-dirty={isDirty}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="cmd_enter">{t("settings:cmdCtrlEnterToSend")}</SelectItem>
              <SelectItem value="enter">{t("settings:enterToSend")}</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {value === "cmd_enter"
              ? t("settings:pressCmdCtrlEnterToSend")
              : t("settings:pressEnterToSendMessagesPress")}
          </p>
        </div>
      </CardContent>
    </SettingsCard>
  );
}

function ChangesPanelLayoutCard({
  value,
  isDirty,
  onChange,
}: {
  value: "flat" | "tree";
  isDirty: boolean;
  onChange: (value: "flat" | "tree") => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.changesPanelLayout}
      data-testid="changes-panel-layout-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:changesPanelLayout")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="changes-panel-layout">{t("settings:fileListView")}</Label>
          <Select value={value} onValueChange={(next) => onChange(next as "flat" | "tree")}>
            <SelectTrigger
              id="changes-panel-layout"
              data-testid="changes-panel-layout-select"
              data-settings-dirty={isDirty}
              className="cursor-pointer"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="flat">{t("settings:flatList")}</SelectItem>
              <SelectItem value="tree">{t("settings:tree")}</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {t("settings:displayChangedFilesAsAFlat")}
          </p>
        </div>
      </CardContent>
    </SettingsCard>
  );
}

function StartupPageSettingsSection({
  value,
  isDirty,
  onChange,
}: {
  value: UserSettingsState["startupPage"];
  isDirty: boolean;
  onChange: (startupPage: UserSettingsState["startupPage"]) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Separator />

      <SettingsSection
        icon={<IconHome className="h-5 w-5" />}
        title={t("settings:startupPage")}
        description={t("settings:chooseWhatOpensWhenKandevStarts")}
      >
        <StartupPageSettingsCard value={value} isDirty={isDirty} onChange={onChange} />
      </SettingsSection>

      <Separator />
    </>
  );
}

function SettingsMenuModeSection({
  value,
  isDirty,
  onChange,
}: {
  value: SettingsMenuMode;
  isDirty: boolean;
  onChange: (mode: SettingsMenuMode) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Separator />

      <SettingsSection
        icon={<IconBinaryTree className="h-5 w-5" />}
        title={t("settings:settingsMenu")}
        description={t("settings:chooseHowDeepTheSettingsMenuGoes")}
      >
        <SettingsMenuModeCard value={value} isDirty={isDirty} onChange={onChange} />
      </SettingsSection>
    </>
  );
}

function AppearanceThemeSection({
  theme,
  isDirty,
  onChange,
}: {
  theme: Theme;
  isDirty: boolean;
  onChange: (theme: Theme) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsSection
      icon={<IconPalette className="h-5 w-5" />}
      title={t("settings:appearance")}
      description={t("settings:customizeHowTheApplicationLooks")}
    >
      <ThemeSettingsCard theme={theme} isDirty={isDirty} onChange={onChange} />
    </SettingsSection>
  );
}

function AppStatusBarSettingsSection({
  enabled,
  isDirty,
  onChange,
}: {
  enabled: boolean;
  isDirty: boolean;
  onChange: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Separator />
      <SettingsSection
        icon={<IconActivity className="h-5 w-5" />}
        title={t("settings:statusBar")}
        description={t("settings:configureStatusBarVisibility")}
      >
        <AppStatusBarSettingsCard enabled={enabled} isDirty={isDirty} onChange={onChange} />
      </SettingsSection>
    </>
  );
}

export function TaskActionsSettings() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <SettingsSection
        icon={<IconArchive className="h-5 w-5" />}
        title={t("settings:taskActions")}
        description={t("settings:configureArchiveSafeguardsAndDefaultsFor")}
      >
        <div className="space-y-4">
          <MCPTaskAgentProfileDefaultSettings />
          <AgentGeneratedTaskTitleSettings />
          <ArchiveConfirmationSettings />
          <UnreadDividerSettings />
          <SleepInhibitionSettings />
        </div>
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconArrowBackUp className="h-5 w-5" />}
        title={t("settings:transcriptNavigation")}
        description={t("settings:chooseWhichTranscriptNavigationControls")}
      >
        <AnchoredPromptBarSettings />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconListCheck className="h-5 w-5" />}
        title={t("settings:todoListPanel")}
        description={t("settings:pinTheAgentsLiveTodoChecklistAs")}
      >
        <TodoListPanelSettings />
      </SettingsSection>
    </div>
  );
}

/**
 * Register the Appearance page with the shared save coordinator.
 *
 * Split out of `AppearanceSettings` for length. It is also where the page's two
 * kinds of setting meet: the account-level ones go out in one
 * `updateUserSettings` call, while the theme and the settings menu shape are
 * per-device and commit through their own preview/commit/restore pairs — one
 * Save changes control over both.
 */
function useAppearanceSaveContributor({
  draft,
  draftRef,
  editedDuringSaveRef,
  saved,
  setSaved,
  setDraft,
}: {
  draft: AppearanceState;
  draftRef: { current: AppearanceState };
  editedDuringSaveRef: { current: Set<keyof AppearanceState> | null };
  saved: AppearanceState;
  setSaved: (next: AppearanceState) => void;
  setDraft: (next: AppearanceState) => void;
}) {
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const { previewTheme, commitTheme, restoreTheme } = useTheme();
  const previewMenuMode = useAppStore((state) => state.previewSettingsMenuMode);
  const commitMenuMode = useAppStore((state) => state.commitSettingsMenuMode);
  const restoreMenuMode = useAppStore((state) => state.restoreSettingsMenuMode);
  const revision = appearanceRevision(draft);

  useSettingsSaveContributor({
    id: "general-appearance",
    order: 10,
    revision,
    isDirty: revision !== appearanceRevision(saved),
    save: async () => {
      const submitted = draft;
      const patch = buildAppearanceUserSettingsPatch(submitted, saved);
      const settingsAtSubmit = storeApi.getState().userSettings;
      const editedDuringSave = new Set<keyof AppearanceState>();
      editedDuringSaveRef.current = editedDuringSave;
      try {
        const response = await updateUserSettings(patch);
        commitTheme(submitted.theme);
        commitMenuMode(submitted.settingsMenuMode);
        // A draft edited while the save was in flight keeps its preview: what was
        // submitted is now persisted, but the screen should still show what the
        // user is currently looking at.
        if (draftRef.current.theme !== submitted.theme) {
          previewTheme(draftRef.current.theme);
        }
        if (draftRef.current.settingsMenuMode !== submitted.settingsMenuMode) {
          previewMenuMode(draftRef.current.settingsMenuMode);
        }
        const latestUserSettings = storeApi.getState().userSettings;
        const nextUserSettings =
          latestUserSettings === settingsAtSubmit
            ? mapUserSettingsResponse(response, latestUserSettings)
            : latestUserSettings;
        const confirmed = createAppearanceSavedState(
          submitted.theme,
          submitted.settingsMenuMode,
          nextUserSettings,
        );
        setSaved(confirmed);
        setDraft(rebaseAppearanceDraft(draftRef.current, submitted, confirmed, editedDuringSave));
        setUserSettings(nextUserSettings);
      } finally {
        if (editedDuringSaveRef.current === editedDuringSave) {
          editedDuringSaveRef.current = null;
        }
      }
    },
    discard: () => {
      setDraft(saved);
      restoreTheme();
      restoreMenuMode();
    },
  });
}

function useAppearanceDraftUpdater(
  setDraft: (update: (current: AppearanceState) => AppearanceState) => void,
  editedDuringSaveRef: { current: Set<keyof AppearanceState> | null },
) {
  return useCallback(
    (patch: Partial<AppearanceState>) =>
      setDraft((current) => {
        const next = { ...current, ...patch };
        for (const field of Object.keys(patch) as Array<keyof AppearanceState>) {
          if (current[field] !== next[field]) {
            editedDuringSaveRef.current?.add(field);
          }
        }
        return next;
      }),
    [editedDuringSaveRef, setDraft],
  );
}

export function AppearanceSettings() {
  const { t } = useTranslation();
  const userSettings = useAppStore((state) => state.userSettings);
  const { savedTheme, previewTheme } = useTheme();
  const savedMenuMode = useAppStore((state) => state.settingsMenu.savedMode);
  const previewMenuMode = useAppStore((state) => state.previewSettingsMenuMode);
  const [saved, setSaved] = useState(() =>
    createAppearanceSavedState(savedTheme, savedMenuMode, userSettings),
  );
  const [draft, setDraft] = useState(saved);
  const draftRef = useRef(draft);
  const savedRef = useRef(saved);
  const editedDuringSaveRef = useRef<Set<keyof AppearanceState> | null>(null);
  draftRef.current = draft;
  savedRef.current = saved;

  useEffect(() => {
    const previousSaved = savedRef.current;
    const nextSaved = createAppearanceSavedState(
      previousSaved.theme,
      previousSaved.settingsMenuMode,
      userSettings,
    );
    setDraft(
      rebaseAppearanceDraft(
        draftRef.current,
        previousSaved,
        nextSaved,
        editedDuringSaveRef.current ?? undefined,
      ),
    );
    setSaved(nextSaved);
  }, [userSettings]);

  useAppearanceSaveContributor({
    draft,
    draftRef,
    editedDuringSaveRef,
    saved,
    setSaved,
    setDraft,
  });

  const updateDraft = useAppearanceDraftUpdater(setDraft, editedDuringSaveRef);

  return (
    <div className="space-y-8">
      <AppearanceThemeSection
        theme={draft.theme}
        isDirty={draft.theme !== saved.theme}
        onChange={(theme) => {
          updateDraft({ theme });
          previewTheme(theme);
        }}
      />

      <SettingsMenuModeSection
        value={draft.settingsMenuMode}
        isDirty={draft.settingsMenuMode !== saved.settingsMenuMode}
        onChange={(settingsMenuMode) => {
          updateDraft({ settingsMenuMode });
          // The settings menu is on screen beside this page, so previewing is
          // the whole point: you see the shape you are choosing.
          previewMenuMode(settingsMenuMode);
        }}
      />

      <StartupPageSettingsSection
        value={draft.startupPage}
        isDirty={draft.startupPage !== saved.startupPage}
        onChange={(startupPage) => updateDraft({ startupPage })}
      />

      <LanguageSettings />

      <AppStatusBarSettingsSection
        enabled={draft.appStatusBarEnabled}
        isDirty={draft.appStatusBarEnabled !== saved.appStatusBarEnabled}
        onChange={(appStatusBarEnabled) => updateDraft({ appStatusBarEnabled })}
      />

      <Separator />

      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title={t("settings:changesPanel")}
        description={t("settings:customizeHowChangedFilesAreDisplayed")}
      >
        <ChangesPanelLayoutCard
          value={draft.changesPanelLayout}
          isDirty={draft.changesPanelLayout !== saved.changesPanelLayout}
          onChange={(changesPanelLayout) => updateDraft({ changesPanelLayout })}
        />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconActivity className="h-5 w-5" />}
        title={t("settings:resourceMetrics")}
        description={t("settings:configureBackendAndExecutionResourceSampling")}
      >
        <SystemMetricsSettingsCard
          showInTopbar={draft.showMetrics}
          isShowInTopbarDirty={draft.showMetrics !== saved.showMetrics}
          onShowInTopbarChange={(showMetrics) => updateDraft({ showMetrics })}
          simplified={draft.simplifiedMetrics}
          isSimplifiedDirty={draft.simplifiedMetrics !== saved.simplifiedMetrics}
          onSimplifiedChange={(simplifiedMetrics) => updateDraft({ simplifiedMetrics })}
        />
      </SettingsSection>
    </div>
  );
}

export function KeyboardShortcutsSettings() {
  const { t } = useTranslation();
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const { items: pluginItems } = usePlugins();
  const pluginShortcutEntries = buildPluginShortcutEntries(pluginItems);
  const [saved, setSaved] = useState(() => ({
    chatSubmitKey: userSettings.chatSubmitKey,
    keyboardShortcuts: userSettings.keyboardShortcuts as StoredShortcutOverrides,
  }));
  const [draft, setDraft] = useState(saved);
  const revision = JSON.stringify(draft);

  useSettingsSaveContributor({
    id: "general-keyboard-shortcuts",
    revision,
    isDirty: revision !== JSON.stringify(saved),
    save: async () => {
      const submitted = draft;
      const current = storeApi.getState().userSettings;
      await updateUserSettings({
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        chat_submit_key: submitted.chatSubmitKey,
        keyboard_shortcuts: submitted.keyboardShortcuts,
      });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, ...submitted });
    },
    discard: () => setDraft(saved),
  });

  return (
    <div className="space-y-8">
      <SettingsSection
        icon={<IconKeyboard className="h-5 w-5" />}
        title={t("settings:chatInput")}
        description={t("settings:configureChatInputBehavior")}
      >
        <ChatSubmitKeyCard
          value={draft.chatSubmitKey}
          isDirty={draft.chatSubmitKey !== saved.chatSubmitKey}
          onChange={(chatSubmitKey) => setDraft((current) => ({ ...current, chatSubmitKey }))}
        />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconCommand className="h-5 w-5" />}
        title={t("settings:keyboardShortcuts")}
        description={t("settings:customizeKeyboardShortcutsForTheCommand")}
      >
        <KeyboardShortcutsCard
          overrides={draft.keyboardShortcuts}
          baselineOverrides={saved.keyboardShortcuts}
          onChange={(keyboardShortcuts) =>
            setDraft((current) => ({ ...current, keyboardShortcuts }))
          }
          pluginEntries={pluginShortcutEntries}
        />
      </SettingsSection>
    </div>
  );
}
