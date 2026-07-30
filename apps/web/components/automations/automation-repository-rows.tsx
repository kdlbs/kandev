"use client";

import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { useMemo } from "react";
import type { LocalRepository, Repository } from "@/lib/types/http";
import {
  buildRepositoryItems,
  pickSelectionFromOptionId,
  selectionToOptionId,
  type RepositorySelection,
} from "./automation-repository-selection";

type AutomationRepositoryRowsProps = {
  testId?: string;
  repositories: Repository[];
  discoveredRepos: LocalRepository[];
  selections: RepositorySelection[];
  onChange: (selections: RepositorySelection[]) => void;
  isDirty?: boolean;
};

// AutomationRepositoryRows renders the automation editor's repeatable
// repository picker — one row per selected repository, plus an "Add
// repository" control — shown only when the selected executor profile
// supports launching a task against sibling repositories (see
// getMultiRepoExecutorDisabledReason). Each row excludes the "None" option
// (removing a row is how you clear it) and marks a repository already
// chosen by another row so the same repo can't be picked twice.
export function AutomationRepositoryRows({
  testId,
  repositories,
  discoveredRepos,
  selections,
  onChange,
  isDirty,
}: AutomationRepositoryRowsProps) {
  const baseItems = useMemo(
    () => buildRepositoryItems(repositories, discoveredRepos, { includeNone: false }),
    [repositories, discoveredRepos],
  );
  const usedOptionIds = new Set(selections.map(selectionToOptionId));
  const nextAvailableItem = baseItems.find((item) => !usedOptionIds.has(item.id));

  const updateRow = (index: number, optionId: string) => {
    const picked = pickSelectionFromOptionId(optionId, repositories, discoveredRepos);
    if (picked.kind === "none") return;
    onChange(selections.map((sel, i) => (i === index ? picked : sel)));
  };
  const removeRow = (index: number) => {
    onChange(selections.filter((_, i) => i !== index));
  };
  const addRow = () => {
    if (!nextAvailableItem) return;
    const picked = pickSelectionFromOptionId(nextAvailableItem.id, repositories, discoveredRepos);
    if (picked.kind === "none") return;
    onChange([...selections, picked]);
  };

  return (
    <div className="space-y-1.5" data-testid={testId} data-settings-dirty={isDirty}>
      <Label className="text-xs">Repository</Label>
      <div className="space-y-2">
        {selections.map((selection, index) => (
          <RepositoryRow
            key={selectionToOptionId(selection) + index}
            selection={selection}
            baseItems={baseItems}
            usedOptionIds={usedOptionIds}
            onChange={(optionId) => updateRow(index, optionId)}
            onRemove={() => removeRow(index)}
          />
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          disabled={!nextAvailableItem}
          onClick={addRow}
        >
          <IconPlus className="h-3.5 w-3.5 mr-1" />
          Add repository
        </Button>
      </div>
    </div>
  );
}

function RepositoryRow({
  selection,
  baseItems,
  usedOptionIds,
  onChange,
  onRemove,
}: {
  selection: RepositorySelection;
  baseItems: Array<{ id: string; label: string }>;
  usedOptionIds: Set<string>;
  onChange: (optionId: string) => void;
  onRemove: () => void;
}) {
  const currentOptionId = selectionToOptionId(selection);
  return (
    <div className="flex items-center gap-2">
      <Select value={currentOptionId || undefined} onValueChange={onChange}>
        <SelectTrigger className="cursor-pointer flex-1">
          <SelectValue placeholder="Select a repository" />
        </SelectTrigger>
        <SelectContent>
          {baseItems.map((item) => {
            const alreadyAdded = item.id !== currentOptionId && usedOptionIds.has(item.id);
            return (
              <SelectItem
                key={item.id}
                value={item.id}
                disabled={alreadyAdded}
                title={alreadyAdded ? "Already added" : undefined}
              >
                {item.label}
              </SelectItem>
            );
          })}
        </SelectContent>
      </Select>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="cursor-pointer"
        aria-label="Remove repository"
        onClick={onRemove}
      >
        <IconTrash className="h-3.5 w-3.5 text-destructive" />
      </Button>
    </div>
  );
}
