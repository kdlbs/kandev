"use client";

import { useMemo, useState } from "react";
import {
  IconArrowLeft,
  IconChevronRight,
  IconFolder,
  IconPlus,
  IconStack2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";

import { FolderPicker } from "@/components/folder-picker";
import {
  RepositoryPicker,
  type LocalRepositoryChoice,
} from "@/components/task-create-dialog-repository-picker";
import { SaveRepositorySetDialog } from "@/components/task-create-dialog-repository-sets-save";
import { applyRepositorySet } from "@/components/task-create-dialog-repository-sets";
import type { TaskRepositorySetsConfig } from "@/components/task-create-dialog-types";
import type { LocalRepository, Repository, RepositorySet } from "@/lib/types/http";
import type {
  RemoteRepository,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import { cn } from "@/lib/utils";

export type WorkspaceSourceMenuView = "menu" | "repository" | "folder" | "set";

export type WorkspaceSourceMenuProps = {
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  accessible: UseRemoteRepositoriesResult;
  workspaceId: string | null;
  remoteOriginMode?: boolean;
  selectionsCount?: number;
  repositoryLocked?: boolean;
  folderAvailable: boolean;
  folderDisabledReason?: string;
  repositorySets?: TaskRepositorySetsConfig;
  onSelectLocal: (choice: LocalRepositoryChoice) => void;
  onSelectRemote: (repository: RemoteRepository) => void;
  onPasteRemote: (url: string) => void;
  onSelectFolder: (path: string) => void;
  onCreateRepository?: () => void;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
  repositoryCreationOpen?: boolean;
};

export function DesktopWorkspaceSourceMenu(props: WorkspaceSourceMenuProps) {
  const { t } = useTranslation();
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const [open, setOpen] = useState(false);
  const [view, setView] = useState<WorkspaceSourceMenuView>("menu");
  const close = () => {
    setOpen(false);
    setView("menu");
  };
  const openMenu = () => {
    refreshRepositories(props);
    setView("menu");
    setOpen(true);
  };
  return (
    <Popover open={open} onOpenChange={(nextOpen) => (nextOpen ? setOpen(true) : close())}>
      <PopoverTrigger asChild>
        <button
          type="button"
          onClick={openMenu}
          aria-haspopup="dialog"
          aria-label={sourceMenuLabel(t, props.selectionsCount ?? 0)}
          data-testid="add-repository"
          className="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-md px-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground sm:min-h-7 sm:text-[11px]"
        >
          <IconPlus className="size-3.5" aria-hidden="true" />
          <span>{sourceMenuLabel(t, props.selectionsCount ?? 0)}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[440px] max-w-[calc(100vw-2rem)] overflow-hidden p-0"
        data-testid="workspace-source-menu"
        portalContainer={portalContainer}
        onEscapeKeyDown={(event) => {
          if (props.repositoryCreationOpen) event.preventDefault();
        }}
        onPointerDownOutside={
          props.repositoryCreationOpen ? (event) => event.preventDefault() : undefined
        }
      >
        <WorkspaceSourceMenuContent {...props} view={view} onViewChange={setView} onClose={close} />
      </PopoverContent>
    </Popover>
  );
}

export function WorkspaceSourceMenuContent({
  view,
  onViewChange,
  onClose,
  showHeader = true,
  ...props
}: WorkspaceSourceMenuProps & {
  view: WorkspaceSourceMenuView;
  onViewChange: (view: WorkspaceSourceMenuView) => void;
  onClose: () => void;
  showHeader?: boolean;
}) {
  const { t } = useTranslation();
  if (view === "menu") {
    return <WorkspaceSourceMenuOptions {...props} onViewChange={onViewChange} />;
  }
  return (
    <div className="flex min-w-0 flex-col" data-testid={`workspace-source-view-${view}`}>
      {showHeader ? (
        <SourceViewHeader title={sourceViewTitle(t, view)} onBack={() => onViewChange("menu")} />
      ) : null}
      <SourceViewBody view={view} {...props} onClose={onClose} />
    </div>
  );
}

function WorkspaceSourceMenuOptions({
  folderAvailable,
  folderDisabledReason,
  onViewChange,
}: Pick<WorkspaceSourceMenuProps, "folderAvailable" | "folderDisabledReason"> & {
  onViewChange: (view: WorkspaceSourceMenuView) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col p-1" data-testid="workspace-source-menu-options">
      <SourceMenuOption
        icon={<IconFolder className="size-4" aria-hidden="true" />}
        label={t("task:addRepositorySource")}
        testId="workspace-source-menu-repository"
        onClick={() => onViewChange("repository")}
      />
      <SourceMenuOption
        icon={<IconFolder className="size-4" aria-hidden="true" />}
        label={t("task:addLocalFolder")}
        testId="workspace-source-menu-folder"
        disabled={!folderAvailable}
        description={
          folderAvailable ? undefined : (folderDisabledReason ?? t("task:addFolderExecutorUnknown"))
        }
        onClick={() => onViewChange("folder")}
      />
      <SourceMenuOption
        icon={<IconStack2 className="size-4" aria-hidden="true" />}
        label={t("task:addRepositorySet")}
        testId="workspace-source-menu-set"
        onClick={() => onViewChange("set")}
      />
    </div>
  );
}

function SourceMenuOption({
  icon,
  label,
  testId,
  description,
  disabled = false,
  onClick,
}: {
  icon: React.ReactNode;
  label: string;
  testId: string;
  description?: string;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "flex min-h-11 items-center gap-3 rounded-md px-3 py-2 text-left text-xs sm:min-h-7 sm:gap-2 sm:px-2 sm:py-1 [@media(pointer:coarse)]:!min-h-11",
        disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer hover:bg-muted",
      )}
      data-testid={testId}
    >
      <span className="shrink-0 text-muted-foreground">{icon}</span>
      <span className="min-w-0 flex-1">
        <span className="block text-xs font-medium">{label}</span>
        {description ? (
          <span className="block text-xs text-muted-foreground">{description}</span>
        ) : null}
      </span>
      <IconChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </button>
  );
}

