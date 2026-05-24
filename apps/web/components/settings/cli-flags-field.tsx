"use client";

import { useId, useMemo, useState } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { CLIFlag, PermissionSetting } from "@/lib/types/http";

/**
 * Editor-side representation of a single custom CLI flag. The persisted
 * `CLIFlag.flag` string is shell-tokenised by the backend, so a single
 * stored value like `--add-dir /shared` becomes two argv tokens. We
 * surface that split as separate Flag + Value inputs so the user doesn't
 * have to think about quoting, and re-concatenate on save. Description
 * is preserved through the round-trip but no longer shown in the UI.
 */
type CustomFlagRow = {
  flag: string;
  value: string;
  description: string;
  enabled: boolean;
};

function flagToRow(f: CLIFlag): CustomFlagRow {
  const trimmed = f.flag.trim();
  const ws = trimmed.search(/\s/);
  if (ws === -1) {
    return { flag: trimmed, value: "", description: f.description, enabled: f.enabled };
  }
  return {
    flag: trimmed.slice(0, ws),
    value: trimmed.slice(ws + 1).trim(),
    description: f.description,
    enabled: f.enabled,
  };
}

function rowToFlag(r: CustomFlagRow): CLIFlag {
  const flagText = r.flag.trim();
  const valueText = r.value.trim();
  return {
    flag: valueText ? `${flagText} ${valueText}` : flagText,
    description: r.description,
    enabled: r.enabled,
  };
}

type CLIFlagsFieldProps = {
  flags: CLIFlag[];
  onChange: (next: CLIFlag[]) => void;
  /**
   * The agent's curated PermissionSettings catalogue. Entries with
   * apply_method === "cli_flag" render as labelled predefined toggles
   * (one per setting) above the custom-flags list. The switch state is
   * read from `flags` by matching `setting.cli_flag` to `flag.flag`.
   */
  permissionSettings?: Record<string, PermissionSetting>;
  variant?: "default" | "compact";
  /**
   * When true, the custom-flag list + Add form is hidden. Used by the
   * onboarding flow, where users should only see the agent's curated
   * predefined toggles and defer advanced edits to the profile page.
   */
  hideCustomFlags?: boolean;
};

type CuratedSetting = {
  key: string;
  label: string;
  description: string;
  flag: string;
  default: boolean;
};

/**
 * CLIFlagsField renders two distinct controls backed by the same
 * `profile.cli_flags` list:
 *
 * 1. Predefined toggles — one Switch per entry in the agent's curated
 *    PermissionSettings catalogue. Styled like the CLI Passthrough
 *    toggle. State is read/written into cli_flags by flag-text match.
 * 2. Custom flags — a list of user-authored entries (anything in
 *    cli_flags whose flag text does not match a curated setting),
 *    with an Add form for new entries.
 *
 * Only entries with `enabled: true` reach the agent subprocess argv at
 * launch.
 */
export function CLIFlagsField({
  flags,
  onChange,
  permissionSettings,
  variant = "default",
  hideCustomFlags = false,
}: CLIFlagsFieldProps) {
  const isCompact = variant === "compact";

  const curated = useMemo(() => extractCuratedSettings(permissionSettings), [permissionSettings]);
  const curatedFlagTexts = useMemo(() => new Set(curated.map((s) => s.flag)), [curated]);
  const customFlags = useMemo(
    () =>
      flags
        .map((f, i) => ({ flag: f, index: i }))
        .filter((e) => !curatedFlagTexts.has(e.flag.flag)),
    [flags, curatedFlagTexts],
  );

  const toggleCurated = (setting: CuratedSetting, enabled: boolean) => {
    const existingIdx = flags.findIndex((f) => f.flag === setting.flag);
    if (existingIdx >= 0) {
      onChange(flags.map((f, i) => (i === existingIdx ? { ...f, enabled } : f)));
      return;
    }
    // No existing entry: append an explicit record. The displayed switch
    // state derives from setting.default when absent, so we must persist
    // the user's choice (even when turning off) to actually flip it.
    onChange([...flags, { flag: setting.flag, description: setting.description, enabled }]);
  };

  const updateRowAt = (index: number, next: CLIFlag) => {
    onChange(flags.map((f, i) => (i === index ? next : f)));
  };
  const removeAt = (index: number) => onChange(flags.filter((_, i) => i !== index));
  const appendCustom = (next: CLIFlag) => onChange([...flags, next]);

  return (
    <div className={isCompact ? "space-y-3" : "space-y-4"} data-testid="cli-flags-field">
      {curated.length > 0 && (
        <CuratedFlagsSection
          curated={curated}
          flags={flags}
          onToggle={toggleCurated}
          compact={isCompact}
        />
      )}
      {!hideCustomFlags && (
        <CustomFlagsSection
          customFlags={customFlags}
          onUpdateRow={updateRowAt}
          onRemove={removeAt}
          onAdd={appendCustom}
        />
      )}
    </div>
  );
}

