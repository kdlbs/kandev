"use client";

import { useId, useMemo, useState } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { CLIFlag, PermissionSetting } from "@/lib/types/http";

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
    if (!enabled) return; // nothing to turn off
    onChange([...flags, { flag: setting.flag, description: setting.description, enabled: true }]);
  };

  const toggleAt = (index: number, enabled: boolean) => {
    onChange(flags.map((f, i) => (i === index ? { ...f, enabled } : f)));
  };
  const removeAt = (index: number) => onChange(flags.filter((_, i) => i !== index));
  const appendCustom = (flag: string, description: string) =>
    onChange([...flags, { flag, description, enabled: true }]);

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
          onToggle={toggleAt}
          onRemove={removeAt}
          onAdd={appendCustom}
          compact={isCompact}
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
  onToggle,
  onRemove,
  onAdd,
  compact,
}: {
  customFlags: Array<{ flag: CLIFlag; index: number }>;
  onToggle: (index: number, enabled: boolean) => void;
  onRemove: (index: number) => void;
  onAdd: (flag: string, description: string) => void;
  compact: boolean;
}) {
  const labelCls = compact ? "text-xs" : undefined;
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label className={labelCls}>Agent CLI flags</Label>
        {customFlags.length > 0 && (
          <span className="text-[10px] text-muted-foreground" data-testid="cli-flags-count">
            {customFlags.filter((e) => e.flag.enabled).length} of {customFlags.length} enabled
          </span>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        Flags passed to the agent CLI on launch. Only enabled entries are applied. Use quotes for
        values with spaces, e.g.{" "}
        <code className="bg-muted px-1 rounded">{`--msg "hello world"`}</code>.
      </p>

      {customFlags.length === 0 ? (
        <p className="text-xs italic text-muted-foreground" data-testid="cli-flags-empty">
          No CLI flags configured. Add one below.
        </p>
      ) : (
        <ul className="space-y-2" data-testid="cli-flags-list">
          {customFlags.map(({ flag, index }) => (
            <CLIFlagRow
              key={`${flag.flag}-${index}`}
              flag={flag}
              index={index}
              onToggle={onToggle}
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
  onToggle,
  onRemove,
}: {
  flag: CLIFlag;
  index: number;
  onToggle: (i: number, enabled: boolean) => void;
  onRemove: (i: number) => void;
}) {
  return (
    <li
      className="flex items-start justify-between gap-3 rounded-md border p-3"
      data-testid={`cli-flag-row-${index}`}
    >
      <div className="flex-1 min-w-0 space-y-1">
        <code className="text-sm font-semibold break-all" data-testid={`cli-flag-text-${index}`}>
          {flag.flag}
        </code>
        {flag.description && <p className="text-xs text-muted-foreground">{flag.description}</p>}
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <Switch
          checked={flag.enabled}
          onCheckedChange={(checked) => onToggle(index, checked)}
          data-testid={`cli-flag-enabled-${index}`}
          aria-label={`${flag.enabled ? "Disable" : "Enable"} ${flag.flag}`}
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={() => onRemove(index)}
          className="cursor-pointer"
          data-testid={`cli-flag-remove-${index}`}
          aria-label={`Remove ${flag.flag}`}
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </div>
    </li>
  );
}

function CLIFlagsAddForm({ onAdd }: { onAdd: (flag: string, description: string) => void }) {
  const uid = useId();
  const flagId = `${uid}-flag`;
  const descId = `${uid}-desc`;
  const [newFlag, setNewFlag] = useState("");
  const [newDesc, setNewDesc] = useState("");
  const commit = () => {
    const trimmed = newFlag.trim();
    if (trimmed === "") return;
    onAdd(trimmed, newDesc.trim());
    setNewFlag("");
    setNewDesc("");
  };
  const onEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && newFlag.trim() !== "") {
      e.preventDefault();
      commit();
    }
  };
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
      <div className="flex-1 space-y-1">
        <Label className="text-xs" htmlFor={flagId}>
          Flag
        </Label>
        <Input
          id={flagId}
          value={newFlag}
          onChange={(e) => setNewFlag(e.target.value)}
          placeholder="--my-flag or --key=value"
          data-testid="cli-flag-new-flag-input"
          onKeyDown={onEnter}
        />
      </div>
      <div className="flex-1 space-y-1">
        <Label className="text-xs" htmlFor={descId}>
          Description (optional)
        </Label>
        <Input
          id={descId}
          value={newDesc}
          onChange={(e) => setNewDesc(e.target.value)}
          placeholder="What this flag does"
          data-testid="cli-flag-new-desc-input"
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
        <IconPlus className="h-4 w-4 mr-1" />
        Add
      </Button>
    </div>
  );
}
