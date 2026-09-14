"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { PluginRecord } from "@/lib/types/plugins";
import {
  buildPluginShortcutEntries,
  resolveShortcutEntry,
  type ShortcutEntry,
} from "@/lib/keyboard/plugin-shortcuts";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import { UNBOUND_SHORTCUT, type StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import {
  ShortcutRecorder,
  useShortcutConflictLabels,
} from "@/components/settings/keyboard-shortcuts-card";
import { SettingsCard } from "../settings-card";
import { usePluginShortcutDraft } from "./use-plugin-shortcut-draft";

type PluginShortcutEntry = Extract<ShortcutEntry, { source: "plugin" }>;

export function PluginShortcutsCard({
  plugin,
  plugins,
}: {
  plugin: PluginRecord;
  plugins: PluginRecord[];
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const pluginEntries = useMemo(() => buildPluginShortcutEntries(plugins), [plugins]);
  const selectedEntries = useMemo(
    () =>
      pluginEntries.filter(
        (entry): entry is PluginShortcutEntry =>
          entry.source === "plugin" && entry.pluginId === plugin.id,
      ),
    [plugin.id, pluginEntries],
  );
  const { saved, draft, setDraft, isDirty } = usePluginShortcutDraft(
    `plugin-shortcuts:${plugin.id}`,
  );

  const conflictLabels = useShortcutConflictLabels(pluginEntries, draft, t);
  if (selectedEntries.length === 0) return null;

  const touchSized = isMobile || !isFinePointer;
  return (
    <SettingsCard isDirty={isDirty} className="min-w-0" data-testid="plugin-shortcuts-card">
      <CardHeader>
        <CardTitle className="text-base">{t("plugins:shortcuts")}</CardTitle>
      </CardHeader>
      <CardContent className="min-w-0">
        <p
          className="mb-3 text-xs text-muted-foreground"
          data-testid="plugin-shortcuts-description"
        >
          {t("plugins:shortcutsDescription")}
        </p>
        <div className="min-w-0 divide-y divide-border">
          {selectedEntries.map((entry) => (
            <PluginShortcutRow
              key={entry.id}
              entry={entry}
              plugin={plugin}
              overrides={draft}
              baselineOverrides={saved}
              conflictsWith={conflictLabels.get(entry.id)}
              touchSized={touchSized}
              onChange={(id, shortcut) => setDraft((current) => ({ ...current, [id]: shortcut }))}
              onReset={(id) =>
                setDraft((current) => {
                  const next = { ...current };
                  delete next[id];
                  return next;
                })
              }
              onClear={(id) =>
                setDraft((current) => ({
                  ...current,
                  [id]: UNBOUND_SHORTCUT,
                }))
              }
            />
          ))}
        </div>
        <p className="mt-3 text-xs text-muted-foreground">
          {t("settings:clickAShortcutToRecordA")}
        </p>
      </CardContent>
    </SettingsCard>
  );
}

function PluginShortcutRow({
  entry,
  plugin,
  overrides,
  baselineOverrides,
  conflictsWith,
  touchSized,
  onChange,
  onReset,
  onClear,
}: {
  entry: PluginShortcutEntry;
  plugin: PluginRecord;
  overrides: StoredShortcutOverrides;
  baselineOverrides: StoredShortcutOverrides;
  conflictsWith?: string[];
  touchSized: boolean;
  onChange: (id: string, shortcut: KeyboardShortcut) => void;
  onReset: (id: string) => void;
  onClear: (id: string) => void;
}) {
  const keybinding = plugin.ui?.keybindings?.find(({ id }) => id === entry.keybindingId);
  const current = resolveShortcutEntry(entry, overrides);
  return (
    <ShortcutRecorder
      shortcutId={entry.id}
      label={keybinding?.description ?? entry.label}
      defaultShortcut={entry.default}
      current={current}
      onChange={onChange}
      onReset={onReset}
      onClear={onClear}
      isDirty={
        JSON.stringify(current) !== JSON.stringify(resolveShortcutEntry(entry, baselineOverrides))
      }
      conflictsWith={conflictsWith}
      touchSized={touchSized}
    />
  );
}
