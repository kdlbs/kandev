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
  IconServer,
  IconKeyboard,
  IconTerminal2,
  IconGitBranch,
} from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { ShellSettingsCard } from "@/components/settings/shell-settings-card";
import { KeyboardShortcutsCard } from "@/components/settings/keyboard-shortcuts-card";
import { SystemMetricsSettingsCard } from "@/components/settings/system-metrics-settings-card";
import { getBackendConfig } from "@/lib/config";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { Theme } from "@/lib/settings/types";

const GENERAL_SETTING_LINKS = [
  {
    href: "/settings/general/appearance",
    title: "Appearance",
    description: "Color theme and visual preferences",
    icon: IconPalette,
  },
  {
    href: "/settings/general/shell",
    title: "Shell",
    description: "Default shell for task sessions",
    icon: IconTerminal2,
  },
  {
    href: "/settings/general/terminal",
    title: "Terminal",
    description: "Terminal fonts and link behavior",
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
    href: "/settings/general/resource-metrics",
    title: "Resource Metrics",
    description: "Backend and execution sampling",
    icon: IconActivity,
  },
  {
    href: "/settings/general/chat-input",
    title: "Chat Input",
    description: "Message submit behavior",
    icon: IconKeyboard,
  },
  {
    href: "/settings/general/changes-panel",
    title: "Changes Panel",
    description: "Changed-file display preferences",
    icon: IconGitBranch,
  },
  {
    href: "/settings/general/keyboard-shortcuts",
    title: "Keyboard Shortcuts",
    description: "Command panel shortcuts",
    icon: IconCommand,
  },
  {
    href: "/settings/general/backend-connection",
    title: "Backend Connection",
    description: "Runtime backend server URL",
    icon: IconServer,
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

function BackendConnectionCard() {
  const [backendUrl] = useState<string>(() => getBackendConfig().apiBaseUrl);
  const displayBackendUrl = backendUrl.replace(/^https?:\/\//, "").replace(/\/$/, "");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Backend Server URL</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="backend-url">Server URL</Label>
          <Input
            id="backend-url"
            type="url"
            value={displayBackendUrl}
            readOnly
            disabled
            placeholder="http://localhost:38429"
            className="cursor-default"
          />
          <p className="text-xs text-muted-foreground">
            Backend URL is provided at runtime for SSR and WebSocket connections.
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

export function GeneralSettings() {
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">General Settings</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Manage application preferences, input behavior, and local runtime defaults
        </p>
      </div>

      <Separator />

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
      <div>
        <h2 className="text-2xl font-bold">Appearance</h2>
        <p className="text-sm text-muted-foreground mt-1">Customize how the application looks</p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconPalette className="h-5 w-5" />}
        title="Appearance"
        description="Customize how the application looks"
      >
        <ThemeSettingsCard />
      </SettingsSection>
    </div>
  );
}

export function ShellSettings() {
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Shell</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Pick the default shell for task sessions
        </p>
      </div>

      <Separator />

      <ShellSettingsCard />
    </div>
  );
}

export function ResourceMetricsSettings() {
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Resource Metrics</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Configure backend and execution resource sampling
        </p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconActivity className="h-5 w-5" />}
        title="Resource Metrics"
        description="Configure backend and execution resource sampling"
      >
        <SystemMetricsSettingsCard />
      </SettingsSection>
    </div>
  );
}

export function ChatInputSettings() {
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Chat Input</h2>
        <p className="text-sm text-muted-foreground mt-1">Configure chat input behavior</p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconKeyboard className="h-5 w-5" />}
        title="Chat Input"
        description="Configure chat input behavior"
      >
        <ChatSubmitKeyCard />
      </SettingsSection>
    </div>
  );
}

export function ChangesPanelSettings() {
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Changes Panel</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Customize how changed files are displayed
        </p>
      </div>

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
      <div>
        <h2 className="text-2xl font-bold">Keyboard Shortcuts</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Customize keyboard shortcuts for the command panel
        </p>
      </div>

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

export function BackendConnectionSettings() {
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Backend Connection</h2>
        <p className="text-sm text-muted-foreground mt-1">View the runtime backend server URL</p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconServer className="h-5 w-5" />}
        title="Backend Connection"
        description="Configure the backend server URL"
      >
        <BackendConnectionCard />
      </SettingsSection>
    </div>
  );
}
