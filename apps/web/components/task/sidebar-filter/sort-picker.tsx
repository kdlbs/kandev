"use client";

import { IconArrowUp, IconArrowDown } from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Button } from "@kandev/ui/button";
import type { SortKey, SortSpec } from "@/lib/state/slices/ui/sidebar-view-types";
import { useTranslation } from "react-i18next";

// `labelKey` holds a catalog key, not copy: this table is module scope, so a
// resolved `t()` here would freeze at the boot locale. `key` is a persisted
// sort value and stays in English.
const SORT_OPTIONS: Array<{ key: SortKey; labelKey: string }> = [
  { key: "state", labelKey: "task:sortStatus" },
  { key: "updatedAt", labelKey: "task:sortUpdated" },
  { key: "createdAt", labelKey: "task:sortCreated" },
  { key: "title", labelKey: "task:sortTitle" },
  { key: "custom", labelKey: "task:sortCustom" },
];

type Props = {
  value: SortSpec;
  onChange: (next: SortSpec) => void;
};

export function SortPicker({ value, onChange }: Props) {
  const { t } = useTranslation();
  // Direction has no meaning for the custom sort — its order is the user's
  // drag, not an asc/desc field. Hide the toggle to avoid surprising flips.
  const isCustom = value.key === "custom";
  return (
    <div className="flex items-center gap-1.5">
      <Select value={value.key} onValueChange={(v) => onChange({ ...value, key: v as SortKey })}>
        <SelectTrigger size="sm" className="h-7 flex-1 text-xs" data-testid="sort-key-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {SORT_OPTIONS.map((opt) => (
            <SelectItem key={opt.key} value={opt.key} className="text-xs">
              {t(opt.labelKey)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {!isCustom && (
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
          aria-label={t("task:sortDirection", { direction: value.direction })}
        >
          {value.direction === "asc" ? (
            <IconArrowUp className="h-3.5 w-3.5" />
          ) : (
            <IconArrowDown className="h-3.5 w-3.5" />
          )}
        </Button>
      )}
    </div>
  );
}
