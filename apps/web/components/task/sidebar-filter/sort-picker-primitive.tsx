"use client";

import { IconArrowDown, IconArrowUp } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";

export type TypedSortSpec<Key extends string> = {
  key: Key;
  direction: "asc" | "desc";
};

export type TypedSortOption<Key extends string> = {
  key: Key;
  label: string;
  description?: string;
};

type Props<Key extends string> = {
  value: TypedSortSpec<Key>;
  options: readonly TypedSortOption<Key>[];
  onChange: (next: TypedSortSpec<Key>) => void;
  directionless?: (key: Key) => boolean;
  directionLabel?: string;
  testIds?: { key: string; direction: string };
  mobile?: boolean;
};

const DEFAULT_TEST_IDS = { key: "sort-key-select", direction: "sort-direction-toggle" };

export function TypedSortPicker<Key extends string>({
  value,
  options,
  onChange,
  directionless,
  directionLabel,
  testIds: customTestIds,
  mobile = false,
}: Props<Key>) {
  const testIds = { ...DEFAULT_TEST_IDS, ...customTestIds };
  const hideDirection = directionless?.(value.key) ?? false;
  return (
    <div
      className={`flex items-center gap-1.5${mobile ? " [&_button]:min-h-11 [&_[role=combobox]]:min-h-11" : ""}`}
    >
      <Select value={value.key} onValueChange={(key) => onChange({ ...value, key: key as Key })}>
        <SelectTrigger size="sm" className="h-7 min-w-0 flex-1 text-xs" data-testid={testIds.key}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem
              key={option.key}
              value={option.key}
              className="text-xs"
              description={option.description}
            >
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {!hideDirection && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={mobile ? "h-11 cursor-pointer" : "h-7 cursor-pointer"}
          onClick={() =>
            onChange({ ...value, direction: value.direction === "asc" ? "desc" : "asc" })
          }
          data-testid={testIds.direction}
          data-direction={value.direction}
          aria-label={directionLabel}
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