function SourceViewHeader({ title, onBack }: { title: string; onBack: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-11 items-center gap-2 border-b border-border px-2">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={onBack}
        data-testid="workspace-source-back"
      >
        <IconArrowLeft className="mr-1 size-4" aria-hidden="true" />
        {t("common:back")}
      </Button>
      <span className="min-w-0 truncate text-xs font-medium">{title}</span>
    </div>
  );
}

function SourceViewBody({
  view,
  onClose,
  ...props
}: Omit<WorkspaceSourceMenuProps, "selectionsCount" | "repositoryLocked"> & {
  view: Exclude<WorkspaceSourceMenuView, "menu">;
  onClose: () => void;
  repositoryLocked?: boolean;
}) {
  if (view === "repository") {
    return (
      <RepositoryPicker
        repositories={props.repositories}
        discoveredRepositories={props.discoveredRepositories}
        accessible={props.accessible}
        workspaceId={props.workspaceId}
        remoteOriginMode={props.remoteOriginMode}
        scopeKey={props.workspaceId ?? ""}
        onSelectLocal={(choice) => {
          props.onSelectLocal(choice);
          onClose();
        }}
        onSelectRemote={(repository) => {
          props.onSelectRemote(repository);
          onClose();
        }}
        onPasteRemote={(url) => {
          props.onPasteRemote(url);
          onClose();
        }}
        onRefresh={() => refreshRepositories(props)}
        refreshing={props.repositoriesRefreshing}
        onCreateRepository={
          props.onCreateRepository
            ? () => {
                props.onCreateRepository?.();
                onClose();
              }
            : undefined
        }
      />
    );
  }
  if (view === "folder") {
    return (
      <FolderPicker
        value=""
        autoOpen
        allowScratch={false}
        placeholder={useFolderPlaceholder()}
        onChange={(path) => {
          if (path) props.onSelectFolder(path);
          onClose();
        }}
      />
    );
  }
  if (!props.repositorySets) return <EmptySourceView />;
  return <RepositorySetPickerView config={props.repositorySets} onApply={onClose} />;
}

