"use client";

import { useState } from "react";
import { useTheme } from "next-themes";
import {
  IconCommand,
  IconPalette,
  IconServer,
  IconKeyboard,
  IconTerminal2,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { ShellSettingsCard } from "@/components/settings/shell-settings-card";
import { KeyboardShortcutsCard } from "@/components/settings/keyboard-shortcuts-card";
import { getBackendConfig } from "@/lib/config";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { TERMINAL_FONT_PRESETS } from "@/lib/terminal/terminal-font";
import type { FontCategory } from "@/lib/terminal/terminal-font";
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
  const storeApi = useAppStoreApi();
  const [isSaving, setIsSaving] = useState(false);

  const handleChange = async (value: "new_tab" | "browser_panel") => {
    if (isSaving) return;
    setIsSaving(true);
    const current = storeApi.getState().userSettings;
    const previous = current.terminalLinkBehavior;
    try {
      setUserSettings({ ...current, terminalLinkBehavior: value });
      await updateUserSettings({
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        terminal_link_behavior: value,
      });
    } catch {
      setUserSettings({
        ...storeApi.getState().userSettings,
        terminalLinkBehavior: previous,
      });
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
          <p className="text-xs text-muted-foreground">Click a URL in the terminal to open it.</p>
        </div>
      </CardContent>
    </Card>
  );
}

const CUSTOM_VALUE = "__custom__";
const CATEGORY_LABELS: Record<FontCategory, string> = {
  icons: "Nerd Fonts",
  ligatures: "Programming",
  system: "System",
};
const CATEGORY_BADGES: Partial<Record<FontCategory, string>> = {
  icons: "Icons",
  ligatures: "Ligatures",
};
const FONT_GROUPS: Record<string, typeof TERMINAL_FONT_PRESETS> = TERMINAL_FONT_PRESETS.reduce(
  (acc, p) => {
    (acc[p.category] ??= []).push(p);
    return acc;
  },
  {} as Record<string, typeof TERMINAL_FONT_PRESETS>,
);
const FONT_CATEGORIES: FontCategory[] = ["icons", "ligatures", "system"];

function FontGroupOptions() {
  return FONT_CATEGORIES.map((category) => (
    <SelectGroup key={category}>
      <SelectLabel className="flex items-center gap-2">
        {CATEGORY_LABELS[category]}
        {CATEGORY_BADGES[category] && (
          <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
            {CATEGORY_BADGES[category]}
          </Badge>
        )}
      </SelectLabel>
      {(FONT_GROUPS[category] ?? []).map((preset) => (
        <SelectItem key={preset.value} value={preset.value}>
          {preset.label}
        </SelectItem>
      ))}
    </SelectGroup>
  ));
}

function TerminalFontSizeCard() {
  const storeApi = useAppStoreApi();
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const [isSaving, setIsSaving] = useState(false);
  const [fontSize, setFontSize] = useState(() => userSettings.terminalFontSize ?? 13);

  const saveFontSize = async (value: number) => {
    if (isSaving) return;
    if (value < 8 || value > 24) return;
    setIsSaving(true);
    const current = storeApi.getState().userSettings;
    const previous = current.terminalFontSize;
    try {
      setUserSettings({ ...current, terminalFontSize: value });
      await updateUserSettings({
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        terminal_font_size: value,
      });
    } catch {
      setUserSettings({ ...storeApi.getState().userSettings, terminalFontSize: previous });
    } finally {
      setIsSaving(false);
    }
  };

  const handleFontSizeBlur = () => {
    const v = Math.min(24, Math.max(8, fontSize));
    setFontSize(v);
    saveFontSize(v);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Terminal Font Size</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="terminal-font-size">Font Size</Label>
          <div className="flex items-center gap-3">
            <Input
              id="terminal-font-size"
              type="number"
              min={8}
              max={24}
              value={fontSize}
              onChange={(e) => setFontSize(Number(e.target.value))}
              onBlur={handleFontSizeBlur}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleFontSizeBlur();
              }}
              className="w-20"
              disabled={isSaving}
              data-testid="terminal-font-size-input"
            />
            <span className="text-xs text-muted-foreground">px (8-24)</span>
          </div>
          <p className="text-xs text-muted-foreground">
            Set the font size for the terminal. Default is 13px.
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

function TerminalFontCard() {
  const storeApi = useAppStoreApi();
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const [isSaving, setIsSaving] = useState(false);
  const [isCustom, setIsCustom] = useState(() => {
    const current = userSettings.terminalFontFamily;
    if (!current) return false;
    return !TERMINAL_FONT_PRESETS.some((p) => p.value === current);
  });
  const [customValue, setCustomValue] = useState(
    () => (isCustom ? userSettings.terminalFontFamily : "") ?? "",
  );

  const saveFontFamily = async (value: string) => {
    if (isSaving) return;
    setIsSaving(true);
    const current = storeApi.getState().userSettings;
    const previous = current.terminalFontFamily;
    try {
      setUserSettings({ ...current, terminalFontFamily: value || null });
      await updateUserSettings({
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        terminal_font_family: value,
      });
    } catch {
      setUserSettings({
        ...storeApi.getState().userSettings,
        terminalFontFamily: previous,
      });
    } finally {
      setIsSaving(false);
    }
  };

  const handleSelectChange = (value: string) => {
    if (value === CUSTOM_VALUE) {
      setIsCustom(true);
      return;
    }
    setIsCustom(false);
    setCustomValue("");
    saveFontFamily(value === "default" ? "" : value);
  };

  const handleCustomBlur = () => {
    const trimmed = customValue.trim();
    if (trimmed) saveFontFamily(trimmed);
  };

  const selectValue = isCustom ? CUSTOM_VALUE : userSettings.terminalFontFamily || "default";

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Terminal Font</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          <Label htmlFor="terminal-font">Font Family</Label>
          <Select value={selectValue} onValueChange={handleSelectChange} disabled={isSaving}>
            <SelectTrigger id="terminal-font" data-testid="terminal-font-select">
              <SelectValue placeholder="Default" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="default">Default (Menlo / Monaco)</SelectItem>
              <FontGroupOptions />
              <SelectSeparator />
              <SelectItem value={CUSTOM_VALUE}>Custom...</SelectItem>
            </SelectContent>
          </Select>
          {isCustom && (
            <Input
              placeholder='e.g. "My Custom Font"'
              value={customValue}
              onChange={(e) => setCustomValue(e.target.value)}
              onBlur={handleCustomBlur}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCustomBlur();
              }}
              data-testid="terminal-font-custom-input"
            />
          )}
          <p className="text-xs text-muted-foreground">
            Choose a monospace font for the terminal. Nerd Fonts include icons for CLI tools.
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
        icon={<IconTerminal2 className="h-5 w-5" />}
        title="Terminal"
        description="Configure terminal appearance and behavior"
      >
        <TerminalFontCard />
        <TerminalFontSizeCard />
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
