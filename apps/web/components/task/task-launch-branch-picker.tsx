"use client";

import type { ReactNode } from "react";
import { useState } from "react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { MobilePickerSheet } from "./mobile/mobile-picker-sheet";
import { BranchPickerList } from "./branch-picker-list";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { repositoryId, type TaskRepository } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

type TaskLaunchBranchPickerProps = {
  workspaceId: string;
  repositories?: TaskRepository[];
  taskRepositoryId?: string;
  currentBase: string;
  trigger: ReactNode;
  onSelect: (branch: string) => Promise<void>;
};

/** Responsive branch picker used by the task-scoped recovery card. */
export function TaskLaunchBranchPicker({
  workspaceId,
  repositories,
  taskRepositoryId,
  currentBase,
  trigger,
  onSelect,
}: TaskLaunchBranchPickerProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [repository] = repositories?.filter((candidate) => candidate.id === taskRepositoryId) ?? [];
  const source = repository
    ? {
        kind: "id" as const,
        workspaceId,
        repositoryId: repositoryId(repository.repository_id),
      }
    : null;
  const [open, setOpen] = useState(false);
  const { branches, isLoading: isLoadingBranches } = useBranches(source, open);
  const handleSelect = async (branch: string) => {
    setOpen(false);
    await onSelect(branch);
  };

  if (isMobile) {
    return (
      <>
        <span onClick={() => setOpen(true)}>{trigger}</span>
        <MobilePickerSheet
          open={open}
          onOpenChange={setOpen}
          title={t("task:chooseBaseBranch")}
          description={t("task:chooseBaseBranchDescription")}
          contentTestId="task-launch-branch-picker-scroll"
        >
          <div role="listbox" data-testid="task-launch-branch-picker-mobile">
            <BranchPickerList
              branches={branches}
              isLoadingBranches={isLoadingBranches}
              currentBase={currentBase}
              onSelect={(branch) => void handleSelect(branch)}
              remoteOnly
              testIdPrefix="task-launch-branch-picker"
            />
          </div>
        </MobilePickerSheet>
      </>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-64 max-h-72 overflow-auto p-1"
        role="listbox"
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <BranchPickerList
          branches={branches}
          isLoadingBranches={isLoadingBranches}
          currentBase={currentBase}
          onSelect={(branch) => void handleSelect(branch)}
          remoteOnly
          testIdPrefix="task-launch-branch-picker"
        />
      </PopoverContent>
    </Popover>
  );
}
