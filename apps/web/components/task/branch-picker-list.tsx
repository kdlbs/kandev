"use client";

import { useMemo, useState } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { prioritizeSelectedOption } from "@/lib/utils/selector-options";
import type { Branch } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

type BranchPickerListProps = {
  branches: Branch[];
  isLoadingBranches: boolean;
  currentBase: string;
  onSelect: (name: string) => void;
  remoteOnly?: boolean;
  testIdPrefix?: string;
};

/** Shared, bounded branch list used by desktop popovers and phone sheets. */
export function BranchPickerList({
  branches,
  isLoadingBranches,
  currentBase,
  onSelect,
  remoteOnly = false,
  testIdPrefix = "base-branch-picker",
}: BranchPickerListProps) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState("");
  const uniqueByName = useMemo(() => {
    const seen = new Set<string>();
    return branches.filter((branch) => {
      if (remoteOnly && branch.type !== "remote") return false;
      if (seen.has(branch.name)) return false;
      seen.add(branch.name);
      return true;
    });
  }, [branches, remoteOnly]);
  const filtered = useMemo(() => {
    const query = filter.trim().toLowerCase();
    return query
      ? uniqueByName.filter((branch) => branch.name.toLowerCase().includes(query))
      : uniqueByName;
  }, [filter, uniqueByName]);
  const orderedBranches = useMemo(() => {
    if (filter.trim()) return filtered;
    return prioritizeSelectedOption(filtered, currentBase, (branch) => branch.name);
  }, [currentBase, filter, filtered]);

  return (
    <div className="flex min-w-0 flex-col gap-1">
      <input
        type="text"
        value={filter}
        onChange={(event) => setFilter(event.target.value)}
        placeholder={t("task:filterBranches")}
        data-testid={`${testIdPrefix}-filter`}
        className="min-h-11 w-full rounded border border-input bg-background px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring sm:min-h-8"
        autoFocus
      />
      <BranchPickerListBody
        branches={orderedBranches}
        isLoadingBranches={isLoadingBranches}
        currentBase={currentBase}
        onSelect={onSelect}
        testIdPrefix={testIdPrefix}
      />
    </div>
  );
}

function BranchPickerListBody({
  branches,
  isLoadingBranches,
  currentBase,
  onSelect,
  testIdPrefix,
}: {
  branches: Branch[];
  isLoadingBranches: boolean;
  currentBase: string;
  onSelect: (name: string) => void;
  testIdPrefix: string;
}) {
  const { t } = useTranslation();
  if (isLoadingBranches && branches.length === 0) {
    return (
      <div className="flex min-h-11 items-center gap-2 px-2 py-1.5 text-xs text-muted-foreground sm:min-h-8">
        <IconLoader2 className="h-3 w-3 animate-spin" />
        {t("task:loadingBranches")}
      </div>
    );
  }
  if (branches.length === 0) {
    return (
      <div className="px-2 py-1.5 text-xs text-muted-foreground">
        {t("task:noMatchingBranches")}
      </div>
    );
  }
  return (
    <>
      {branches.map((branch) => (
        <button
          key={`${branch.type}:${branch.name}`}
          type="button"
          role="option"
          aria-selected={branch.name === currentBase}
          data-testid={`${testIdPrefix}-option-${branch.name}`}
          className={cn(
            "flex min-h-11 w-full items-center gap-2 rounded border border-transparent px-2 py-1.5 text-left text-xs cursor-pointer hover:bg-muted sm:min-h-8",
            branch.name === currentBase &&
              "border-primary/50 bg-card font-medium ring-1 ring-inset ring-primary/20",
          )}
          onClick={() => onSelect(branch.name)}
        >
          <span className="truncate">{branch.name}</span>
        </button>
      ))}
    </>
  );
}
