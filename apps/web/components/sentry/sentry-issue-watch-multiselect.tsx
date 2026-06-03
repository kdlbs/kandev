"use client";

import { Badge } from "@kandev/ui/badge";
import { levelBadgeClass, statusBadgeClass } from "./sentry-issue-common";
import { LEVEL_OPTIONS, STATUS_OPTIONS } from "./sentry-issue-watch-form";
import type { SentryLevel, SentryStatus } from "@/lib/types/sentry";

// LevelMultiSelect / StatusMultiSelect are toggle-badge pickers: click a badge
// to add/remove it from the selection. A match means ANY selected value. Both
// are thin wrappers over the generic BadgeMultiSelect below.

// BadgeMultiSelect renders a toggle-badge picker for any string-union option
// set. `colorClass` resolves the active-state styling for a given option.
function BadgeMultiSelect<T extends string>({
  options,
  selected,
  onToggle,
  colorClass,
}: {
  options: readonly T[];
  selected: T[];
  onToggle: (value: T) => void;
  colorClass: (value: T) => string;
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {options.map((value) => {
        const active = selected.includes(value);
        return (
          <button
            key={value}
            type="button"
            onClick={() => onToggle(value)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge variant="outline" className={`uppercase ${active ? colorClass(value) : ""}`}>
              {value}
            </Badge>
          </button>
        );
      })}
    </div>
  );
}

export function LevelMultiSelect({
  selected,
  onToggle,
}: {
  selected: SentryLevel[];
  onToggle: (level: SentryLevel) => void;
}) {
  return (
    <BadgeMultiSelect
      options={LEVEL_OPTIONS}
      selected={selected}
      onToggle={onToggle}
      colorClass={levelBadgeClass}
    />
  );
}

export function StatusMultiSelect({
  selected,
  onToggle,
}: {
  selected: SentryStatus[];
  onToggle: (status: SentryStatus) => void;
}) {
  return (
    <BadgeMultiSelect
      options={STATUS_OPTIONS}
      selected={selected}
      onToggle={onToggle}
      colorClass={statusBadgeClass}
    />
  );
}
