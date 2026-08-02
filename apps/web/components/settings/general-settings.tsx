"use client";
import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { useTheme } from "@/components/theme/app-theme";
import {
  IconActivity,
  IconCommand,
  IconPalette,
  IconKeyboard,
  IconGitBranch,
  IconArchive,
  IconArrowBackUp,
  IconHome,
} from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { KeyboardShortcutsCard } from "@/components/settings/keyboard-shortcuts-card";
import { SystemMetricsSettingsCard } from "@/components/settings/system-metrics-settings-card";
import { useGeneralNavItems } from "@/components/settings/general-nav";
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
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import { buildPluginShortcutEntries } from "@/lib/keyboard/plugin-shortcuts";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { StartupPageSettingsCard } from "@/components/settings/startup-page-settings-card";

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
    <SettingsCard isDirty={isDirty} data-testid="theme-settings-card">
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
    <SettingsCard isDirty={isDirty} data-testid="chat-submit-key-card">
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
    <SettingsCard isDirty={isDirty} data-testid="changes-panel-layout-card">
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

function createAppearanceSavedState(
  theme: Theme,
  userSettings: Pick<
    UserSettingsState,
    "changesPanelLayout" | "startupPage" | "systemMetricsDisplay"
  >,
) {
  return {
    theme,
    changesPanelLayout: userSettings.changesPanelLayout,
    startupPage: userSettings.startupPage,
    showMetrics: userSettings.systemMetricsDisplay.showInTopbar,
    simplifiedMetrics: userSettings.systemMetricsDisplay.simplified,
  };
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

export function GeneralSettings() {
  const navItems = useGeneralNavItems();
  return (
    <div className="space-y-8">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {navItems.map(({ href, label, description, icon: Icon }) => (
          <Link key={href} href={href} className="cursor-pointer">
            <Card className="h-full transition-colors hover:bg-muted/40">
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Icon className="h-4 w-4 text-muted-foreground" />
                  {label}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-sm text-muted-foreground">{description}</p>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>
    </div>
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
    </div>
  );
}

export function AppearanceSettings() {
  const { t } = useTranslation();
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const { savedTheme, previewTheme, commitTheme, restoreTheme } = useTheme();
  const [saved, setSaved] = useState(() => createAppearanceSavedState(savedTheme, userSettings));
  const [draft, setDraft] = useState(saved);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const revision = JSON.stringify(draft);
  const isDirty = revision !== JSON.stringify(saved);

  useSettingsSaveContributor({
    id: "general-appearance",
    order: 10,
    revision,
    isDirty,
    save: async () => {
      const submitted = draft;
      const current = storeApi.getState().userSettings;
      await updateUserSettings({
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        startup_page: submitted.startupPage,
        changes_panel_layout: submitted.changesPanelLayout,
        system_metrics_display: {
          show_in_topbar: submitted.showMetrics,
          simplified: submitted.simplifiedMetrics,
        },
      });
      commitTheme(submitted.theme);
      if (draftRef.current.theme !== submitted.theme) {
        previewTheme(draftRef.current.theme);
      }
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        changesPanelLayout: submitted.changesPanelLayout,
        startupPage: submitted.startupPage,
        systemMetricsDisplay: {
          showInTopbar: submitted.showMetrics,
          simplified: submitted.simplifiedMetrics,
        },
      });
    },
    discard: () => {
      setDraft(saved);
      restoreTheme();
    },
  });

  const updateDraft = useCallback(
    (patch: Partial<typeof draft>) => setDraft((current) => ({ ...current, ...patch })),
    [],
  );

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

      <StartupPageSettingsSection
        value={draft.startupPage}
        isDirty={draft.startupPage !== saved.startupPage}
        onChange={(startupPage) => updateDraft({ startupPage })}
      />

      <LanguageSettings />

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
