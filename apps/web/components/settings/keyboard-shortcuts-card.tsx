"use client";

import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { IconAlertTriangle, IconRotate, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Kbd } from "@kandev/ui/kbd";
import { Input } from "@kandev/ui/input";
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
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  buildIntegrationShortcutEntries,
  isFocusTraversalKey,
  type IntegrationShortcutEntry,
} from "@/lib/keyboard/integration-shortcuts";

type ShortcutRecorderProps = {
  shortcutId: string;
  label: string;
  defaultShortcut: KeyboardShortcut;
  current: KeyboardShortcut;
  onChange: (id: string, shortcut: KeyboardShortcut) => void;
  onReset: (id: string) => void;
  // Optional: callers that don't support an explicit "unbind" (e.g. a single-row
  // settings recorder) omit this, and the Clear button is hidden for them.
  onClear?: (id: string) => void;
  isDirty?: boolean;
  conflictsWith?: string[];
  touchSized?: boolean;
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
  touchSized = false,
}: ShortcutRecorderProps) {
  const statusId = useId();
  const [recording, setRecording] = useState(false);
  const isDefault = JSON.stringify(current) === JSON.stringify(defaultShortcut);
  const isUnbound = isUnboundShortcut(current);
  const defaultIsUnbound = isUnboundShortcut(defaultShortcut);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (!recording) return;
      if (e.target instanceof HTMLElement && e.target.dataset.shortcutFilter === "true") {
        setRecording(false);
        return;
      }
      if (["Control", "Meta", "Alt", "Shift"].includes(e.key)) return;
      if (shortcutId.startsWith("integration:") && isFocusTraversalKey(e)) {
        setRecording(false);
        return;
      }

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
    <div
      className={
        touchSized
          ? "flex flex-col items-stretch gap-2 py-3"
          : "flex items-center justify-between py-2"
      }
    >
      <ShortcutRecorderLabel label={label} conflictsWith={conflictsWith} />
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <button
          data-testid={`shortcut-recorder-${shortcutId}`}
          data-shortcut-recording={recording}
          aria-label={label}
          aria-describedby={statusId}
          data-settings-dirty={isDirty}
          onClick={() => setRecording(!recording)}
          className={`${controlSizingClassName("standard", "min-w-0 px-3 rounded-md border text-sm")}
            ${
              recording
                ? "border-primary bg-primary/10 text-primary"
                : "border-border bg-background hover:bg-accent"
            }`}
        >
          <span id={statusId} role="status" aria-live="polite" aria-atomic="true">
            <RecorderLabel recording={recording} current={current} isUnbound={isUnbound} />
          </span>
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
  const { t } = useTranslation();
  return (
    <span className="min-w-0 break-words text-sm flex items-center gap-1.5">
      {label}
      {conflictsWith && conflictsWith.length > 0 && (
        <span
          className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-500"
          role="img"
          aria-label={t("settings:sameShortcutAs", { shortcuts: conflictsWith.join(", ") })}
          title={t("settings:sameShortcutAs", { shortcuts: conflictsWith.join(", ") })}
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
  const { t } = useTranslation();
  return (
    <>
      {onClear && !isUnbound && !defaultIsUnbound && (
        <Button
          variant="ghost"
          size="icon"
          className={`${controlSizingClassName("icon")} cursor-pointer`}
          onClick={() => onClear(shortcutId)}
          aria-label={t("settings:clearShortcut")}
          title={t("settings:clearShortcut")}
        >
          <IconX className="size-3.5" />
        </Button>
      )}
      {!isDefault && (
        <Button
          variant="ghost"
          size="icon"
          className={`${controlSizingClassName("icon")} cursor-pointer`}
          onClick={() => onReset(shortcutId)}
          aria-label={
            defaultIsUnbound ? t("settings:resetClearShortcut") : t("settings:resetToDefault")
          }
          title={defaultIsUnbound ? t("settings:resetClearShortcut") : t("settings:resetToDefault")}
        >
          <IconRotate className="size-3.5" />
        </Button>
      )}
    </>
  );
}

function RecorderLabel({
  recording,
  current,
  isUnbound,
}: {
  recording: boolean;
  current: KeyboardShortcut;
  isUnbound: boolean;
}) {
  const { t } = useTranslation();
  if (recording) return <span className="animate-pulse">{t("settings:pressAKeyCombo")}</span>;
  if (isUnbound) {
    return <span className="text-muted-foreground italic">{t("settings:unbound")}</span>;
  }
  return <Kbd>{formatShortcut(current)}</Kbd>;
}

/** Builds a `shortcutId -> conflicting labels` lookup from conflict groups. */
export function buildConflictLabels(groups: ShortcutConflictGroup[]): Map<string, string[]> {
  const peersByAction = new Map<string, Map<string, string>>();
  for (const group of groups) {
    for (const entry of group.entries) {
      const peers = peersByAction.get(entry.id) ?? new Map<string, string>();
      for (const other of group.entries) {
        if (other.id !== entry.id && !peers.has(other.id)) peers.set(other.id, other.label);
      }
      peersByAction.set(entry.id, peers);
    }
  }
  return new Map([...peersByAction].map(([id, peers]) => [id, [...new Set(peers.values())]]));
}

const CONFIGURABLE_SHORTCUT_IDS = Object.keys(CONFIGURABLE_SHORTCUTS) as ConfigurableShortcutId[];

export function useShortcutConflictLabels(
  pluginEntries: ShortcutEntry[],
  overrides: StoredShortcutOverrides,
  translate: NonNullable<Parameters<typeof coreShortcutEntries>[0]>,
  integrationEntries: IntegrationShortcutEntry[] = buildIntegrationShortcutEntries(
    [],
    [],
    translate,
  ),
) {
  return useMemo(() => {
    const allEntries = [...coreShortcutEntries(translate), ...integrationEntries, ...pluginEntries];
    const resolved = allEntries.map((entry) => ({
      entry,
      shortcut: resolveShortcutEntry(entry, overrides),
    }));
    return buildConflictLabels(findShortcutConflicts(resolved));
  }, [pluginEntries, integrationEntries, overrides, translate]);
}

export function KeyboardShortcutsCard({
  overrides,
  baselineOverrides = {},
  onChange,
  pluginEntries = [],
  integrationEntries,
}: {
  overrides: StoredShortcutOverrides;
  baselineOverrides?: StoredShortcutOverrides;
  onChange: (overrides: StoredShortcutOverrides) => void;
  /** Dynamic plugin-declared shortcuts (see `lib/keyboard/plugin-shortcuts.ts`). */
  pluginEntries?: ShortcutEntry[];
  integrationEntries?: IntegrationShortcutEntry[];
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const [query, setQuery] = useState("");
  const integrations = integrationEntries ?? buildIntegrationShortcutEntries([], [], t);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const matches = (label: string) => label.toLocaleLowerCase().includes(normalizedQuery);
  const visibleIds = CONFIGURABLE_SHORTCUT_IDS.filter((id) =>
    matches(t(CONFIGURABLE_SHORTCUTS[id].labelKey)),
  );
  const visibleIntegrations = integrations.filter((entry) => matches(entry.label));
  const shortcuts = resolveAllShortcuts(overrides);
  const baselineShortcuts = resolveAllShortcuts(baselineOverrides);

  const conflictLabels = useShortcutConflictLabels(pluginEntries, overrides, t, integrations);

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

  return (
    <SettingsCard
      isDirty={JSON.stringify(overrides) !== JSON.stringify(baselineOverrides)}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.keyboardShortcuts}
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:keyboardShortcuts")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ShortcutFilter query={query} onQueryChange={setQuery} />
        {normalizedQuery && visibleIds.length === 0 && visibleIntegrations.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("settings:noMatchingShortcuts")}</p>
        )}
        <div className="divide-y divide-border">
          {visibleIds.map((id) => (
            <ShortcutRecorder
              key={id}
              shortcutId={id}
              label={t(CONFIGURABLE_SHORTCUTS[id].labelKey)}
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
        {(!normalizedQuery || visibleIntegrations.length > 0) && (
          <IntegrationShortcutRows
            entries={visibleIntegrations}
            overrides={overrides}
            baselineOverrides={baselineOverrides}
            conflictLabels={conflictLabels}
            touchSized={isMobile || !isFinePointer}
            onChange={handleChange}
            onReset={handleReset}
          />
        )}
        <p className="text-xs text-muted-foreground mt-3">
          {t("settings:clickAShortcutToRecordA")}
        </p>
      </CardContent>
    </SettingsCard>
  );
}

function ShortcutFilter({
  query,
  onQueryChange,
}: {
  query: string;
  onQueryChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <div className="mb-3 flex min-w-0 items-center gap-2">
      <Input
        ref={inputRef}
        data-shortcut-filter="true"
        value={query}
        onChange={(event) => onQueryChange(event.target.value)}
        placeholder={t("common:searchEllipsis")}
        aria-label={t("settings:searchShortcuts")}
      />
      {query && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={controlSizingClassName("icon", "shrink-0 cursor-pointer")}
          onClick={() => {
            onQueryChange("");
            inputRef.current?.focus();
          }}
          aria-label={t("tasks:clearSearch")}
          title={t("tasks:clearSearch")}
        >
          <IconX className="size-4" />
        </Button>
      )}
    </div>
  );
}

function IntegrationShortcutRows({
  entries,
  overrides,
  baselineOverrides,
  conflictLabels,
  touchSized,
  onChange,
  onReset,
}: {
  entries: IntegrationShortcutEntry[];
  overrides: StoredShortcutOverrides;
  baselineOverrides: StoredShortcutOverrides;
  conflictLabels: Map<string, string[]>;
  touchSized: boolean;
  onChange: ShortcutRecorderProps["onChange"];
  onReset: ShortcutRecorderProps["onReset"];
}) {
  const { t } = useTranslation();
  return (
    <section
      className="mt-4 min-w-0"
      data-testid="integration-shortcuts-group"
      aria-label={t("settings:integrationShortcuts")}
    >
      <h3 className="text-sm font-medium mb-1">{t("settings:integrationShortcuts")}</h3>
      <div className="min-w-0 divide-y divide-border">
        {entries.map((entry) => (
          <ShortcutRecorder
            key={entry.id}
            shortcutId={entry.id}
            label={entry.label}
            defaultShortcut={entry.default}
            current={resolveShortcutEntry(entry, overrides)}
            onChange={onChange}
            onReset={onReset}
            touchSized={touchSized}
            isDirty={
              JSON.stringify(resolveShortcutEntry(entry, overrides)) !==
              JSON.stringify(resolveShortcutEntry(entry, baselineOverrides))
            }
            conflictsWith={conflictLabels.get(entry.id)}
          />
        ))}
      </div>
    </section>
  );
}
