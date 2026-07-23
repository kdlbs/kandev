"use client";

import { useCallback, useRef, useState } from "react";
import Link from "@/components/routing/app-link";
import { useTheme } from "@/components/theme/app-theme";
import {
  IconActivity,
  IconCommand,
  IconPalette,
  IconKeyboard,
  IconGitBranch,
  IconArchive,
} from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { KeyboardShortcutsCard } from "@/components/settings/keyboard-shortcuts-card";
import { SystemMetricsSettingsCard } from "@/components/settings/system-metrics-settings-card";
import { GENERAL_NAV_ITEMS } from "@/components/settings/general-nav";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { Theme } from "@/lib/settings/types";
import { ArchiveConfirmationSettings } from "@/components/settings/archive-confirmation-settings";
import { MCPTaskAgentProfileDefaultSettings } from "@/components/settings/mcp-task-agent-profile-default-settings";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import { buildPluginShortcutEntries } from "@/lib/keyboard/plugin-shortcuts";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";

function ThemeSettingsCard({
  theme,
  isDirty,
  onChange,
}: {
  theme: Theme;
  isDirty: boolean;
  onChange: (theme: Theme) => void;
}) {
  return (
    <SettingsCard isDirty={isDirty} data-testid="theme-settings-card">
      <CardHeader>
        <CardTitle className="text-base">Color Theme</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Select value={theme} onValueChange={(value) => onChange(value as Theme)}>
            <SelectTrigger id="theme" data-settings-dirty={isDirty}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="system">System</SelectItem>
              <SelectItem value="light">Light</SelectItem>
              <SelectItem value="dark">Dark</SelectItem>
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
  return (
    <SettingsCard isDirty={isDirty} data-testid="chat-submit-key-card">
      <CardHeader>
        <CardTitle className="text-base">Submit Shortcut</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="chat-submit-key">Message Submit Key</Label>
          <Select value={value} onValueChange={(next) => onChange(next as "enter" | "cmd_enter")}>
            <SelectTrigger id="chat-submit-key" data-settings-dirty={isDirty}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="cmd_enter">Cmd/Ctrl+Enter to send</SelectItem>
              <SelectItem value="enter">Enter to send</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {value === "cmd_enter"
              ? "Press Cmd/Ctrl+Enter to send messages. Press Enter for newlines."
              : "Press Enter to send messages. Press Shift+Enter for newlines."}
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
  return (
    <SettingsCard isDirty={isDirty} data-testid="changes-panel-layout-card">
      <CardHeader>
        <CardTitle className="text-base">Changes Panel Layout</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="changes-panel-layout">File list view</Label>
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
              <SelectItem value="flat">Flat list</SelectItem>
              <SelectItem value="tree">Tree</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            Display changed files as a flat list with full paths, or as a folder tree.
          </p>
        </div>
      </CardContent>
    </SettingsCard>
  );
}

export function GeneralSettings() {
  return (
    <div className="space-y-8">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {GENERAL_NAV_ITEMS.map(({ href, label, description, icon: Icon }) => (
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
  return (
    <div className="space-y-8">
      <SettingsSection
        icon={<IconArchive className="h-5 w-5" />}
        title="Task Actions"
        description="Configure archive safeguards and defaults for tasks created by agents"
      >
        <div className="space-y-4">
          <MCPTaskAgentProfileDefaultSettings />
          <ArchiveConfirmationSettings />
        </div>
      </SettingsSection>
    </div>
  );
}

export function AppearanceSettings() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const { savedTheme, previewTheme, commitTheme, restoreTheme } = useTheme();
  const [saved, setSaved] = useState(() => ({
    theme: savedTheme,
    changesPanelLayout: userSettings.changesPanelLayout,
    showMetrics: userSettings.systemMetricsDisplay.showInTopbar,
  }));
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
        changes_panel_layout: submitted.changesPanelLayout,
        system_metrics_display: { show_in_topbar: submitted.showMetrics },
      });
      commitTheme(submitted.theme);
      if (draftRef.current.theme !== submitted.theme) {
        previewTheme(draftRef.current.theme);
      }
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        changesPanelLayout: submitted.changesPanelLayout,
        systemMetricsDisplay: { showInTopbar: submitted.showMetrics },
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
      <SettingsSection
        icon={<IconPalette className="h-5 w-5" />}
        title="Appearance"
        description="Customize how the application looks"
      >
        <ThemeSettingsCard
          theme={draft.theme}
          isDirty={draft.theme !== saved.theme}
          onChange={(theme) => {
            updateDraft({ theme });
            previewTheme(theme);
          }}
        />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title="Changes Panel"
        description="Customize how changed files are displayed"
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
        title="Resource Metrics"
        description="Configure backend and execution resource sampling"
      >
        <SystemMetricsSettingsCard
          showInTopbar={draft.showMetrics}
          isShowInTopbarDirty={draft.showMetrics !== saved.showMetrics}
          onShowInTopbarChange={(showMetrics) => updateDraft({ showMetrics })}
        />
      </SettingsSection>
    </div>
  );
}

export function KeyboardShortcutsSettings() {
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
        title="Chat Input"
        description="Configure chat input behavior"
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
        title="Keyboard Shortcuts"
        description="Customize keyboard shortcuts for the command panel"
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
