"use client";

import { useCallback, useEffect, useState } from "react";
import { IconRotate, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Kbd } from "@kandev/ui/kbd";
import type { Key, KeyboardShortcut } from "@/lib/keyboard/constants";
import { formatShortcut } from "@/lib/keyboard/utils";
import {
  CONFIGURABLE_SHORTCUTS,
  UNBOUND_SHORTCUT,
  isUnboundShortcut,
  resolveAllShortcuts,
  type ConfigurableShortcutId,
  type StoredShortcutOverrides,
} from "@/lib/keyboard/shortcut-overrides";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { updateUserSettings } from "@/lib/api/domains/settings-api";

type ShortcutRecorderProps = {
  shortcutId: ConfigurableShortcutId;
  current: KeyboardShortcut;
  onChange: (id: ConfigurableShortcutId, shortcut: KeyboardShortcut) => void;
  onReset: (id: ConfigurableShortcutId) => void;
  onClear: (id: ConfigurableShortcutId) => void;
};

function ShortcutRecorder({
  shortcutId,
  current,
  onChange,
  onReset,
  onClear,
}: ShortcutRecorderProps) {
  const [recording, setRecording] = useState(false);
  const defaultShortcut = CONFIGURABLE_SHORTCUTS[shortcutId].default;
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
      <span className="text-sm">{CONFIGURABLE_SHORTCUTS[shortcutId].label}</span>
      <div className="flex items-center gap-2">
        <button
          data-testid={`shortcut-recorder-${shortcutId}`}
          onClick={() => setRecording(!recording)}
          className={`px-3 py-1.5 rounded-md border text-sm cursor-pointer transition-colors ${
            recording
              ? "border-primary bg-primary/10 text-primary"
              : "border-border bg-background hover:bg-accent"
          }`}
        >
          {renderRecorderLabel({ recording, current, isUnbound })}
        </button>
        {!isUnbound && !defaultIsUnbound && (
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 cursor-pointer"
            onClick={() => onClear(shortcutId)}
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
            title={defaultIsUnbound ? "Reset (clear shortcut)" : "Reset to default"}
          >
            <IconRotate className="size-3.5" />
          </Button>
        )}
      </div>
    </div>
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

export function KeyboardShortcutsCard() {
  const storeOverrides = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const setUserSettings = useAppStore((s) => s.setUserSettings);
  const userSettings = useAppStore((s) => s.userSettings);
  const shortcuts = resolveAllShortcuts(storeOverrides);
  const { toast } = useToast();

  const persistOverrides = useCallback(
    (overrides: StoredShortcutOverrides) => {
      const previous = userSettings.keyboardShortcuts;
      setUserSettings({ ...userSettings, keyboardShortcuts: overrides });
      updateUserSettings({ keyboard_shortcuts: overrides }).catch(() => {
        setUserSettings({ ...userSettings, keyboardShortcuts: previous });
        toast({ title: "Failed to save shortcut", variant: "error" });
      });
    },
    [userSettings, setUserSettings, toast],
  );

  const handleChange = useCallback(
    (id: ConfigurableShortcutId, shortcut: KeyboardShortcut) => {
      const next = { ...storeOverrides, [id]: shortcut };
      persistOverrides(next);
    },
    [storeOverrides, persistOverrides],
  );

  const handleReset = useCallback(
    (id: ConfigurableShortcutId) => {
      const next = { ...storeOverrides };
      delete next[id];
      persistOverrides(next);
    },
    [storeOverrides, persistOverrides],
  );

  const handleClear = useCallback(
    (id: ConfigurableShortcutId) => {
      const next = { ...storeOverrides, [id]: UNBOUND_SHORTCUT };
      persistOverrides(next);
    },
    [storeOverrides, persistOverrides],
  );

  const ids = Object.keys(CONFIGURABLE_SHORTCUTS) as ConfigurableShortcutId[];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Keyboard Shortcuts</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="divide-y divide-border">
          {ids.map((id) => (
            <ShortcutRecorder
              key={id}
              shortcutId={id}
              current={shortcuts[id]}
              onChange={handleChange}
              onReset={handleReset}
              onClear={handleClear}
            />
          ))}
        </div>
        <p className="text-xs text-muted-foreground mt-3">
          Click a shortcut to record a new key combination. Changes are saved automatically.
        </p>
      </CardContent>
    </Card>
  );
}
