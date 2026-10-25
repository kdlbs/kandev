"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconGitBranch, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { BranchRefreshButton } from "@/components/branch-refresh-button";
import { useBranches, type BranchSource } from "@/hooks/domains/workspace/use-repository-branches";
import { useRepoBranchAutoselect } from "@/components/task-create-dialog-repo-branch-autoselect";
import { branchToOption, sortBranches } from "@/components/branch-picker-options";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import type { QuickChatSetupRepositoryState } from "./quick-chat-setup-fields";

function useChipBranches(
  row: TaskRepoRow,
  workspaceId: string,
  handleBranchChange: QuickChatSetupRepositoryState["handleBranchChange"],
) {
  const source = useMemo<BranchSource | null>(
    () => (row.repositoryId ? { kind: "id", workspaceId, repositoryId: row.repositoryId } : null),
    [workspaceId, row.repositoryId],
  );
  const { branches, isLoading, refresh } = useBranches(source, !!source);
  const onBranchChange = useCallback(
    (branch: string) => handleBranchChange(row.key, branch),
    [handleBranchChange, row.key],
  );
  useRepoBranchAutoselect({
    branchSource: source,
    branchesLoading: isLoading,
    branches,
    rowBranch: row.branch,
    onBranchChange,
  });
  return { branches, isLoading, refresh, onBranchChange };
}

export function QuickChatMobileRepositoryChip({
  row,
  workspaceId,
  repositories,
}: {
  row: TaskRepoRow;
  workspaceId: string;
  repositories: QuickChatSetupRepositoryState;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const { branches, isLoading, refresh, onBranchChange } = useChipBranches(
    row,
    workspaceId,
    repositories.handleBranchChange,
  );
  const name =
    repositories.repositories.find((repo) => repo.id === row.repositoryId)?.name ??
    t("task:repository");
  return (
    <>
      <span
        className="inline-flex max-w-full items-center rounded-md border border-input bg-input/20 pl-2"
        data-testid="repo-chip"
        data-repository-id={row.repositoryId}
      >
        <span className="max-w-24 truncate text-xs" title={name}>
          {name}
        </span>
        <Button
          ref={triggerRef}
          type="button"
          variant="ghost"
          className="h-11 min-w-11 min-h-11 max-w-44 gap-1 px-2 text-xs"
          aria-haspopup="dialog"
          aria-expanded={open}
          onClick={() => setOpen(true)}
          data-testid="branch-chip-trigger"
        >
          <IconGitBranch className="h-3 w-3 shrink-0" aria-hidden />
          <span className="truncate">{row.branch || t("task:branch")}</span>
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-11 w-11"
          aria-label={t("task:removeRepository")}
          data-testid="remove-repo-chip"
          onClick={() => repositories.removeRepository(row.key)}
        >
          <IconX className="h-4 w-4" aria-hidden />
        </Button>
      </span>
      <MobilePickerSheet
        open={open}
        onOpenChange={setOpen}
        title={t("task:branch")}
        contentTestId="quick-chat-branch-picker"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus();
        }}
      >
        <Command>
          <div className="flex items-center gap-1 pr-2">
            <CommandInput placeholder={t("task:searchBranches")} />
            {refresh && (
              <BranchRefreshButton onRefresh={refresh} refreshing={isLoading} touchTarget />
            )}
          </div>
          <CommandList>
            <CommandEmpty>{t("task:noBranches")}</CommandEmpty>
            <CommandGroup>
              {sortBranches(branches)
                .map(branchToOption)
                .map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    keywords={option.keywords}
                    className="min-h-12"
                    onSelect={() => {
                      onBranchChange(option.value);
                      setOpen(false);
                    }}
                  >
                    {option.label}
                  </CommandItem>
                ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </MobilePickerSheet>
    </>
  );
}
