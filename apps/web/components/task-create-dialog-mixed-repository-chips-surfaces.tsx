"use client";

import { useState } from "react";
import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import {
  DesktopWorkspaceSourceMenu,
  WorkspaceSourceMenuContent,
  type WorkspaceSourceMenuProps,
  type WorkspaceSourceMenuView,
} from "@/components/task-create-dialog-workspace-source-menu";
import { useTranslation } from "react-i18next";

type MobileMixedRepositoryChipsProps = WorkspaceSourceMenuProps & {
  selectionsCount: number;
  selectionRows: React.ReactNode;
  freshBranchToggle?: React.ReactNode;
  branchLocked?: boolean;
  repositoryCreationOpen?: boolean;
};

export function MobileMixedRepositoryChips({
  selectionsCount,
  selectionRows,
  freshBranchToggle,
  branchLocked,
  repositoryLocked,
  repositoryCreationOpen,
  ...sourceProps
}: MobileMixedRepositoryChipsProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [view, setView] = useState<"manage" | WorkspaceSourceMenuView>(
    selectionsCount > 0 ? "manage" : "menu",
  );
  const hasManagement = selectionsCount > 0;
  const close = () => {
    setOpen(false);
    setView(hasManagement ? "manage" : "menu");
  };
  const openSheet = () => {
    sourceProps.accessible.refresh?.();
    sourceProps.onRefreshRepositories?.();
    setView(hasManagement ? "manage" : "menu");
    setOpen(true);
  };
  return (
    <>
      <button
        type="button"
        onClick={openSheet}
        aria-haspopup="dialog"
        data-testid="mobile-repository-manager"
        className="inline-flex min-h-11 items-center justify-center rounded-md px-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        {hasManagement
          ? t("task:repositoriesSelected", { count: selectionsCount })
          : t("task:addRepositoryFolder")}
      </button>
      <MobileRepositorySheet
        open={open}
        onOpenChange={(nextOpen) => (nextOpen ? setOpen(true) : close())}
        view={view}
        hasManagement={hasManagement}
        selectionsCount={selectionsCount}
        selectionRows={selectionRows}
        freshBranchToggle={freshBranchToggle}
        branchLocked={branchLocked}
        repositoryLocked={repositoryLocked}
        repositoryCreationOpen={repositoryCreationOpen}
        sourceProps={sourceProps}
        onClose={close}
        onViewChange={setView}
      />
    </>
  );
}

function MobileRepositorySheet({
  open,
  onOpenChange,
  view,
  hasManagement,
  selectionsCount,
  selectionRows,
  freshBranchToggle,
  branchLocked,
  repositoryLocked,
  repositoryCreationOpen,
  sourceProps,
  onClose,
  onViewChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  view: "manage" | WorkspaceSourceMenuView;
  hasManagement: boolean;
  selectionsCount: number;
  selectionRows: React.ReactNode;
  freshBranchToggle?: React.ReactNode;
  branchLocked?: boolean;
  repositoryLocked?: boolean;
  repositoryCreationOpen?: boolean;
  sourceProps: WorkspaceSourceMenuProps;
  onClose: () => void;
  onViewChange: (view: "manage" | WorkspaceSourceMenuView) => void;
}) {
  const { t } = useTranslation();
  const sourceView = view === "manage" ? "menu" : view;
  return (
    <MobilePickerSheet
      open={open}
      onOpenChange={onOpenChange}
      title={mobileSourceTitle(t, view, selectionsCount)}
      headerAction={
        view === "manage" || (!hasManagement && view === "menu") ? (
          <Button
            type="button"
            variant="ghost"
            onClick={onClose}
            data-testid="mobile-repository-done"
            className="min-h-11 cursor-pointer px-2 text-xs"
          >
            {t("task:done")}
          </Button>
        ) : (
          <Button
            type="button"
            variant="ghost"
            onClick={() => onViewChange(view === "menu" ? "manage" : "menu")}
            data-testid="mobile-repository-back"
            className="min-h-11 cursor-pointer px-2 text-xs"
          >
            <IconArrowLeft className="mr-1 size-4" aria-hidden="true" />
            {t("common:back")}
          </Button>
        )
      }
      contentTestId="mobile-repository-sheet-content"
      onEscapeKeyDown={(event) => {
        if (
          repositoryCreationOpen ||
          document.querySelector('[data-testid="create-local-repository-drawer"]')
        ) {
          event.preventDefault();
        }
      }}
      onPointerDownOutside={repositoryCreationOpen ? (event) => event.preventDefault() : undefined}
      onFocusOutside={(event) => event.preventDefault()}
    >
      {view === "manage" ? (
        <MobileRepositoryManagement
          selectionRows={selectionRows}
          freshBranchToggle={freshBranchToggle}
          branchLocked={branchLocked}
          repositoryLocked={repositoryLocked}
          selectionsCount={selectionsCount}
          onAdd={() => {
            sourceProps.accessible.refresh?.();
            sourceProps.onRefreshRepositories?.();
            onViewChange("menu");
          }}
        />
      ) : (
        <WorkspaceSourceMenuContent
          {...sourceProps}
          view={sourceView}
          showHeader={false}
          onViewChange={onViewChange}
          onClose={onClose}
        />
      )}
    </MobilePickerSheet>
  );
}

function MobileRepositoryManagement({
  selectionRows,
  freshBranchToggle,
  branchLocked,
  repositoryLocked,
  selectionsCount,
  onAdd,
}: {
  selectionRows: React.ReactNode;
  freshBranchToggle?: React.ReactNode;
  branchLocked?: boolean;
  repositoryLocked?: boolean;
  selectionsCount: number;
  onAdd: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3" data-testid="mobile-repository-management">
      <div className="flex flex-col gap-2">{selectionRows}</div>
      {branchLocked ? null : freshBranchToggle}
      {repositoryLocked ? null : (
        <button
          type="button"
          onClick={onAdd}
          data-testid="mobile-repository-add"
          className="inline-flex min-h-11 items-center justify-center self-start rounded-md px-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          {sourceAddLabel(t, selectionsCount)}
        </button>
      )}
    </div>
  );
}

type DesktopMixedRepositoryChipsProps = WorkspaceSourceMenuProps & {
  selectionRows: React.ReactNode;
  freshBranchToggle?: React.ReactNode;
  branchLocked?: boolean;
};

export function DesktopMixedRepositoryChips({
  selectionRows,
  freshBranchToggle,
  branchLocked,
  ...sourceProps
}: DesktopMixedRepositoryChipsProps) {
  return (
    <div
      className="flex min-h-9 w-full flex-wrap items-center gap-2"
      data-testid="mixed-repository-chips"
    >
      {selectionRows}
      {branchLocked ? null : freshBranchToggle}
      {sourceProps.repositoryLocked ? null : <DesktopWorkspaceSourceMenu {...sourceProps} />}
    </div>
  );
}

function mobileSourceTitle(
  t: (key: string, options?: { count: number }) => string,
  view: "manage" | WorkspaceSourceMenuView,
  count: number,
) {
  if (view === "manage") return t("task:repositoriesSelected", { count });
  if (view === "menu") return t("task:addToWorkspace");
  if (view === "repository") return t("task:addRepositorySource");
  if (view === "folder") return t("task:addLocalFolder");
  return t("task:addRepositorySet");
}

function sourceAddLabel(t: (key: string, options?: { count: number }) => string, count: number) {
  return count === 0 ? t("task:addRepositoryFolder") : t("task:add");
}
