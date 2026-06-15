"use client";

import { useState } from "react";
import Link from "next/link";
import { useTheme } from "next-themes";
import {
  IconActivity,
  IconBell,
  IconCommand,
  IconCode,
  IconPalette,
  IconKeyboard,
  IconTerminal2,
  IconGitBranch,
} from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { KeyboardShortcutsCard } from "@/components/settings/keyboard-shortcuts-card";
import { SystemMetricsSettingsCard } from "@/components/settings/system-metrics-settings-card";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { Theme } from "@/lib/settings/types";

const GENERAL_SETTING_LINKS = [
  {
    href: "/settings/general/appearance",
    title: "Appearance",
    description: "Theme, metrics, and changes panel preferences",
    icon: IconPalette,
  },
  {
    href: "/settings/general/terminal",
    title: "Terminal",
    description: "Shell, terminal fonts, and link behavior",
    icon: IconTerminal2,
  },
  {
    href: "/settings/general/notifications",
    title: "Notifications",
    description: "Providers and notification events",
    icon: IconBell,
  },
  {
    href: "/settings/general/editors",
    title: "Editors",
    description: "Editor integrations and defaults",
    icon: IconCode,
  },
  {
    href: "/settings/general/keyboard-shortcuts",
    title: "Keyboard Shortcuts",
    description: "Chat input and command shortcuts",
    icon: IconCommand,
  },
];

function ThemeSettingsCard() {
  const { theme: currentTheme, setTheme } = useTheme();
  const themeValue = currentTheme ?? "system";

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Color Theme</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Select value={themeValue} onValueChange={(value) => setTheme(value as Theme)}>
            <SelectTrigger id="theme">
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
    </Card>
  );
}

function ChatSubmitKeyCard() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const [isSavingSubmitKey, setIsSavingSubmitKey] = useState(false);

  const handleChatSubmitKeyChange = async (value: "enter" | "cmd_enter") => {
    if (isSavingSubmitKey) return;
    setIsSavingSubmitKey(true);
    const previousValue = userSettings.chatSubmitKey;
    try {
      setUserSettings({ ...userSettings, chatSubmitKey: value });
      await updateUserSettings({
        workspace_id: userSettings.workspaceId || "",
        repository_ids: userSettings.repositoryIds || [],
        chat_submit_key: value,
      });
    } catch {
      setUserSettings({ ...userSettings, chatSubmitKey: previousValue });
    } finally {
      setIsSavingSubmitKey(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Submit Shortcut</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="chat-submit-key">Message Submit Key</Label>
          <Select
            value={userSettings.chatSubmitKey}
            onValueChange={(value) => handleChatSubmitKeyChange(value as "enter" | "cmd_enter")}
            disabled={isSavingSubmitKey}
          >
            <SelectTrigger id="chat-submit-key">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="cmd_enter">Cmd/Ctrl+Enter to send</SelectItem>
              <SelectItem value="enter">Enter to send</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {userSettings.chatSubmitKey === "cmd_enter"
              ? "Press Cmd/Ctrl+Enter to send messages. Press Enter for newlines."
              : "Press Enter to send messages. Press Shift+Enter for newlines."}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

function ChangesPanelLayoutCard() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [isSaving, setIsSaving] = useState(false);

  const handleChange = async (value: "flat" | "tree") => {
    if (isSaving) return;
    setIsSaving(true);
    const current = storeApi.getState().userSettings;
    const previous = current.changesPanelLayout;
    try {
      setUserSettings({ ...current, changesPanelLayout: value });
      await updateUserSettings({
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        changes_panel_layout: value,
      });
    } catch {
      setUserSettings({ ...storeApi.getState().userSettings, changesPanelLayout: previous });
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Changes Panel Layout</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="changes-panel-layout">File list view</Label>
          <Select
            value={userSettings.changesPanelLayout}
            onValueChange={(v) => handleChange(v as "flat" | "tree")}
            disabled={isSaving}
          >
            <SelectTrigger
              id="changes-panel-layout"
              data-testid="changes-panel-layout-select"
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
    </Card>
  );
}

export function GeneralSettings() {
  return (
    <div className="space-y-8">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {GENERAL_SETTING_LINKS.map(({ href, title, description, icon: Icon }) => (
          <Link key={href} href={href} className="cursor-pointer">
            <Card className="h-full transition-colors hover:bg-muted/40">
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Icon className="h-4 w-4 text-muted-foreground" />
                  {title}
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

export function AppearanceSettings() {
  return (
    <div className="space-y-8">
      <SettingsSection
        icon={<IconPalette className="h-5 w-5" />}
        title="Appearance"
        description="Customize how the application looks"
      >
        <ThemeSettingsCard />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconActivity className="h-5 w-5" />}
        title="Resource Metrics"
        description="Configure backend and execution resource sampling"
      >
        <SystemMetricsSettingsCard />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title="Changes Panel"
        description="Customize how changed files are displayed"
      >
        <ChangesPanelLayoutCard />
      </SettingsSection>
    </div>
  );
}

export function KeyboardShortcutsSettings() {
  return (
    <div className="space-y-8">
      <SettingsSection
        icon={<IconKeyboard className="h-5 w-5" />}
        title="Chat Input"
        description="Configure chat input behavior"
      >
        <ChatSubmitKeyCard />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconCommand className="h-5 w-5" />}
        title="Keyboard Shortcuts"
        description="Customize keyboard shortcuts for the command panel"
      >
        <KeyboardShortcutsCard />
      </SettingsSection>
    </div>
  );
}
