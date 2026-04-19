"use client";

import { IconArrowUp, IconArrowDown } from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Button } from "@kandev/ui/button";
import type { SortKey, SortSpec } from "@/lib/state/slices/ui/sidebar-view-types";

const SORT_OPTIONS: Array<{ key: SortKey; label: string }> = [
  { key: "state", label: "State" },
  { key: "updatedAt", label: "Updated" },
  { key: "createdAt", label: "Created" },
  { key: "title", label: "Title" },
  { key: "repository", label: "Repository" },
];

type Props = {
  value: SortSpec;
  onChange: (next: SortSpec) => void;
};

export function SortPicker({ value, onChange }: Props) {
  return (
    <div className="flex items-center gap-1.5">
      <Select value={value.key} onValueChange={(v) => onChange({ ...value, key: v as SortKey })}>
        <SelectTrigger size="sm" className="h-7 flex-1 text-xs" data-testid="sort-key-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {SORT_OPTIONS.map((opt) => (
            <SelectItem key={opt.key} value={opt.key} className="text-xs">
              {opt.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-7 cursor-pointer"
        onClick={() =>
          onChange({ ...value, direction: value.direction === "asc" ? "desc" : "asc" })
        }
        data-testid="sort-direction-toggle"
        data-direction={value.direction}
        aria-label={`Sort direction ${value.direction}`}
      >
        {value.direction === "asc" ? (
          <IconArrowUp className="h-3.5 w-3.5" />
        ) : (
          <IconArrowDown className="h-3.5 w-3.5" />
        )}
      </Button>
    </div>
  );
}
