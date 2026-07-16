"use client";

import { useEffect, useState } from "react";
import { IconTerminal2 } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
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
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { useShellSettings } from "@/hooks/domains/settings/use-shell-settings";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { TERMINAL_FONT_PRESETS } from "@/lib/terminal/terminal-font";
import type { FontCategory } from "@/lib/terminal/terminal-font";

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

export function normalizeTerminalFontSize(value: number, fallback: number): number {
  const base = Number.isFinite(value) ? value : fallback;
  return Math.min(24, Math.max(8, base));
}

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

function TerminalFontSizeCard({
  fontSize,
  isDirty,
  onChange,
}: {
  fontSize: number;
  isDirty: boolean;
  onChange: (value: number) => void;
}) {
  const handleFontSizeBlur = () => {
    onChange(normalizeTerminalFontSize(fontSize, 13));
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
              data-settings-dirty={isDirty}
              onChange={(e) => onChange(Number(e.target.value))}
              onBlur={handleFontSizeBlur}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleFontSizeBlur();
              }}
              className="w-20"
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

function TerminalFontCard({
  fontFamily,
  isDirty,
  onChange,
}: {
  fontFamily: string | null;
  isDirty: boolean;
  onChange: (value: string | null) => void;
}) {
  const [isCustom, setIsCustom] = useState(() => {
    const current = fontFamily;
    if (!current) return false;
    return !TERMINAL_FONT_PRESETS.some((p) => p.value === current);
  });
  const [customValue, setCustomValue] = useState(() => (isCustom ? fontFamily : "") ?? "");

  useEffect(() => {
    const nextIsCustom = Boolean(
      fontFamily && !TERMINAL_FONT_PRESETS.some((preset) => preset.value === fontFamily),
    );
    setIsCustom(nextIsCustom);
    setCustomValue(nextIsCustom ? (fontFamily ?? "") : "");
  }, [fontFamily]);

  const handleSelectChange = (value: string) => {
    if (value === CUSTOM_VALUE) {
      setIsCustom(true);
      return;
    }
    setIsCustom(false);
    setCustomValue("");
    onChange(value === "default" ? null : value);
  };

  const handleCustomBlur = () => {
    const trimmed = customValue.trim();
    if (trimmed) onChange(trimmed);
  };

  const selectValue = isCustom ? CUSTOM_VALUE : fontFamily || "default";

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Terminal Font</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          <Label htmlFor="terminal-font">Font Family</Label>
          <Select value={selectValue} onValueChange={handleSelectChange}>
            <SelectTrigger
              id="terminal-font"
              data-testid="terminal-font-select"
              data-settings-dirty={isDirty}
            >
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
              data-settings-dirty={isDirty}
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

function TerminalLinksCard({
  value,
  isDirty,
  onChange,
}: {
  value: "new_tab" | "browser_panel";
  isDirty: boolean;
  onChange: (value: "new_tab" | "browser_panel") => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Terminal Links</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="terminal-link-behavior">Open links in</Label>
          <Select
            value={value}
            onValueChange={(next) => onChange(next as "new_tab" | "browser_panel")}
          >
            <SelectTrigger id="terminal-link-behavior" data-settings-dirty={isDirty}>
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

export function TerminalSettings() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const shellSettings = useShellSettings();
  const [saved, setSaved] = useState(() => ({
    preferredShell: shellSettings.preferredShell ?? "",
    terminalFontFamily: userSettings.terminalFontFamily,
    terminalFontSize: userSettings.terminalFontSize ?? 13,
    terminalLinkBehavior: userSettings.terminalLinkBehavior,
  }));
  const [draft, setDraft] = useState(saved);
  const revision = JSON.stringify(draft);
  const validFontSize = normalizeTerminalFontSize(draft.terminalFontSize, 13);

  useSettingsSaveContributor({
    id: "general-terminal",
    revision,
    isDirty: revision !== JSON.stringify(saved),
    canSave: Number.isFinite(draft.terminalFontSize),
    invalidReason: "Terminal font size must be a number between 8 and 24.",
    save: async () => {
      const submitted = { ...draft, terminalFontSize: validFontSize };
      const current = storeApi.getState().userSettings;
      await updateUserSettings({
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        preferred_shell: submitted.preferredShell.trim(),
        terminal_font_family: submitted.terminalFontFamily ?? "",
        terminal_font_size: submitted.terminalFontSize,
        terminal_link_behavior: submitted.terminalLinkBehavior,
      });
      setSaved(submitted);
      setDraft((latest) => (latest === draft ? submitted : latest));
      setUserSettings({
        ...storeApi.getState().userSettings,
        preferredShell: submitted.preferredShell.trim() || null,
        terminalFontFamily: submitted.terminalFontFamily,
        terminalFontSize: submitted.terminalFontSize,
        terminalLinkBehavior: submitted.terminalLinkBehavior,
      });
    },
    discard: () => setDraft(saved),
  });

  return (
    <div className="space-y-8">
      <ShellSettingsCard
        preferredShell={draft.preferredShell}
        isDirty={draft.preferredShell !== saved.preferredShell}
        onPreferredShellChange={(preferredShell) =>
          setDraft((current) => ({ ...current, preferredShell }))
        }
        shellLoaded={shellSettings.loaded}
        shellOptions={shellSettings.shellOptions ?? []}
      />

      <Separator />

      <SettingsSection
        icon={<IconTerminal2 className="h-5 w-5" />}
        title="Terminal"
        description="Configure terminal appearance and behavior"
      >
        <TerminalFontCard
          fontFamily={draft.terminalFontFamily}
          isDirty={draft.terminalFontFamily !== saved.terminalFontFamily}
          onChange={(terminalFontFamily) =>
            setDraft((current) => ({ ...current, terminalFontFamily }))
          }
        />
        <TerminalFontSizeCard
          fontSize={draft.terminalFontSize}
          isDirty={draft.terminalFontSize !== saved.terminalFontSize}
          onChange={(terminalFontSize) => setDraft((current) => ({ ...current, terminalFontSize }))}
        />
        <TerminalLinksCard
          value={draft.terminalLinkBehavior}
          isDirty={draft.terminalLinkBehavior !== saved.terminalLinkBehavior}
          onChange={(terminalLinkBehavior) =>
            setDraft((current) => ({ ...current, terminalLinkBehavior }))
          }
        />
      </SettingsSection>
    </div>
  );
}