function extractCuratedSettings(
  permissionSettings?: Record<string, PermissionSetting>,
): CuratedSetting[] {
  if (!permissionSettings) return [];
  const out: CuratedSetting[] = [];
  for (const [key, s] of Object.entries(permissionSettings)) {
    if (!s.supported || s.apply_method !== "cli_flag" || !s.cli_flag) continue;
    out.push({
      key,
      label: s.label,
      description: s.description,
      flag: s.cli_flag,
      default: s.default,
    });
  }
  // Stable order by flag text so the UI doesn't reshuffle across renders.
  out.sort((a, b) => a.flag.localeCompare(b.flag));
  return out;
}

function CuratedFlagsSection({
  curated,
  flags,
  onToggle,
  compact,
}: {
  curated: CuratedSetting[];
  flags: CLIFlag[];
  onToggle: (setting: CuratedSetting, enabled: boolean) => void;
  compact: boolean;
}) {
  const labelCls = compact ? "text-xs" : undefined;
  const switchSize = compact ? ("sm" as const) : ("default" as const);
  return (
    <div className="space-y-2" data-testid="cli-flags-curated">
      {curated.map((setting) => {
        const entry = flags.find((f) => f.flag === setting.flag);
        const enabled = entry ? entry.enabled : setting.default;
        return (
          <div
            key={setting.key}
            className="flex items-center justify-between gap-3 rounded-md border p-3"
            data-testid={`cli-flag-curated-${setting.key}`}
          >
            <div className="flex-1 min-w-0 space-y-0.5">
              <Label className={labelCls}>{setting.label}</Label>
              <p className="text-xs text-muted-foreground">{setting.description}</p>
              <code className="text-[10px] text-muted-foreground/80">{setting.flag}</code>
            </div>
            <Switch
              size={switchSize}
              checked={enabled}
              onCheckedChange={(checked) => onToggle(setting, checked)}
              data-testid={`cli-flag-curated-enabled-${setting.key}`}
              aria-label={`${enabled ? "Disable" : "Enable"} ${setting.label}`}
            />
          </div>
        );
      })}
    </div>
  );
}

function CustomFlagsSection({
  customFlags,
  onUpdateRow,
  onRemove,
  onAdd,
}: {
  customFlags: Array<{ flag: CLIFlag; index: number }>;
  onUpdateRow: (index: number, next: CLIFlag) => void;
  onRemove: (index: number) => void;
  onAdd: (next: CLIFlag) => void;
}) {
  return (
    <div className="space-y-3">
      {customFlags.length === 0 ? (
        <p className="text-xs italic text-muted-foreground" data-testid="cli-flags-empty">
          No CLI flags configured. Add one below.
        </p>
      ) : (
        <ul className="space-y-2" data-testid="cli-flags-list">
          {customFlags.map(({ flag, index }) => (
            <CLIFlagRow
              key={index}
              flag={flag}
              index={index}
              onUpdateRow={onUpdateRow}
              onRemove={onRemove}
            />
          ))}
        </ul>
      )}
      <CLIFlagsAddForm onAdd={onAdd} />
    </div>
  );
}

function CLIFlagRow({
  flag,
  index,
  onUpdateRow,
  onRemove,
}: {
  flag: CLIFlag;
  index: number;
  onUpdateRow: (index: number, next: CLIFlag) => void;
  onRemove: (index: number) => void;
}) {
  const row = flagToRow(flag);
  const update = (patch: Partial<CustomFlagRow>) => {
    onUpdateRow(index, rowToFlag({ ...row, ...patch }));
  };
  return (
    <li className="flex items-center gap-2" data-testid={`cli-flag-row-${index}`}>
      <Input
        value={row.flag}
        onChange={(e) => update({ flag: e.target.value })}
        placeholder="--my-flag"
        className="flex-[2] font-mono text-xs"
        data-testid={`cli-flag-flag-${index}`}
      />
      <Input
        value={row.value}
        onChange={(e) => update({ value: e.target.value })}
        placeholder="value (optional)"
        className="flex-[3] font-mono text-xs"
        data-testid={`cli-flag-value-${index}`}
      />
      <Switch
        checked={row.enabled}
        onCheckedChange={(checked) => update({ enabled: checked })}
        data-testid={`cli-flag-enabled-${index}`}
        aria-label={`${row.enabled ? "Disable" : "Enable"} ${row.flag}`}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={() => onRemove(index)}
        className="h-8 w-8 shrink-0 cursor-pointer"
        data-testid={`cli-flag-remove-${index}`}
        aria-label={`Remove ${row.flag || "flag"}`}
      >
        <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
      </Button>
    </li>
  );
}

