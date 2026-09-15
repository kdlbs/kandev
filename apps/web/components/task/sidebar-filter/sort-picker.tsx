"use client";

import type { SortKey, SortSpec } from "@/lib/state/slices/ui/sidebar-view-types";
import { useTranslation } from "react-i18next";
import { TypedSortPicker } from "./sort-picker-primitive";

// `labelKey` holds a catalog key, not copy: this table is module scope, so a
// resolved `t()` here would freeze at the boot locale. `key` is a persisted
// sort value and stays in English.
const SORT_OPTIONS: Array<{ key: SortKey; labelKey: string; descriptionKey: string }> = [
  {
    key: "state",
    labelKey: "task:sortStatus",
    descriptionKey: "task:sortStatusDescription",
  },
  {
    key: "updatedAt",
    labelKey: "task:sortUpdated",
    descriptionKey: "task:sortUpdatedDescription",
  },
  {
    key: "lastActivityAt",
    labelKey: "task:sortLastActivity",
    descriptionKey: "task:sortLastActivityDescription",
  },
  {
    key: "createdAt",
    labelKey: "task:sortCreated",
    descriptionKey: "task:sortCreatedDescription",
  },
  {
    key: "title",
    labelKey: "task:sortTitle",
    descriptionKey: "task:sortTitleDescription",
  },
  {
    key: "custom",
    labelKey: "task:sortCustom",
    descriptionKey: "task:sortCustomDescription",
  },
];

export function sortKeyLabelKey(key: SortKey): string {
  return SORT_OPTIONS.find((option) => option.key === key)?.labelKey ?? "task:sortStatus";
}

type Props = {
  value: SortSpec;
  onChange: (next: SortSpec) => void;
};

export function SortPicker({ value, onChange }: Props) {
  const { t } = useTranslation();
  return (
    <TypedSortPicker
      value={value}
      options={SORT_OPTIONS.map((option) => ({
        key: option.key,
        label: t(option.labelKey),
        description: t(option.descriptionKey),
      }))}
      onChange={onChange}
      directionless={(key) => key === "custom"}
      directionLabel={t("task:sortDirection", { direction: value.direction })}
    />
  );
}