function useFolderPlaceholder() {
  const { t } = useTranslation();
  return t("task:folderPathPlaceholder");
}

function EmptySourceView() {
  const { t } = useTranslation();
  return <p className="p-4 text-sm text-muted-foreground">{t("task:repositorySetsUnavailable")}</p>;
}

export function RepositorySetPickerView({
  config,
  onApply,
}: {
  config: TaskRepositorySetsConfig;
  onApply: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex max-h-[min(360px,calc(100vh-16rem))] flex-col overflow-y-auto p-1"
      data-testid="workspace-source-set-picker"
    >
      {config.sets.length === 0 ? (
        <p className="p-3 text-xs text-muted-foreground">{t("task:repositorySetsEmpty")}</p>
      ) : (
        config.sets.map((set) => (
          <RepositorySetOption key={set.id} set={set} config={config} onApply={onApply} />
        ))
      )}
      {config.save ? (
        <button
          type="button"
          className="mt-1 min-h-11 rounded-md border-t border-border px-3 py-2 text-left text-xs font-medium hover:bg-muted sm:min-h-7 sm:px-2 sm:py-1 [@media(pointer:coarse)]:!min-h-11"
          data-testid="repository-set-save-action"
          onClick={() => config.save?.setOpen(true)}
        >
          {t("task:repositorySetsSaveAction")}
        </button>
      ) : null}
      {config.save ? <RepositorySetSaveDialog config={config.save} /> : null}
    </div>
  );
}

function RepositorySetOption({
  set,
  config,
  onApply,
}: {
  set: RepositorySet;
  config: TaskRepositorySetsConfig;
  onApply: () => void;
}) {
  const { t } = useTranslation();
  const outcome = useMemo(
    () =>
      applyRepositorySet({
        rows: config.rows ?? config.save?.rows ?? [],
        set,
        repositories: config.repositories ?? config.save?.repositories ?? [],
      }),
    [config.repositories, config.rows, config.save?.repositories, config.save?.rows, set],
  );
  return (
    <button
      type="button"
      className="flex min-h-11 flex-col items-start rounded-md px-3 py-2 text-left text-xs hover:bg-muted sm:min-h-7 sm:px-2 sm:py-1 [@media(pointer:coarse)]:!min-h-11"
      data-testid="repository-set-option"
      onClick={() => {
        config.onApply(set);
        onApply();
      }}
    >
      <span className="text-xs font-medium">{set.name}</span>
      <span className="text-xs text-muted-foreground">
        {t("task:repositorySetsMemberCount", { count: set.repositories.length })}
        {outcome.missingCount > 0
          ? ` ${t("task:repositorySetsMissingMembers", { count: outcome.missingCount })}`
          : ""}
      </span>
    </button>
  );
}

function RepositorySetSaveDialog({
  config,
}: {
  config: NonNullable<TaskRepositorySetsConfig["save"]>;
}) {
  return (
    <SaveRepositorySetDialog
      open={config.open}
      onOpenChange={config.setOpen}
      workspaceId={config.workspaceId}
      rows={config.rows}
      selections={config.selections}
      repositories={config.repositories}
      isLocalExecutor={config.isLocalExecutor}
      freshBranchEnabled={config.freshBranchEnabled}
    />
  );
}

export function sourceMenuLabel(
  t: (key: string, options?: { count: number }) => string,
  count: number,
) {
  return count === 0 ? t("task:addRepositoryFolder") : t("task:add");
}

function sourceViewTitle(
  t: (key: string) => string,
  view: Exclude<WorkspaceSourceMenuView, "menu">,
) {
  if (view === "repository") return t("task:addRepositorySource");
  if (view === "folder") return t("task:addLocalFolder");
  return t("task:addRepositorySet");
}

function refreshRepositories({ accessible, onRefreshRepositories }: WorkspaceSourceMenuProps) {
  accessible.refresh?.();
  onRefreshRepositories?.();
}
