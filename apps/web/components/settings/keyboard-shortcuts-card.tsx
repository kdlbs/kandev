"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconAlertTriangle, IconRotate, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Kbd } from "@kandev/ui/kbd";
import type { Key, KeyboardShortcut } from "@/lib/keyboard/constants";
import { formatShortcut, isMac } from "@/lib/keyboard/utils";
import {
  CONFIGURABLE_SHORTCUTS,
  UNBOUND_SHORTCUT,
  isUnboundShortcut,
  resolveAllShortcuts,
  type ConfigurableShortcutId,
  type StoredShortcutOverrides,
} from "@/lib/keyboard/shortcut-overrides";
import {
  coreShortcutEntries,
  resolveShortcutEntry,
  type ShortcutEntry,
} from "@/lib/keyboard/plugin-shortcuts";
import {
  findShortcutConflicts,
  type ShortcutConflictGroup,
} from "@/lib/keyboard/shortcut-conflicts";
import { SettingsCard } from "./settings-card";

type ShortcutRecorderProps = {
  shortcutId: string;
  label: string;
  defaultShortcut: KeyboardShortcut;
  current: KeyboardShortcut;
  onChange: (id: string, shortcut: KeyboardShortcut) => void;
  onReset: (id: string) => void;
  // Optional: callers that don't support an explicit "unbind" (e.g. the voice
  // settings recorder) omit this, and the Clear button is hidden for them.
  onClear?: (id: string) => void;
  isDirty?: boolean;
  conflictsWith?: string[];
};

export function ShortcutRecorder({
  shortcutId,
  label,
  defaultShortcut,
  current,
  onChange,
  onReset,
  onClear,
  isDirty = false,
  conflictsWith,
}: ShortcutRecorderProps) {
  const [recording, setRecording] = useState(false);
  const isDefault = JSON.stringify(current) === JSON.stringify(defaultShortcut);
  const isUnbound = isUnboundShortcut(current);
  const defaultIsUnbound = isUnboundShortcut(defaultShortcut);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (!recording) return;
      if (["Control", "Meta", "Alt", "Shift"].includes(e.key)) return;

      e.preventDefault();
      e.stopPropagation();

      const newShortcut: KeyboardShortcut = {
        key: (e.key.length === 1 ? e.key.toLowerCase() : e.key) as Key,
        modifiers: {
          ...(e.ctrlKey || e.metaKey ? { ctrlOrCmd: true } : {}),
          ...(e.shiftKey ? { shift: true } : {}),
          ...(e.altKey ? { alt: true } : {}),
        },
      };

      if (Object.keys(newShortcut.modifiers!).length === 0) {
        delete newShortcut.modifiers;
      }

      onChange(shortcutId, newShortcut);
      setRecording(false);
    },
    [recording, shortcutId, onChange],
  );

  useEffect(() => {
    if (!recording) return;
    window.addEventListener("keydown", handleKeyDown, true);
    return () => window.removeEventListener("keydown", handleKeyDown, true);
  }, [recording, handleKeyDown]);

  useEffect(() => {
    if (!recording) return;
    const handleBlur = () => setRecording(false);
    window.addEventListener("blur", handleBlur);
    return () => window.removeEventListener("blur", handleBlur);
  }, [recording]);

  return (
    <div className="flex items-center justify-between py-2">
      <ShortcutRecorderLabel label={label} conflictsWith={conflictsWith} />
      <div className="flex items-center gap-2">
        <button
          data-testid={`shortcut-recorder-${shortcutId}`}
          data-settings-dirty={isDirty}
          onClick={() => setRecording(!recording)}
          className={`px-3 py-1.5 rounded-md border text-sm cursor-pointer transition-colors ${
            recording
              ? "border-primary bg-primary/10 text-primary"
              : "border-border bg-background hover:bg-accent"
          }`}
        >
          {renderRecorderLabel({ recording, current, isUnbound })}
        </button>
        <ShortcutRecorderActions
          shortcutId={shortcutId}
          isDefault={isDefault}
          isUnbound={isUnbound}
          defaultIsUnbound={defaultIsUnbound}
          onReset={onReset}
          onClear={onClear}
        />
      </div>
    </div>
  );
}

function ShortcutRecorderLabel({
  label,
  conflictsWith,
}: {
  label: string;
  conflictsWith?: string[];
}) {
  return (
    <span className="text-sm flex items-center gap-1.5">
      {label}
      {conflictsWith && conflictsWith.length > 0 && (
        <span
          className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-500"
          title={`Same shortcut as: ${conflictsWith.join(", ")}`}
        >
          <IconAlertTriangle className="size-3.5" />
        </span>
      )}
    </span>
  );
}

