"use client";

import { useCallback, useEffect, useState } from "react";
import { IconRotate } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Kbd } from "@kandev/ui/kbd";
import type { Key, KeyboardShortcut } from "@/lib/keyboard/constants";
import { formatShortcut } from "@/lib/keyboard/utils";
import {
  CONFIGURABLE_SHORTCUTS,
  resolveAllShortcuts,
  type ConfigurableShortcutId,
  type StoredShortcutOverrides,
} from "@/lib/keyboard/shortcut-overrides";
import { useAppStore } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api/domains/settings-api";

function ShortcutRecorder({
  shortcutId,
  current,
  onChange,
  onReset,
}: {
  shortcutId: ConfigurableShortcutId;
  current: KeyboardShortcut;
  onChange: (id: ConfigurableShortcutId, shortcut: KeyboardShortcut) => void;
  onReset: (id: ConfigurableShortcutId) => void;
}) {
  const [recording, setRecording] = useState(false);
  const isDefault =
    JSON.stringify(current) === JSON.stringify(CONFIGURABLE_SHORTCUTS[shortcutId].default);

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
          {recording ? (
            <span className="animate-pulse">Press a key combo...</span>
          ) : (
            <Kbd>{formatShortcut(current)}</Kbd>
          )}
        </button>
        {!isDefault && (
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 cursor-pointer"
            onClick={() => onReset(shortcutId)}
            title="Reset to default"
          >
            <IconRotate className="size-3.5" />
          </Button>
        )}
      </div>
    </div>
  );
}

export function KeyboardShortcutsCard() {
  const storeOverrides = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const setUserSettings = useAppStore((s) => s.setUserSettings);
  const userSettings = useAppStore((s) => s.userSettings);
  const shortcuts = resolveAllShortcuts(storeOverrides);

  const persistOverrides = useCallback(
    (overrides: StoredShortcutOverrides) => {
      setUserSettings({ ...userSettings, keyboardShortcuts: overrides });
      updateUserSettings({ keyboard_shortcuts: overrides }).catch(() => {});
    },
    [userSettings, setUserSettings],
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
