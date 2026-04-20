"use client";

import { useId, useState } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { CLIFlag } from "@/lib/types/http";

type CLIFlagsFieldProps = {
  flags: CLIFlag[];
  onChange: (next: CLIFlag[]) => void;
  variant?: "default" | "compact";
};

/**
 * CLIFlagsField renders the unified list of user-configurable CLI flags on an
 * agent profile. The list is seeded by the backend at profile creation with
 * the agent's curated suggestions; the user can toggle entries, remove them,
 * or append custom flags. Only entries with `enabled: true` reach the agent
 * subprocess argv at task launch.
 */
export function CLIFlagsField({ flags, onChange, variant = "default" }: CLIFlagsFieldProps) {
  const isCompact = variant === "compact";
  const enabledCount = flags.filter((f) => f.enabled).length;

  const toggleAt = (index: number, enabled: boolean) => {
    onChange(flags.map((f, i) => (i === index ? { ...f, enabled } : f)));
  };
  const removeAt = (index: number) => onChange(flags.filter((_, i) => i !== index));
  const appendCustom = (flag: string, description: string) =>
    onChange([...flags, { flag, description, enabled: true }]);

  return (
    <div className={isCompact ? "space-y-2" : "space-y-3"} data-testid="cli-flags-field">
      <CLIFlagsHeader total={flags.length} enabled={enabledCount} compact={isCompact} />
      {flags.length === 0 ? (
        <p className="text-xs italic text-muted-foreground" data-testid="cli-flags-empty">
          No CLI flags configured. Add one below.
        </p>
      ) : (
        <ul className="space-y-2" data-testid="cli-flags-list">
          {flags.map((flag, index) => (
            <CLIFlagRow
              key={`${flag.flag}-${index}`}
              flag={flag}
              index={index}
              onToggle={toggleAt}
              onRemove={removeAt}
            />
          ))}
        </ul>
      )}
      <CLIFlagsAddForm onAdd={appendCustom} />
    </div>
  );
}

function CLIFlagsHeader({
  total,
  enabled,
  compact,
}: {
  total: number;
  enabled: number;
  compact: boolean;
}) {
  return (
    <>
      <div className="flex items-center justify-between">
        <Label className={compact ? "text-xs" : undefined}>Agent CLI flags</Label>
        {total > 0 && (
          <span className="text-[10px] text-muted-foreground" data-testid="cli-flags-count">
            {enabled} of {total} enabled
          </span>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        Flags passed to the agent CLI on launch. Only enabled entries are applied. Use quotes for
        values with spaces, e.g.{" "}
        <code className="bg-muted px-1 rounded">{`--msg "hello world"`}</code>.
      </p>
    </>
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
