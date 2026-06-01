"use client";

import { Badge } from "@kandev/ui/badge";
import { levelBadgeClass, statusBadgeClass } from "./sentry-issue-common";
import { LEVEL_OPTIONS, STATUS_OPTIONS } from "./sentry-issue-watch-form";
import type { SentryLevel, SentryStatus } from "@/lib/types/sentry";

// LevelMultiSelect / StatusMultiSelect are toggle-badge pickers: click a badge
// to add/remove it from the selection. A match means ANY selected value.

export function LevelMultiSelect({
  selected,
  onToggle,
}: {
  selected: SentryLevel[];
  onToggle: (level: SentryLevel) => void;
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {LEVEL_OPTIONS.map((level) => {
        const active = selected.includes(level);
        const colorClass = active ? levelBadgeClass(level) : "";
        return (
          <button
            key={level}
            type="button"
            onClick={() => onToggle(level)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge variant="outline" className={`uppercase ${colorClass}`}>
              {level}
            </Badge>
          </button>
        );
      })}
    </div>
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
    <div className="flex flex-wrap gap-1.5">
      {STATUS_OPTIONS.map((status) => {
        const active = selected.includes(status);
        const colorClass = active ? statusBadgeClass(status) : "";
        return (
          <button
            key={status}
            type="button"
            onClick={() => onToggle(status)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge variant="outline" className={`uppercase ${colorClass}`}>
              {status}
            </Badge>
          </button>
        );
      })}
    </div>
  );
}