function ShortcutRecorderActions({
  shortcutId,
  isDefault,
  isUnbound,
  defaultIsUnbound,
  onReset,
  onClear,
}: {
  shortcutId: string;
  isDefault: boolean;
  isUnbound: boolean;
  defaultIsUnbound: boolean;
  onReset: (id: string) => void;
  onClear?: (id: string) => void;
}) {
  return (
    <>
      {onClear && !isUnbound && !defaultIsUnbound && (
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 cursor-pointer"
          onClick={() => onClear(shortcutId)}
          aria-label="Clear shortcut"
          title="Clear shortcut"
        >
          <IconX className="size-3.5" />
        </Button>
      )}
      {!isDefault && (
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 cursor-pointer"
          onClick={() => onReset(shortcutId)}
          aria-label={defaultIsUnbound ? "Reset (clear shortcut)" : "Reset to default"}
          title={defaultIsUnbound ? "Reset (clear shortcut)" : "Reset to default"}
        >
          <IconRotate className="size-3.5" />
        </Button>
      )}
    </>
  );
}

function renderRecorderLabel({
  recording,
  current,
  isUnbound,
}: {
  recording: boolean;
  current: KeyboardShortcut;
  isUnbound: boolean;
}) {
  if (recording) return <span className="animate-pulse">Press a key combo...</span>;
  if (isUnbound) return <span className="text-muted-foreground italic">Unbound</span>;
  return <Kbd>{formatShortcut(current)}</Kbd>;
}

/** Builds a `shortcutId -> conflicting labels` lookup from conflict groups. */
function buildConflictLabels(groups: ShortcutConflictGroup[]): Map<string, string[]> {
  const labels = new Map<string, string[]>();
  for (const group of groups) {
    for (const entry of group.entries) {
      labels.set(
        entry.id,
        group.entries.filter((other) => other.id !== entry.id).map((other) => other.label),
      );
    }
  }
  return labels;
}

export function KeyboardShortcutsCard({
  overrides,
  baselineOverrides = {},
  onChange,
  pluginEntries = [],
}: {
  overrides: StoredShortcutOverrides;
  baselineOverrides?: StoredShortcutOverrides;
  onChange: (overrides: StoredShortcutOverrides) => void;
  /** Dynamic plugin-declared shortcuts (see `lib/keyboard/plugin-shortcuts.ts`). */
  pluginEntries?: ShortcutEntry[];
}) {
  const shortcuts = resolveAllShortcuts(overrides);
  const baselineShortcuts = resolveAllShortcuts(baselineOverrides);

  const allEntries = useMemo(() => [...coreShortcutEntries(), ...pluginEntries], [pluginEntries]);

  const conflictLabels = useMemo(() => {
    const resolved = allEntries.map((entry) => ({
      entry,
      shortcut: resolveShortcutEntry(entry, overrides),
    }));
    return buildConflictLabels(findShortcutConflicts(resolved, isMac()));
  }, [allEntries, overrides]);

  const handleChange = useCallback(
    (id: string, shortcut: KeyboardShortcut) => {
      onChange({ ...overrides, [id]: shortcut });
    },
    [onChange, overrides],
  );

  const handleReset = useCallback(
    (id: string) => {
      const next = { ...overrides };
      delete next[id];
      onChange(next);
    },
    [onChange, overrides],
  );

  const handleClear = useCallback(
    (id: string) => {
      onChange({ ...overrides, [id]: UNBOUND_SHORTCUT });
    },
    [onChange, overrides],
  );

  const ids = Object.keys(CONFIGURABLE_SHORTCUTS) as ConfigurableShortcutId[];

  return (
    <SettingsCard isDirty={JSON.stringify(overrides) !== JSON.stringify(baselineOverrides)}>
      <CardHeader>
        <CardTitle className="text-base">Keyboard Shortcuts</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="divide-y divide-border">
          {ids.map((id) => (
            <ShortcutRecorder
              key={id}
              shortcutId={id}
              label={CONFIGURABLE_SHORTCUTS[id].label}
              defaultShortcut={CONFIGURABLE_SHORTCUTS[id].default}
              current={shortcuts[id]}
              onChange={handleChange}
              onReset={handleReset}
              onClear={handleClear}
              isDirty={JSON.stringify(shortcuts[id]) !== JSON.stringify(baselineShortcuts[id])}
              conflictsWith={conflictLabels.get(id)}
            />
          ))}
        </div>
        {pluginEntries.length > 0 && (
          <div className="mt-4">
            <p className="text-xs font-medium text-muted-foreground mb-1">Plugin Shortcuts</p>
            <div className="divide-y divide-border">
              {pluginEntries.map((entry) => (
                <ShortcutRecorder
                  key={entry.id}
                  shortcutId={entry.id}
                  label={entry.label}
                  defaultShortcut={entry.default}
                  current={resolveShortcutEntry(entry, overrides)}
                  onChange={handleChange}
                  onReset={handleReset}
                  onClear={handleClear}
                  isDirty={
                    JSON.stringify(resolveShortcutEntry(entry, overrides)) !==
                    JSON.stringify(resolveShortcutEntry(entry, baselineOverrides))
                  }
                  conflictsWith={conflictLabels.get(entry.id)}
                />
              ))}
            </div>
          </div>
        )}
        <p className="text-xs text-muted-foreground mt-3">
          Click a shortcut to record a new key combination.
        </p>
      </CardContent>
    </SettingsCard>
  );
}