function CLIFlagsAddForm({ onAdd }: { onAdd: (next: CLIFlag) => void }) {
  const uid = useId();
  const flagId = `${uid}-flag`;
  const valueId = `${uid}-value`;
  const [newFlag, setNewFlag] = useState("");
  const [newValue, setNewValue] = useState("");
  const commit = () => {
    const trimmed = newFlag.trim();
    if (trimmed === "") return;
    onAdd(rowToFlag({ flag: trimmed, value: newValue, description: "", enabled: true }));
    setNewFlag("");
    setNewValue("");
  };
  const onEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && newFlag.trim() !== "") {
      e.preventDefault();
      commit();
    }
  };
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
      <div className="flex-[2] space-y-1">
        <Label className="text-xs" htmlFor={flagId}>
          Flag
        </Label>
        <Input
          id={flagId}
          value={newFlag}
          onChange={(e) => setNewFlag(e.target.value)}
          placeholder="--my-flag"
          className="font-mono text-xs"
          data-testid="cli-flag-new-flag-input"
          onKeyDown={onEnter}
        />
      </div>
      <div className="flex-[3] space-y-1">
        <Label className="text-xs" htmlFor={valueId}>
          Value (optional)
        </Label>
        <Input
          id={valueId}
          value={newValue}
          onChange={(e) => setNewValue(e.target.value)}
          placeholder="value"
          className="font-mono text-xs"
          data-testid="cli-flag-new-value-input"
          onKeyDown={onEnter}
        />
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={commit}
        disabled={newFlag.trim() === ""}
        className="cursor-pointer"
        data-testid="cli-flag-add-button"
      >
        <IconPlus className="h-3.5 w-3.5 mr-1" />
        Add
      </Button>
    </div>
  );
}

type CustomCLIFlagsCardProps = {
  flags: CLIFlag[];
  onChange: (next: CLIFlag[]) => void;
  permissionSettings?: Record<string, PermissionSetting>;
};

/**
 * Standalone card surfacing only the user-authored (custom) CLI flags,
 * with their values exposed as a dedicated input. Curated flags
 * (Switch toggles derived from PermissionSettings) stay inline within
 * ProfileFormFields so they sit alongside the related profile fields.
 */
export function CustomCLIFlagsCard({
  flags,
  onChange,
  permissionSettings,
}: CustomCLIFlagsCardProps) {
  const curatedFlagTexts = useMemo(
    () => new Set(extractCuratedSettings(permissionSettings).map((s) => s.flag)),
    [permissionSettings],
  );
  const customFlags = useMemo(
    () =>
      flags
        .map((f, i) => ({ flag: f, index: i }))
        .filter((e) => !curatedFlagTexts.has(e.flag.flag)),
    [flags, curatedFlagTexts],
  );
  const enabledCount = customFlags.filter((e) => e.flag.enabled).length;

  const onUpdateRow = (index: number, next: CLIFlag) => {
    onChange(flags.map((f, i) => (i === index ? next : f)));
  };
  const onRemove = (index: number) => onChange(flags.filter((_, i) => i !== index));
  const onAdd = (next: CLIFlag) => onChange([...flags, next]);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div>
            <CardTitle>Agent CLI flags</CardTitle>
            <CardDescription>
              Flags passed to the agent CLI on launch. Only enabled entries are applied.
            </CardDescription>
          </div>
          {customFlags.length > 0 && (
            <span className="text-[10px] text-muted-foreground" data-testid="cli-flags-count">
              {enabledCount} of {customFlags.length} enabled
            </span>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <CustomFlagsSection
          customFlags={customFlags}
          onUpdateRow={onUpdateRow}
          onRemove={onRemove}
          onAdd={onAdd}
        />
      </CardContent>
    </Card>
  );
}
