"use client";

import { useState } from "react";
import { useTheme } from "next-themes";
import { IconCommand, IconLink, IconPalette, IconServer, IconKeyboard } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { ShellSettingsCard } from "@/components/settings/shell-settings-card";
import { KeyboardShortcutsCard } from "@/components/settings/keyboard-shortcuts-card";
import { getBackendConfig } from "@/lib/config";
import { useAppStore } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { Theme } from "@/lib/settings/types";

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

function TerminalLinksCard() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const [isSaving, setIsSaving] = useState(false);

  const handleChange = async (value: "new_tab" | "browser_panel") => {
    if (isSaving) return;
    setIsSaving(true);
    const previous = userSettings.terminalLinkBehavior;
    try {
      setUserSettings({ ...userSettings, terminalLinkBehavior: value });
      await updateUserSettings({
        workspace_id: userSettings.workspaceId || "",
        repository_ids: userSettings.repositoryIds || [],
        terminal_link_behavior: value,
      });
    } catch {
      setUserSettings({ ...userSettings, terminalLinkBehavior: previous });
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Terminal Links</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="terminal-link-behavior">Open links in</Label>
          <Select
            value={userSettings.terminalLinkBehavior}
            onValueChange={(v) => handleChange(v as "new_tab" | "browser_panel")}
            disabled={isSaving}
          >
            <SelectTrigger id="terminal-link-behavior">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="new_tab">New browser tab</SelectItem>
              <SelectItem value="browser_panel">Built-in browser panel</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            Cmd+click (or Ctrl+click) a URL in the terminal to open it.
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
            placeholder="http://localhost:8080"
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
          Manage your application preferences and notifications
        </p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconPalette className="h-5 w-5" />}
        title="Appearance"
        description="Customize how the application looks"
      >
        <ThemeSettingsCard />
      </SettingsSection>

      <Separator />

      <ShellSettingsCard />

      <Separator />

      <SettingsSection
        icon={<IconLink className="h-5 w-5" />}
        title="Terminal Links"
        description="Configure how clickable URLs in the terminal are opened"
      >
        <TerminalLinksCard />
      </SettingsSection>

      <Separator />

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
